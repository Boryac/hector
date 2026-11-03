// Package mcp 提供最小的 MCP（Model Context Protocol）客户端能力。
//
// 只做一件事：按 stdio 传输执行 `initialize` → `tools/list`，把工具清单取回来。
// 不实现工具调用、不管理长连接——ResearchOS 当前对 MCP 的需求是「让用户看清
// 某个 Server 暴露了什么、再决定要不要批准它」，而不是把工具编排交给外部进程。
//
// 安全边界：执行任何外部命令都是高风险动作，因此
//   - 发现过程有硬超时，超时即杀进程；
//   - 只传入显式登记的 env，不继承整个父进程环境（避免把宿主机的 Key 带进去）；
//   - 输出有大小上限，防止恶意 Server 用无限输出把服务撑死。
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Tool 是 Server 声明的一个工具。
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// Result 是一次发现的结果。
type Result struct {
	Tools           []Tool         `json:"tools"`
	ServerInfo      map[string]any `json:"server_info,omitempty"`
	ProtocolVersion string         `json:"protocol_version,omitempty"`
	DurationMS      int64          `json:"duration_ms"`
}

// 限制。
const (
	// MaxOutputBytes 是读 stdout 的上限。
	MaxOutputBytes = 2 << 20
	// DefaultTimeout 是默认发现超时。
	DefaultTimeout = 15 * time.Second
)

// rpcRequest 是 JSON-RPC 2.0 请求。
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcResponse 是 JSON-RPC 2.0 响应。
type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// InitializeRequest 返回 initialize 请求体（导出的纯函数便于测试协议形状）。
func InitializeRequest(id int, clientName, clientVersion string) rpcRequest {
	return rpcRequest{
		JSONRPC: "2.0", ID: id, Method: "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": clientName, "version": clientVersion},
		},
	}
}

// ToolsListRequest 返回 tools/list 请求体。
func ToolsListRequest(id int) rpcRequest {
	return rpcRequest{JSONRPC: "2.0", ID: id, Method: "tools/list"}
}

// InitializedNotification 返回 initialize 之后的 notification（无 id）。
func InitializedNotification() map[string]any {
	return map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}
}

// ParseToolsList 解析 tools/list 的 result。
//
// 单独抽出来是因为各家实现的字段大小写并不统一：MCP 规范用 inputSchema，
// 也有 Server 返回 input_schema。两种都收，避免因为命名差异判定「没有工具」。
func ParseToolsList(raw json.RawMessage) ([]Tool, error) {
	if len(raw) == 0 {
		return nil, errors.New("tools/list 返回空结果")
	}
	var payload struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
			InputSnake  json.RawMessage `json:"input_schema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("解析 tools/list 失败: %w", err)
	}
	out := make([]Tool, 0, len(payload.Tools))
	for _, t := range payload.Tools {
		if strings.TrimSpace(t.Name) == "" {
			continue
		}
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = t.InputSnake
		}
		out = append(out, Tool{Name: t.Name, Description: t.Description, InputSchema: schema})
	}
	return out, nil
}

// Discover 启动命令并通过 stdio 完成一次工具发现。
func Discover(ctx context.Context, command string, args []string, env map[string]string, timeout time.Duration) (*Result, error) {
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("命令为空")
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	// 只传显式登记的环境变量：继承父进程会把宿主机凭据一并交给外部进程。
	cmd.Env = make([]string, 0, len(env))
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 MCP Server 失败: %w", err)
	}
	// 无论成功失败都要回收，避免僵尸进程。
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	// 后台收集 stderr，出错时作为原因返回（很多 Server 把错误只写 stderr）。
	errBuf := &limitedBuffer{max: 8 << 10}
	go func() { _, _ = io.Copy(errBuf, stderr) }()

	writer := bufio.NewWriter(stdin)
	reader := bufio.NewReaderSize(io.LimitReader(stdout, MaxOutputBytes), 1<<16)

	send := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(b, '\n')); err != nil {
			return err
		}
		return writer.Flush()
	}

	if err := send(InitializeRequest(1, "researchos", "1.0")); err != nil {
		return nil, fmt.Errorf("写入 initialize 失败: %w", err)
	}
	initResp, err := readResponse(reader, 1)
	if err != nil {
		return nil, fmt.Errorf("%w（stderr: %s）", err, strings.TrimSpace(errBuf.String()))
	}
	if initResp.Error != nil {
		return nil, fmt.Errorf("Server 拒绝 initialize: %s", initResp.Error.Message)
	}

	if err := send(InitializedNotification()); err != nil {
		return nil, fmt.Errorf("写入 initialized 失败: %w", err)
	}
	if err := send(ToolsListRequest(2)); err != nil {
		return nil, fmt.Errorf("写入 tools/list 失败: %w", err)
	}
	listResp, err := readResponse(reader, 2)
	if err != nil {
		return nil, fmt.Errorf("%w（stderr: %s）", err, strings.TrimSpace(errBuf.String()))
	}
	if listResp.Error != nil {
		return nil, fmt.Errorf("tools/list 返回错误: %s", listResp.Error.Message)
	}
	tools, err := ParseToolsList(listResp.Result)
	if err != nil {
		return nil, err
	}

	res := &Result{Tools: tools, DurationMS: time.Since(start).Milliseconds()}
	var initPayload struct {
		ProtocolVersion string         `json:"protocolVersion"`
		ServerInfo      map[string]any `json:"serverInfo"`
	}
	if err := json.Unmarshal(initResp.Result, &initPayload); err == nil {
		res.ProtocolVersion = initPayload.ProtocolVersion
		res.ServerInfo = initPayload.ServerInfo
	}
	return res, nil
}

// readResponse 逐行读取，跳过通知与其他 id 的响应，找到目标响应。
func readResponse(r *bufio.Reader, wantID int) (*rpcResponse, error) {
	for i := 0; i < 50; i++ {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, errors.New("Server 在响应前关闭了连接")
			}
			return nil, err
		}
		line = []byte(strings.TrimSpace(string(line)))
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue // 非 JSON 噪声行，跳过
		}
		if resp.ID == wantID {
			return &resp, nil
		}
	}
	return nil, errors.New("等待响应超时（读满 50 行仍未匹配到 id）")
}

// limitedBuffer 是带容量上限的写缓冲。
type limitedBuffer struct {
	buf []byte
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(b.buf) < b.max {
		room := b.max - len(b.buf)
		if room > len(p) {
			room = len(p)
		}
		b.buf = append(b.buf, p[:room]...)
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string { return string(b.buf) }
