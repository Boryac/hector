package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/verikod/hector/internal/researchos/config"
	"github.com/verikod/hector/internal/researchos/secrets"
	"github.com/verikod/hector/internal/researchos/settings"
)

// stubBackend 是设置持久化的内存替身：接口测试不碰数据库。
type stubBackend struct {
	mu   sync.Mutex
	rows map[string]settings.StoredValue
}

func newStubBackend() *stubBackend { return &stubBackend{rows: map[string]settings.StoredValue{}} }

func (s *stubBackend) LoadSettings(ctx context.Context) (map[string]settings.StoredValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]settings.StoredValue{}
	for k, v := range s.rows {
		out[k] = v
	}
	return out, nil
}

func (s *stubBackend) SaveSetting(ctx context.Context, key, section string, value any, encrypted bool, updatedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[key] = settings.StoredValue{Value: value, Encrypted: encrypted}
	return nil
}

func (s *stubBackend) DeleteSetting(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, key)
	return nil
}

const testToken = "test-token-for-settings"

func testConfig() config.Config {
	return config.Config{
		Addr:            "127.0.0.1:8080",
		DatabaseURL:     "postgres://postgres:secretpw@127.0.0.1:55432/researchos?sslmode=disable",
		APIToken:        testToken,
		LocalUser:       "local-user",
		LLMProvider:     config.LLMNone,
		LLMTimeout:      90 * time.Second,
		SearchProviders: []string{"bing", "github"},
		SearchTimeout:   30 * time.Second,
		SearchResults:   6,
		FetchTimeout:    25 * time.Second,
		FetchMaxBytes:   3_000_000,
		RespectRobots:   true,
		UserAgent:       "test-agent",
		DefaultDepth:    "standard",
		RequireApproval: true,
		Workers:         4,
		MaxAgents:       4,
		NodeTimeout:     180 * time.Second,
		TaskTimeout:     1800 * time.Second,
		NodeMaxAttempts: 3,
		DataDir:         "data",
	}
}

func newSettingsServer(t *testing.T) (*Server, *stubBackend) {
	t.Helper()
	key := make([]byte, secrets.KeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	box, err := secrets.NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	be := newStubBackend()
	cfg := testConfig()
	mgr := settings.NewManager(be, box, "test", cfg)
	if err := mgr.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Options{Cfg: &cfg, Settings: mgr, Version: "test"})
	return srv, be
}

func doReq(t *testing.T, h http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		buf = bytes.NewBuffer(raw)
	} else {
		buf = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON: %v\n%s", err, rec.Body.String())
	}
	return out
}

// 设置接口必须鉴权：能打开网页不等于能改配置。
func TestSettingsRequiresAuth(t *testing.T) {
	srv, _ := newSettingsServer(t)
	h := srv.Handler()

	if rec := doReq(t, h, http.MethodGet, "/api/v1/settings", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 读设置应 401，实际 %d", rec.Code)
	}
	if rec := doReq(t, h, http.MethodPut, "/api/v1/settings/model", map[string]any{"model.provider": "openai"}, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 写设置应 401，实际 %d", rec.Code)
	}
}

// GET 返回分组与来源说明，且绝不包含明文密钥与数据库密码。
func TestGetSettingsShapeAndNoLeak(t *testing.T) {
	srv, _ := newSettingsServer(t)
	h := srv.Handler()

	rec := doReq(t, h, http.MethodGet, "/api/v1/settings", nil, testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)

	sections, ok := body["sections"].([]any)
	if !ok || len(sections) == 0 {
		t.Fatal("应返回配置分组")
	}
	if _, ok := body["precedence"]; !ok {
		t.Fatal("应说明优先级")
	}
	if body["secrets"].(map[string]any)["cipher"] != "AES-256-GCM" {
		t.Fatal("应说明加密算法")
	}

	raw := rec.Body.String()
	if strings.Contains(raw, testToken) {
		t.Fatal("响应泄露了 API Token")
	}
	if strings.Contains(raw, "secretpw") {
		t.Fatal("响应泄露了数据库密码")
	}
}

