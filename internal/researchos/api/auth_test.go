package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/verikod/hector/internal/researchos/config"
)

// newAuthServer 构造只用于鉴权测试的 Server（不连数据库）。
func newAuthServer(token string) *Server {
	return &Server{Cfg: &config.Config{
		APIToken:     token,
		LocalUser:    "local-user",
		SSETicketTTL: time.Minute,
	}}
}

func newAuthedHandler(token string, next http.Handler) http.Handler {
	return newAuthServer(token).withAuth(next)
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestWithAuthRejectsMissingToken(t *testing.T) {
	h := newAuthedHandler("secret-token", okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/research", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("缺少 Token 应 401，实际 %d", rec.Code)
	}
}

func TestWithAuthRejectsWrongToken(t *testing.T) {
	h := newAuthedHandler("secret-token", okHandler())
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
	h := newAuthedHandler("secret-token", okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/research", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("正确 Token 应放行，实际 %d", rec.Code)
	}
}

// 安全回归：长期 API Token 不再接受查询参数传递。
//
// 回归用例：早期实现允许 `GET /events?token=<API Token>`，Token 会沉淀在浏览器历史、
// Referer 与反向代理 access log 中，且长期有效、可调用全部接口。
func TestWithAuthRejectsLongLivedQueryToken(t *testing.T) {
	h := newAuthedHandler("secret-token", okHandler())
	for _, path := range []string{
		"/api/v1/research/abc/events?token=secret-token",
		"/api/v1/research/abc/events?token=secret-token&format=json",
		"/api/v1/research?token=secret-token",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s：长期 Token 走查询串应被拒绝，实际 %d", path, rec.Code)
		}
	}
}

// SSE ticket 可以建立事件流，且一次性。
func TestWithAuthAcceptsSSETicketOnce(t *testing.T) {
	s := newAuthServer("secret-token")
	h := s.withAuth(okHandler())
	tk, _, err := s.tstore().Issue("task-1")
	if err != nil {
		t.Fatalf("签发失败：%v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/research/task-1/events?ticket="+tk, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ticket 应放行事件流，实际 %d", rec.Code)
	}
	// 重放同一张 ticket
	req = httptest.NewRequest(http.MethodGet, "/api/v1/research/task-1/events?ticket="+tk, nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("ticket 重放应 401，实际 %d", rec.Code)
	}
}

// ticket 不能用于其他任务。
func TestWithAuthRejectsTicketForOtherTask(t *testing.T) {
	s := newAuthServer("secret-token")
	h := s.withAuth(okHandler())
	tk, _, _ := s.tstore().Issue("task-1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/research/task-2/events?ticket="+tk, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("跨任务 ticket 应 403，实际 %d", rec.Code)
	}
}

// ticket 不能替代 Bearer Token 访问其他接口。
func TestWithAuthRejectsTicketOnOtherEndpoints(t *testing.T) {
	s := newAuthServer("secret-token")
	h := s.withAuth(okHandler())
	tk, _, _ := s.tstore().Issue("task-1")
	paths := []string{
		"/api/v1/research/task-1",
		"/api/v1/research/task-1/sources",
		"/api/v1/research/task-1/report.md",
		"/api/v1/settings",
	}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, p+"?ticket="+tk, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s：ticket 不应被接受为 API Token，实际 %d", p, rec.Code)
		}
	}
	// 上面的请求不应消耗 ticket：仍可正常用于事件流。
	req := httptest.NewRequest(http.MethodGet, "/api/v1/research/task-1/events?ticket="+tk, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ticket 被非事件接口消耗了，事件流应仍可用，实际 %d", rec.Code)
	}
}

func TestWithAuthAllowsHealth(t *testing.T) {
	h := newAuthedHandler("secret-token", okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("健康检查应免鉴权，实际 %d", rec.Code)
	}
}

func TestWithAuthAllowsStaticAssets(t *testing.T) {
	h := newAuthedHandler("secret-token", okHandler())
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
	h := newAuthedHandler("secret-token", okHandler())
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
	req = httptest.NewRequest(http.MethodGet, "/api/v1/research", nil)
	req.Header.Set("X-API-Token", "h")
	if got := bearerToken(req); got != "h" {
		t.Fatalf("X-API-Token 解析失败：%q", got)
	}
	// 查询串一律不参与凭据解析（含 SSE 端点）。
	req = httptest.NewRequest(http.MethodGet, "/api/v1/research/abc/events?token=q", nil)
	if got := bearerToken(req); got != "" {
		t.Fatalf("查询串不应被当作凭据，实际 %q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/research?token=q", nil)
	if got := bearerToken(req); got != "" {
		t.Fatalf("非 SSE 端点不应接受 query token，实际 %q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/research", nil)
	if got := bearerToken(req); got != "" {
		t.Fatalf("无 Token 时应为空，实际 %q", got)
	}
	// header 优先于查询串（查询串本来就不参与）
	req = httptest.NewRequest(http.MethodGet, "/api/v1/research/x/events?ticket=t", nil)
	req.Header.Set("Authorization", "Bearer h")
	if got := bearerToken(req); got != "h" {
		t.Fatalf("header 应被读到，实际 %q", got)
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

func TestEventsTaskID(t *testing.T) {
	cases := []struct {
		path string
		id   string
		ok   bool
	}{
		{"/api/v1/research/abc/events", "abc", true},
		{"/api/v1/research/abc/events/", "", false},
		{"/api/v1/research/abc/sources", "", false},
		{"/api/v1/research/abc/events/ticket", "", false},
		{"/api/v1/research/events", "", false},
		{"/other/abc/events", "", false},
	}
	for _, c := range cases {
		id, ok := eventsTaskID(c.path)
		if ok != c.ok || id != c.id {
			t.Fatalf("eventsTaskID(%q) = (%q,%v)，期望 (%q,%v)", c.path, id, ok, c.id, c.ok)
		}
	}
}
