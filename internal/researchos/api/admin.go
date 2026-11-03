package api

import (
	"net/http"
)

// handleAdminShutdown 供托盘管理程序等受控本地管理方优雅停止服务。
// 位于 withAuth 之后，需要与普通 API 相同的 Bearer Token；
// 触发的是与 Ctrl+C 完全相同的优雅关闭路径（先停接入、再排空 worker）。
// 未注入 Shutdown 回调（如部分单测）时返回 501，而不是假装成功。
func (s *Server) handleAdminShutdown(w http.ResponseWriter, r *http.Request) {
	if s.Shutdown == nil {
		http.Error(w, `{"error":"shutdown 未启用"}`, http.StatusNotImplemented)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"shutting_down"}`))
	// 延迟到响应写出之后触发，避免客户端读到连接重置。
	go s.Shutdown()
}