// 保存密钥后：接口只回掩码，数据库里是密文。
func TestPutSecretReturnsMaskOnly(t *testing.T) {
	srv, be := newSettingsServer(t)
	h := srv.Handler()
	plain := "sk-live-abcdefghijklmnop-9876"

	rec := doReq(t, h, http.MethodPut, "/api/v1/settings/model", map[string]any{
		"model.provider": "openai",
		"model.api_key":  plain,
	}, testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("保存失败: %d %s", rec.Code, rec.Body.String())
	}
	resp := decode(t, rec)
	result := resp["result"].(map[string]any)
	if result["hot_applied"] != true {
		t.Fatalf("模型设置应即时生效，实际 %v", result["hot_applied"])
	}
	if msg, _ := result["message"].(string); !strings.Contains(msg, "即时生效") {
		t.Fatalf("应给出明确的生效反馈，实际 %q", msg)
	}

	stored, _ := be.rows["model.api_key"].Value.(string)
	if !secrets.IsEncrypted(stored) || strings.Contains(stored, plain) {
		t.Fatalf("应密文入库，实际 %q", stored)
	}
	if strings.Contains(rec.Body.String(), plain) {
		t.Fatal("保存响应泄露了明文")
	}

	// 再读一次：掩码可见，明文不可见。
	got := doReq(t, h, http.MethodGet, "/api/v1/settings", nil, testToken)
	if strings.Contains(got.Body.String(), plain) {
		t.Fatal("GET 泄露了明文密钥")
	}
	var found bool
	for _, s := range decode(t, got)["sections"].([]any) {
		for _, f := range s.(map[string]any)["fields"].([]any) {
			fm := f.(map[string]any)
			if fm["key"] == "model.api_key" {
				found = true
				if fm["configured"] != true {
					t.Fatal("configured 应为 true")
				}
				if v, _ := fm["value"].(string); !strings.Contains(v, "****") {
					t.Fatalf("应返回掩码，实际 %q", v)
				}
			}
		}
	}
	if !found {
		t.Fatal("未找到 model.api_key 字段")
	}
}

// 部署级参数不能通过界面改：这是产品约束，不是"暂时不支持"。
func TestPutDeployFieldForbidden(t *testing.T) {
	srv, be := newSettingsServer(t)
	h := srv.Handler()
	for _, key := range []string{"system.addr", "system.database", "system.api_token"} {
		rec := doReq(t, h, http.MethodPut, "/api/v1/settings/system", map[string]any{key: "hacked"}, testToken)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s 应返回 403，实际 %d %s", key, rec.Code, rec.Body.String())
		}
		if _, exists := be.rows[key]; exists {
			t.Fatalf("%s 不应落库", key)
		}
	}
}

func TestPutValidationErrors(t *testing.T) {
	srv, _ := newSettingsServer(t)
	h := srv.Handler()

	cases := []struct {
		section string
		body    map[string]any
		code    int
		expect  string
	}{
		{"execution", map[string]any{"execution.max_agents": 99}, http.StatusUnprocessableEntity, "1 到 8"},
		{"execution", map[string]any{"execution.node_timeout": "abc"}, http.StatusUnprocessableEntity, "合法时长"},
		{"model", map[string]any{"model.nope": 1}, http.StatusUnprocessableEntity, "未知的配置项"},
		{"nope", map[string]any{"model.provider": "openai"}, http.StatusUnprocessableEntity, "未知的配置项"},
	}
	for _, c := range cases {
		rec := doReq(t, h, http.MethodPut, "/api/v1/settings/"+c.section, c.body, testToken)
		if rec.Code != c.code {
			t.Fatalf("%v 期望 %d，实际 %d", c.body, c.code, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), c.expect) {
			t.Fatalf("错误信息应含 %q，实际 %s", c.expect, rec.Body.String())
		}
	}

	// 空 patch 也要拒绝，避免前端误发空请求后以为保存成功。
	if rec := doReq(t, h, http.MethodPut, "/api/v1/settings/model", map[string]any{}, testToken); rec.Code != http.StatusBadRequest {
		t.Fatalf("空 patch 应 400，实际 %d", rec.Code)
	}
}

