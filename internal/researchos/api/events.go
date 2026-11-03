package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/verikod/hector/internal/researchos/domain"
)

// handleEvents 同时支持两种模式：
//   - text/event-stream（默认）：实时推送，支持 Last-Event-ID 与 ?after_sequence= 续接；
//   - application/json（?format=json 或 Accept: application/json）：一次性重放，便于调试与测试。
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}

	after := eventCursor(r)
	if wantsJSON(r) {
		events, err := s.Store.ListEventsAfter(ctx, task.ID, after, 500)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "events_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": events, "latest_sequence": task.LatestSequence, "task_status": task.Status,
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "no_flusher", "当前连接不支持 SSE")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, ": connected task=%s after=%d\n\n", task.ID, after)
	flusher.Flush()

	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	refresh := time.NewTicker(2 * time.Second)
	defer refresh.Stop()

	status := task.Status
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			events, err := s.Store.ListEventsAfter(r.Context(), task.ID, after, 200)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", jsonString(err.Error()))
				flusher.Flush()
				return
			}
			for _, ev := range events {
				payload, _ := json.Marshal(ev)
				fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Sequence, ev.Type, payload)
				after = ev.Sequence
			}
			if len(events) > 0 {
				flusher.Flush()
			}
		case <-refresh.C:
			if t, err := s.Store.GetTaskRow(r.Context(), task.ID); err == nil {
				status = t.Status
			}
			if isTerminal(status) {
				// 排空剩余事件后结束流，避免客户端永久挂起。
				events, _ := s.Store.ListEventsAfter(r.Context(), task.ID, after, 200)
				for _, ev := range events {
					payload, _ := json.Marshal(ev)
					fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Sequence, ev.Type, payload)
					after = ev.Sequence
				}
				fmt.Fprintf(w, "event: stream.end\ndata: {\"status\":%s}\n\n", jsonString(status))
				flusher.Flush()
				return
			}
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func eventCursor(r *http.Request) int64 {
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return int64(intQuery(r, "after_sequence", 0))
}

func wantsJSON(r *http.Request) bool {
	if r.URL.Query().Get("format") == "json" {
		return true
	}
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func isTerminal(status string) bool {
	switch status {
	case domain.TaskCompleted, domain.TaskFailed, domain.TaskCancelled:
		return true
	}
	return false
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ---------------------------------------------------------------------------
// 通用查询参数
// ---------------------------------------------------------------------------

func intQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
