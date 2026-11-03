package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/verikod/hector/internal/researchos/config"
)

func TestWithAuthRejectsMissingToken(t *testing.T) {
	h := withAuth("secret-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/research", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("缺少 Token 应 401，实际 %d", rec.Code)
	}
}

func TestWithAuthRejectsWrongToken(t *testing.T) {
	h := withAuth("secret-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, auth := range []string{"Bearer wrong", "Bearer secret-token-extra", "Basic secret-token", "secret-token"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/research", nil)
		req.Header.Set("Authorization", auth)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Authorization=%q 应 401，实际 %d", auth, rec.Code)
		}
	}
}

func TestWithAuthAcceptsToken(t *testing.T) {
	h := withAuth("secret-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/research", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("正确 Token 应放行，实际 %d", rec.Code)
	}
}

// SSE 无法自定义请求头，只允许 query token 走这条通道，并且仍要校验。
func TestWithAuthAcceptsQueryTokenForSSE(t *testing.T) {
	h := withAuth("secret-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/research/abc/events?token=secret-token", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("SSE query token 应放行，实际 %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/research/abc/events?token=wrong", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误的 query token 应 401，实际 %d", rec.Code)
	}
}

func TestWithAuthAllowsHealth(t *testing.T) {
	h := withAuth("secret-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("健康检查应免鉴权，实际 %d", rec.Code)
	}
}

func TestWithAuthAllowsStaticAssets(t *testing.T) {
	h := withAuth("secret-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, path := range []string{"/", "/task/", "/_next/static/chunk.js", "/favicon.ico"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("静态资源 %s 应免鉴权，实际 %d", path, rec.Code)
		}
	}
}

// 只要路径前缀是 /api/ 就必须鉴权（防止未来新增路由忘记加锁）。
func TestWithAuthGuardsAllAPIPrefix(t *testing.T) {
	h := withAuth("secret-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, path := range []string{"/api/v1/settings", "/api/v1/tools", "/api/v2/anything", "/api/v1/research/x/report.md"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s 应 401，实际 %d", path, rec.Code)
		}
	}
}

func TestBearerToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/research", nil)
	req.Header.Set("Authorization", "Bearer abc")
	if got := bearerToken(req); got != "abc" {
		t.Fatalf("解析失败：%q", got)
	}
	// 查询串 Token 只对 SSE 端点放行，避免 Token 出现在普通请求 URL 与日志里。
	req = httptest.NewRequest(http.MethodGet, "/api/v1/research/abc/events?token=q", nil)
	if got := bearerToken(req); got != "q" {
		t.Fatalf("SSE query token 解析失败：%q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/research?token=q", nil)
	if got := bearerToken(req); got != "" {
		t.Fatalf("非 SSE 端点不应接受 query token，实际 %q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/research", nil)
	if got := bearerToken(req); got != "" {
		t.Fatalf("无 Token 时应为空，实际 %q", got)
	}
	// header 优先于 query
	req = httptest.NewRequest(http.MethodGet, "/api/v1/research/x/events?token=q", nil)
	req.Header.Set("Authorization", "Bearer h")
	if got := bearerToken(req); got != "h" {
		t.Fatalf("header 应优先，实际 %q", got)
	}
}

func TestOwnerIsServerSide(t *testing.T) {
	// 身份只能来自服务端配置，绝不从请求体/查询串读取 user_id。
	s := &Server{Cfg: &config.Config{LocalUser: "local-user"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/research?user_id=attacker", nil)
	req.Header.Set("X-User-Id", "attacker")
	if got := s.owner(req); got != "local-user" {
		t.Fatalf("身份被客户端影响：%q", got)
	}
}