// 需要重启的项要在响应里点明，前端才能提示用户。
func TestPutRestartScopedReportsRequiresRestart(t *testing.T) {
	srv, _ := newSettingsServer(t)
	h := srv.Handler()
	rec := doReq(t, h, http.MethodPut, "/api/v1/settings/execution", map[string]any{"execution.workers": 8}, testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}
	result := decode(t, rec)["result"].(map[string]any)
	list, _ := result["requires_restart"].([]any)
	if len(list) != 1 || list[0] != "execution.workers" {
		t.Fatalf("应标注需重启项，实际 %v", result["requires_restart"])
	}
	if result["hot_applied"] == true {
		t.Fatal("需重启的项不应声称即时生效")
	}
}

// 清除覆盖：删除后回落到环境变量/默认值。
func TestDeleteClearsOverride(t *testing.T) {
	srv, be := newSettingsServer(t)
	h := srv.Handler()
	if rec := doReq(t, h, http.MethodPut, "/api/v1/settings/execution", map[string]any{"execution.max_agents": 7}, testToken); rec.Code != http.StatusOK {
		t.Fatalf("保存失败: %d", rec.Code)
	}
	rec := doReq(t, h, http.MethodDelete, "/api/v1/settings/execution/execution.max_agents", nil, testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("清除失败: %d %s", rec.Code, rec.Body.String())
	}
	if _, exists := be.rows["execution.max_agents"]; exists {
		t.Fatal("清除后不应还留在库里")
	}
	// 运行时回落到环境变量基线 4。
	if got := srv.Settings.Runtime().Cfg.MaxAgents; got != 4 {
		t.Fatalf("清除后应回落到 4，实际 %d", got)
	}
}

// 连接测试：provider=none 时不能假装成功；缺 Key 时要说清缺什么。
func TestLLMTestEndpointHonestFailures(t *testing.T) {
	srv, _ := newSettingsServer(t)
	h := srv.Handler()

	rec := doReq(t, h, http.MethodPost, "/api/v1/settings/llm/test", nil, testToken)
	body := decode(t, rec)
	if body["ok"] != false || body["reason"] != "provider_none" {
		t.Fatalf("provider=none 应明确返回不可测，实际 %v", body)
	}

	rec = doReq(t, h, http.MethodPost, "/api/v1/settings/llm/test", map[string]any{
		"provider": "openai", "api_key": "", "model": "gpt-4o-mini",
	}, testToken)
	body = decode(t, rec)
	if body["ok"] != false || body["reason"] != "missing_credentials" {
		t.Fatalf("缺 Key 应返回 missing_credentials，实际 %v", body)
	}
}

// 搜索测试：没有启用 Provider 时不能报「可用」。
func TestSearchTestEndpointNoProvider(t *testing.T) {
	key := make([]byte, secrets.KeySize)
	box, _ := secrets.NewBox(key)
	cfg := testConfig()
	cfg.SearchProviders = []string{}
	be := newStubBackend()
	mgr := settings.NewManager(be, box, "test", cfg)
	_ = mgr.Reload(context.Background())
	srv := NewServer(Options{Cfg: &cfg, Settings: mgr, Version: "test"})

	rec := doReq(t, srv.Handler(), http.MethodPost, "/api/v1/settings/search/test", nil, testToken)
	body := decode(t, rec)
	if body["ok"] != false || body["reason"] != "no_provider" {
		t.Fatalf("无 Provider 时应明确失败，实际 %v", body)
	}
}

// 秘密项回传掩码时不能覆盖原值（前端常见行为）。
func TestPutMaskedSecretIsIgnored(t *testing.T) {
	srv, be := newSettingsServer(t)
	h := srv.Handler()
	plain := "sk-original-key-1234567890"
	if rec := doReq(t, h, http.MethodPut, "/api/v1/settings/model", map[string]any{"model.api_key": plain}, testToken); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	before := be.rows["model.api_key"].Value

	for _, echo := range []string{"***", "sk-o****7890", ""} {
		if rec := doReq(t, h, http.MethodPut, "/api/v1/settings/model", map[string]any{"model.api_key": echo}, testToken); rec.Code != http.StatusOK {
			t.Fatalf("回传 %q 不应报错: %s", echo, rec.Body.String())
		}
	}
	if after := be.rows["model.api_key"].Value; after != before {
		t.Fatal("回传掩码不应改写已保存的密钥")
	}
	if got := srv.Settings.Runtime().Cfg.LLMAPIKey; got != plain {
		t.Fatalf("运行时密钥不应变化，实际 %q", got)
	}
}
