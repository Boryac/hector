package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// RC1 安全修复回归：CORS 只对同源 Origin 回显，任意 Origin 不再被反射。
func TestWithCORSRestrictsToSameOrigin(t *testing.T) {
	handler := withCORS("127.0.0.1:8080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name   string
		origin string
		host   string
		want   string
	}{
		{"同源回显", "http://127.0.0.1:8080", "127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"跨源不回显", "http://evil.example", "127.0.0.1:8080", ""},
		{"大小写端口不匹配不回显", "http://127.0.0.1:9999", "127.0.0.1:8080", ""},
		{"畸形 Origin 不回显", "::::not-a-url", "127.0.0.1:8080", ""},
		{"无 Origin 无头", "", "127.0.0.1:8080", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			got := rec.Header().Get("Access-Control-Allow-Origin")
			if got != tc.want {
				t.Fatalf("ACAO = %q, want %q", got, tc.want)
			}
		})
	}

	// 跨源预检也不得携带放行头
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/research", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("跨源 OPTIONS 不得回显 ACAO")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS 应返回 204，实际 %d", rec.Code)
	}
}
