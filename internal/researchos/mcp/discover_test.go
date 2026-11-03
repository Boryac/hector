package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// 用测试二进制自己充当 MCP Server：不依赖外部脚本与平台差异，
// 走的是真实的进程启动 + stdio 往返，不是打桩。
func TestMain(m *testing.M) {
	if os.Getenv("RESEARCHOS_FAKE_MCP") == "1" {
		runFakeServer()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runFakeServer() {
	mode := os.Getenv("RESEARCHOS_FAKE_MCP_MODE")
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	for {
		line, err := in.ReadBytes('\n')
		if err != nil {
			return
		}
		line = []byte(strings.TrimSpace(string(line)))
		if len(line) == 0 {
			continue
		}
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		if req.Method == "notifications/initialized" {
			continue
		}
		if mode == "silent" {
			continue // 故意不响应，用于超时测试
		}
		switch req.Method {
		case "initialize":
			fmt.Fprintf(out, `{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"fake","version":"0.1"}}}`+"\n", req.ID)
		case "tools/list":
			// 先发一条通知，再发响应：客户端必须能跳过干扰行。
			fmt.Fprint(out, `{"jsonrpc":"2.0","method":"notifications/message","params":{"level":"info"}}`+"\n")
			fmt.Fprintf(out, `{"jsonrpc":"2.0","id":%d,"result":{"tools":[{"name":"read_file","description":"读文件","inputSchema":{"type":"object"}},{"name":"run_shell","description":"执行命令"}]}}`+"\n", req.ID)
		}
		out.Flush()
	}
}

func fakeEnv(t *testing.T, mode string) map[string]string {
	t.Helper()
	return map[string]string{"RESEARCHOS_FAKE_MCP": "1", "RESEARCHOS_FAKE_MCP_MODE": mode}
}

func TestInitializeRequestShape(t *testing.T) {
	req := InitializeRequest(1, "researchos", "1.0")
	if req.JSONRPC != "2.0" || req.Method != "initialize" || req.ID != 1 {
		t.Fatalf("JSON-RPC 字段不正确: %+v", req)
	}
	params, ok := req.Params.(map[string]any)
	if !ok {
		t.Fatal("params 应为 map")
	}
	if params["protocolVersion"] != "2024-11-05" {
		t.Fatalf("协议版本不正确: %v", params["protocolVersion"])
	}
	if _, ok := params["clientInfo"]; !ok {
		t.Fatal("缺少 clientInfo")
	}
	// 通知不能带 id，否则会被 Server 当成需要响应的请求。
	if _, hasID := InitializedNotification()["id"]; hasID {
		t.Fatal("initialized 通知不应携带 id")
	}
}

func TestParseToolsListAcceptsBothSchemaNaming(t *testing.T) {
	raw := json.RawMessage(`{"tools":[
		{"name":"a","description":"A","inputSchema":{"type":"object"}},
		{"name":"b","input_schema":{"type":"string"}},
		{"name":"","description":"无名字应被丢弃"}
	]}`)
	tools, err := ParseToolsList(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Fatalf("应解析出 2 个工具，实际 %d", len(tools))
	}
	if string(tools[0].InputSchema) != `{"type":"object"}` {
		t.Fatalf("inputSchema 未保留: %s", tools[0].InputSchema)
	}
	if string(tools[1].InputSchema) != `{"type":"string"}` {
		t.Fatalf("input_schema 未兼容: %s", tools[1].InputSchema)
	}
}

func TestParseToolsListRejectsGarbage(t *testing.T) {
	if _, err := ParseToolsList(nil); err == nil {
		t.Fatal("空结果应报错")
	}
	if _, err := ParseToolsList(json.RawMessage(`not-json`)); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

// 走真实进程：启动、initialize、tools/list、解析，全链路。
func TestDiscoverAgainstRealProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := Discover(ctx, os.Args[0], []string{"-test.run=TestHelperProcess"}, fakeEnv(t, "normal"), 10*time.Second)
	if err != nil {
		t.Fatalf("发现失败: %v", err)
	}
	if len(res.Tools) != 2 {
		t.Fatalf("应发现 2 个工具，实际 %d", len(res.Tools))
	}
	if res.Tools[1].Name != "run_shell" {
		t.Fatalf("工具顺序或内容不符: %+v", res.Tools)
	}
	if res.ProtocolVersion != "2024-11-05" {
		t.Fatalf("协议版本未带回: %q", res.ProtocolVersion)
	}
	if res.ServerInfo["name"] != "fake" {
		t.Fatalf("serverInfo 未带回: %+v", res.ServerInfo)
	}
}

// Server 不响应时要按超时失败，而不是把服务挂在那里等。
func TestDiscoverTimesOut(t *testing.T) {
	ctx := context.Background()
	start := time.Now()
	_, err := Discover(ctx, os.Args[0], []string{"-test.run=TestHelperProcess"}, fakeEnv(t, "silent"), 800*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("应超时失败")
	}
	if elapsed > 6*time.Second {
		t.Fatalf("超时未生效，耗时 %v", elapsed)
	}
}

func TestDiscoverRejectsMissingCommand(t *testing.T) {
	_, err := Discover(context.Background(), "", nil, nil, time.Second)
	if err == nil || !strings.Contains(err.Error(), "命令为空") {
		t.Fatalf("空命令应被拒绝，实际 %v", err)
	}
	_, err = Discover(context.Background(), "definitely-not-a-real-binary-xyz", nil, nil, 2*time.Second)
	if err == nil {
		t.Fatal("不存在的命令应报错")
	}
	if !strings.Contains(err.Error(), "启动 MCP Server 失败") {
		t.Fatalf("错误信息应说明启动失败，实际 %v", err)
	}
}

// 测试辅助：被 TestMain 识别的占位用例（真正逻辑在 runFakeServer）。
func TestHelperProcess(t *testing.T) {
	if os.Getenv("RESEARCHOS_FAKE_MCP") != "1" {
		t.Skip("仅作为子进程入口")
	}
}
