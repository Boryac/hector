package api

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/verikod/hector/internal/researchos/config"
)

func TestAdminShutdownRequiresAuthAndTriggers(t *testing.T) {
	var fired int32
	cfg := config.Config{APIToken: "test-token-123"}
	s := NewServer(Options{Cfg: &cfg, Shutdown: func() { atomic.AddInt32(&fired, 1) }})

	// 无 Token → 401
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/shutdown", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无 Token 应 401，实际 %d", rec.Code)
	}
	if atomic.LoadInt32(&fired) != 0 {
		t.Fatal("未授权请求不得触发 Shutdown")
	}

	// 带正确 Token → 202 且触发
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/shutdown", nil)
	req.Header.Set("Authorization", "Bearer test-token-123")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("带 Token 应 202，实际 %d", rec.Code)
	}
	// Shutdown 在响应写出后的 goroutine 中触发，轮询等待。
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&fired) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&fired) != 1 {
		t.Fatal("授权请求应触发一次 Shutdown")
	}
}

func TestAdminShutdownDisabledReturns501(t *testing.T) {
	cfg := config.Config{APIToken: "test-token-123"}
	s := NewServer(Options{Cfg: &cfg}) // 未注入 Shutdown
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/shutdown", nil)
	req.Header.Set("Authorization", "Bearer test-token-123")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("未注入 Shutdown 应 501，实际 %d", rec.Code)
	}
}
