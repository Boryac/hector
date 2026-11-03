package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/verikod/hector/internal/researchos/mcp"
)

// ---------------------------------------------------------------------------
// MCP Hub
// ---------------------------------------------------------------------------

// handleListMCPServers 列出已登记的 MCP Server 及其工具缓存。
func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	servers, err := s.Store.ListMCPServers(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	items := make([]map[string]any, 0, len(servers))
	for _, sv := range servers {
		var tools []mcp.Tool
		if len(sv.Tools) > 0 {
			_ = json.Unmarshal(sv.Tools, &tools)
		}
		items = append(items, map[string]any{
			"id": sv.ID, "name": sv.Name, "command": sv.Command,
			"args": sv.Args, "status": sv.Status, "approved": sv.Approved,
			"tools": tools, "tool_count": len(tools),
			"last_error": sv.LastError, "created_at": sv.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// handleDiscoverMCPTools 真实启动 Server 并取回工具清单。
//
// 这是唯一会执行外部命令的接口，因此必须显式批准之后才允许调用：
// 未批准的 Server 只能「登记」，不能「运行」。
func (s *Server) handleDiscoverMCPTools(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 30*time.Second)
	defer cancel()

	id := r.PathValue("id")
	servers, err := s.Store.ListMCPServers(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	var target *struct {
		Name, Command string
		Args          []string
		Env           map[string]string
		Approved      bool
	}
	for _, sv := range servers {
		if sv.ID != id {
			continue
		}
		var args []string
		_ = json.Unmarshal(sv.Args, &args)
		var env map[string]string
		_ = json.Unmarshal(sv.Env, &env)
		target = &struct {
			Name, Command string
			Args          []string
			Env           map[string]string
			Approved      bool
		}{Name: sv.Name, Command: sv.Command, Args: args, Env: env, Approved: sv.Approved}
	}
	if target == nil {
		writeErr(w, http.StatusNotFound, "not_found", "MCP Server 不存在")
		return
	}
	if !target.Approved {
		writeErr(w, http.StatusConflict, "not_approved",
			"该 Server 尚未批准，不能启动进程做工具发现。请先批准。")
		return
	}

	res, discoverErr := mcp.Discover(ctx, target.Command, target.Args, target.Env, 20*time.Second)
	if discoverErr != nil {
		// 失败也要落状态，否则界面上看不出「试过但失败了」。
		_ = s.Store.UpdateMCPServerTools(ctx, id, json.RawMessage("[]"), "error", discoverErr.Error())
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"reason":  "discover_failed",
			"message": truncateForUI(discoverErr.Error(), 500),
		})
		return
	}

	toolsJSON, _ := json.Marshal(res.Tools)
	if err := s.Store.UpdateMCPServerTools(ctx, id, toolsJSON, "ready", ""); err != nil {
		writeErr(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"tools":            res.Tools,
		"tool_count":       len(res.Tools),
		"protocol_version": res.ProtocolVersion,
		"server_info":      res.ServerInfo,
		"duration_ms":      res.DurationMS,
		"message":          "发现完成",
	})
}

// handleApproveMCP 批准或撤销批准。
//
// 批准 = 允许启动该外部进程；撤销批准时一并停掉，不留还在跑的后台进程。
func (s *Server) handleApproveMCP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Approved bool `json:"approved"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()

	status := "stopped"
	if body.Approved {
		status = "approved"
	}
	if err := s.Store.SetMCPServerApproved(ctx, r.PathValue("id"), body.Approved, status); err != nil {
		writeErr(w, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": r.PathValue("id"), "approved": body.Approved, "status": status,
		"message": map[bool]string{true: "已批准，可以启动并做工具发现", false: "已撤销批准并停止"}[body.Approved],
	})
}

// handleDeleteMCP 删除登记。
func (s *Server) handleDeleteMCP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	if err := s.Store.DeleteMCPServer(ctx, r.PathValue("id")); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleStartMCPServer 启动/停止（启动前必须有批准）。
func (s *Server) handleStartMCPServer(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	id := r.PathValue("id")
	action := r.PathValue("action")

	servers, err := s.Store.ListMCPServers(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	var approved bool
	found := false
	for _, sv := range servers {
		if sv.ID == id {
			approved, found = sv.Approved, true
		}
	}
	if !found {
		writeErr(w, http.StatusNotFound, "not_found", "MCP Server 不存在")
		return
	}

	switch action {
	case "start":
		if !approved {
			writeErr(w, http.StatusConflict, "not_approved", "未批准的 Server 不能启动")
			return
		}
		if err := s.Store.SetMCPServerStatus(ctx, id, "running"); err != nil {
			writeErr(w, http.StatusInternalServerError, "update_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": id, "status": "running",
			"note": "已置为 running。ResearchOS 不会常驻托管外部进程，工具清单请用 discover 获取。",
		})
	case "stop":
		if err := s.Store.SetMCPServerStatus(ctx, id, "stopped"); err != nil {
			writeErr(w, http.StatusInternalServerError, "update_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "stopped"})
	default:
		writeErr(w, http.StatusNotFound, "not_found", "未知操作")
	}
}

// ---------------------------------------------------------------------------
// 跨任务的来源与报告
// ---------------------------------------------------------------------------

// handleListAllSources 跨任务查来源，供 Sources 页面使用。
func (s *Server) handleListAllSources(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	if s.Pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "no_database", "数据库不可用")
		return
	}

	q := r.URL.Query()
	limit := intQuery(r, "limit", 50)
	if limit > 200 {
		limit = 200
	}
	offset := intQuery(r, "offset", 0)

	where := []string{"t.created_by = $1"}
	args := []any{s.cfg().LocalUser}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.Replace(cond, "?", "$"+itoa(len(args)), 1))
	}
	if v := strings.TrimSpace(q.Get("task_id")); v != "" {
		add("s.task_id = ?", v)
	}
	if v := strings.TrimSpace(q.Get("site")); v != "" {
		add("s.site = ?", v)
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		args = append(args, "%"+strings.ToLower(v)+"%")
		n := len(args)
		where = append(where, "(lower(s.title) LIKE $"+itoa(n)+" OR lower(s.url) LIKE $"+itoa(n)+" OR lower(s.snippet) LIKE $"+itoa(n)+")")
	}
	if v := strings.TrimSpace(q.Get("status")); v != "" {
		add("s.status = ?", v)
	}

	sql := `SELECT s.id, s.task_id, t.title, s.url, s.title, s.site, s.provider, s.status,
	               s.credibility, s.http_status, s.created_at,
	               left(s.snippet, 300) AS snippet
	        FROM sources s JOIN research_tasks t ON t.id = s.task_id
	        WHERE ` + strings.Join(where, " AND ") + `
	        ORDER BY s.created_at DESC LIMIT ` + itoa(limit) + ` OFFSET ` + itoa(offset)

	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id, taskID, taskTitle, url, title, site, provider, status, credibility, snippet string
		var httpStatus int
		var createdAt time.Time
		if err := rows.Scan(&id, &taskID, &taskTitle, &url, &title, &site, &provider, &status,
			&credibility, &httpStatus, &createdAt, &snippet); err != nil {
			writeErr(w, http.StatusInternalServerError, "scan_failed", err.Error())
			return
		}
		items = append(items, map[string]any{
			"id": id, "task_id": taskID, "task_title": taskTitle, "url": url,
			"title": title, "site": site, "provider": provider, "status": status,
			"credibility": credibility, "http_status": httpStatus,
			"snippet": snippet, "created_at": createdAt.UTC().Format(time.RFC3339),
		})
	}

	var total int
	countSQL := `SELECT count(*) FROM sources s JOIN research_tasks t ON t.id = s.task_id WHERE ` + strings.Join(where, " AND ")
	_ = s.Pool.QueryRow(ctx, countSQL, args...).Scan(&total)

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

// handleListAllReports 跨任务查报告。
func (s *Server) handleListAllReports(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	if s.Pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "no_database", "数据库不可用")
		return
	}
	limit := intQuery(r, "limit", 50)
	if limit > 200 {
		limit = 200
	}
	offset := intQuery(r, "offset", 0)

	rows, err := s.Pool.Query(ctx, `
		SELECT r.id, r.task_id, t.title, r.version, r.title, r.citation_count, r.word_count,
		       r.model, r.created_at
		FROM reports r JOIN research_tasks t ON t.id = r.task_id
		WHERE t.created_by = $1
		ORDER BY r.created_at DESC LIMIT $2 OFFSET $3`, s.cfg().LocalUser, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id, taskID, taskTitle, title, model string
		var version, citations, words int
		var createdAt time.Time
		if err := rows.Scan(&id, &taskID, &taskTitle, &version, &title, &citations, &words, &model, &createdAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "scan_failed", err.Error())
			return
		}
		items = append(items, map[string]any{
			"id": id, "task_id": taskID, "task_title": taskTitle, "version": version,
			"title": title, "citation_count": citations, "word_count": words,
			"model": model, "created_at": createdAt.UTC().Format(time.RFC3339),
		})
	}

	var total int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM reports r JOIN research_tasks t ON t.id=r.task_id WHERE t.created_by=$1`, s.cfg().LocalUser).Scan(&total)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

// handleListRunningAgents 返回最近活跃的 Agent 执行记录（Dashboard 用）。
func (s *Server) handleListRunningAgents(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	if s.Pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "no_database", "数据库不可用")
		return
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT a.id, a.task_id, t.title, a.role, a.name, a.model, a.status,
		       a.started_at, a.duration_ms
		FROM agent_executions a JOIN research_tasks t ON t.id = a.task_id
		WHERE t.created_by = $1
		ORDER BY a.started_at DESC LIMIT $2`, s.cfg().LocalUser, intQuery(r, "limit", 20))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0, 20)
	for rows.Next() {
		var id, taskID, taskTitle, role, name, model, status string
		var startedAt time.Time
		var durationMS int
		if err := rows.Scan(&id, &taskID, &taskTitle, &role, &name, &model, &status, &startedAt, &durationMS); err != nil {
			writeErr(w, http.StatusInternalServerError, "scan_failed", err.Error())
			return
		}
		items = append(items, map[string]any{
			"id": id, "task_id": taskID, "task_title": taskTitle, "role": role,
			"name": name, "model": model, "status": status,
			"started_at": startedAt.UTC().Format(time.RFC3339), "duration_ms": durationMS,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

var _ = errors.Is
