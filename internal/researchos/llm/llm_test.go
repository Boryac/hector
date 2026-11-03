package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUnavailableWithoutKey(t *testing.T) {
	c := New(Config{Provider: "openai"})
	if c.Available() {
		t.Fatal("没有 Key 时不应可用")
	}
	if _, _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("不可用时调用应返回错误，而不是静默返回空串")
	}
	if _, _, err := c.CompleteJSON(context.Background(), "s", "u"); err == nil {
		t.Fatal("不可用时 CompleteJSON 也应报错")
	}
	// ollama 是本地服务，不要求 Key
	if !New(Config{Provider: "ollama"}).Available() {
		t.Fatal("ollama 无 Key 也应可用")
	}
}

func TestProviderDefaults(t *testing.T) {
	cases := []struct{ provider, model, base string }{
		{"deepseek", "deepseek-chat", "https://api.deepseek.com/v1"},
		{"ollama", "qwen2.5:7b", "http://127.0.0.1:11434/v1"},
		{"openai", "gpt-4o-mini", "https://api.openai.com/v1"},
	}
	for _, c := range cases {
		got := New(Config{Provider: c.provider, APIKey: "k"})
		if got.Model() != c.model {
			t.Fatalf("%s 默认模型应为 %s，实际 %s", c.provider, c.model, got.Model())
		}
		impl := got.(*openAICompat)
		if impl.cfg.BaseURL != c.base {
			t.Fatalf("%s 默认 BaseURL 应为 %s，实际 %s", c.provider, c.base, impl.cfg.BaseURL)
		}
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc) Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(Config{Provider: "openai", APIKey: "test-key", Model: "m1", BaseURL: srv.URL + "/v1", Timeout: 5 * time.Second})
}

func TestCompleteSendsExpectedRequest(t *testing.T) {
	var gotPath, gotAuth string
	var body map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "hello"}}},
			"usage":   map[string]int{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
		})
	})
	out, usage, err := c.Complete(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("调用失败：%v", err)
	}
	if out != "hello" || usage.TotalTokens != 5 {
		t.Fatalf("响应解析错误：%q %+v", out, usage)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("请求路径应为 /v1/chat/completions，实际 %s", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization 头错误：%q", gotAuth)
	}
	if body["model"] != "m1" {
		t.Fatalf("model 字段错误：%v", body["model"])
	}
	if _, ok := body["response_format"]; ok {
		t.Fatal("普通补全不应带 response_format")
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("应发送 system+user 两条消息，实际 %d", len(msgs))
	}
}

func TestCompleteJSONSetsJSONModeAndStripsFence(t *testing.T) {
	var sawJSONMode bool
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if rf, ok := body["response_format"].(map[string]any); ok && rf["type"] == "json_object" {
			sawJSONMode = true
		}
		// 真实模型经常把 JSON 包在 ```json 里，甚至前后带解释
		content := "好的，结果如下：\n```json\n{\"claims\":[{\"statement\":\"s\"}]}\n```\n完毕。"
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
		})
	})
	raw, _, err := c.CompleteJSON(context.Background(), "s", "u")
	if err != nil {
		t.Fatalf("CompleteJSON 失败：%v", err)
	}
	if !sawJSONMode {
		t.Fatal("CompleteJSON 应设置 response_format=json_object")
	}
	var parsed struct {
		Claims []struct {
			Statement string `json:"statement"`
		} `json:"claims"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("返回内容不是合法 JSON：%v（%s）", err, raw)
	}
	if parsed.Claims[0].Statement != "s" {
		t.Fatalf("解析结果错误：%+v", parsed)
	}
}

func TestHTTPErrorsDoNotLeakKey(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key sk-secret-123"}}`))
	})
	_, _, err := c.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("401 应返回错误")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("错误信息应包含状态码：%v", err)
	}
	// 上游返回体里可能带 key 片段，错误信息只截断到 300 字符，不会整段回显
	if len(err.Error()) > 400 {
		t.Fatalf("错误信息不应过长：%d 字符", len(err.Error()))
	}
}

func TestEmptyChoicesIsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	})
	if _, _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("空 choices 应报错，不能返回空串让调用方以为成功")
	}
}

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"纯 JSON", `{"a":1}`, `{"a":1}`, true},
		{"带前后废话", `说明: {"a":1} 结束`, `{"a":1}`, true},
		{"带代码块", "```json\n{\"a\":\"b\"}\n```", `{"a":"b"}`, true},
		{"嵌套对象", `{"a":{"b":{"c":1}}}`, `{"a":{"b":{"c":1}}}`, true},
		{"字符串里含括号", `{"a":"}{"}`, `{"a":"}{"}`, true},
		{"转义引号", `{"a":"he said \"hi\""}`, `{"a":"he said \"hi\""}`, true},
		{"无 JSON", "抱歉，我无法回答", "", false},
		{"不完整", `{"a":1`, "", false},
		{"非法 JSON", `{a:1}`, "", false},
	}
	for _, c := range cases {
		got, err := extractJSONObject(c.in)
		if c.ok {
			if err != nil {
				t.Fatalf("%s：不应报错，实际 %v", c.name, err)
			}
			if string(got) != c.want {
				t.Fatalf("%s：期望 %s，实际 %s", c.name, c.want, got)
			}
		} else if err == nil {
			t.Fatalf("%s：应报错，实际返回 %s", c.name, got)
		}
	}
}

// 超时上下文要能被上游正确接收（避免节点卡死在 LLM 调用上）。
func TestRespectsContextCancel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": "late"}}}})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := c.Complete(ctx, "s", "u"); err == nil {
		t.Fatal("超时后应返回错误")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("超时未及时生效，耗时 %v", time.Since(start))
	}
}
