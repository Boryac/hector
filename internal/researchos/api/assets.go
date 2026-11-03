package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/verikod/hector/internal/researchos/db"
	"github.com/verikod/hector/internal/researchos/domain"
	"github.com/verikod/hector/internal/researchos/report"
)

// handleHealth 健康检查：DB 断连必须如实反映，不允许一律返回 ok。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 5*time.Second)
	defer cancel()

	resp := map[string]any{
		"version":  s.Version,
		"uptime_s": int(time.Since(s.StartedAt).Seconds()),
		"time":     time.Now().UTC().Format(time.RFC3339),
	}
	status := "ok"

	dbCheck := map[string]any{"ok": false}
	if s.Pool != nil {
		start := time.Now()
		if err := s.Pool.Ping(ctx); err != nil {
			dbCheck["error"] = err.Error()
			status = "down"
		} else {
			dbCheck["ok"] = true
			dbCheck["latency_ms"] = time.Since(start).Milliseconds()
			if migs, err := db.AppliedMigrations(ctx, s.Pool); err == nil {
				dbCheck["migrations"] = migs
			} else {
				dbCheck["migration_error"] = err.Error()
				status = "degraded"
			}
			stats := s.Pool.Stat()
			dbCheck["pool"] = map[string]any{
				"total": stats.TotalConns(), "idle": stats.IdleConns(),
				"acquired": stats.AcquiredConns(),
			}
		}
	}
	resp["checks"] = map[string]any{
		"database": dbCheck,
		"llm": map[string]any{
			"enabled":  s.cfg().LLMEnabled(),
			"provider": string(s.cfg().LLMProvider),
			"model":    s.cfg().LLMModel,
			"note":     noteIf(!s.cfg().LLMEnabled(), "未配置 RESEARCHOS_LLM_API_KEY（或 RESEARCHOS_LLM_PROVIDER=none），规划与抽取降级为本地确定性流程"),
		},
		"search": map[string]any{
			"providers": s.searchEngine().Names(),
			"note":      noteIf(len(s.searchEngine().Names()) == 0, "未配置任何搜索 Provider"),
		},
		"auth": map[string]any{"mode": "single-user-local"},
	}
	resp["status"] = status

	code := http.StatusOK
	if status == "down" {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, resp)
}

func noteIf(cond bool, note string) string {
	if cond {
		return note
	}
	return ""
}

// ---------------------------------------------------------------------------
// Agent 执行
// ---------------------------------------------------------------------------

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	agents, err := s.Store.ListAgentExecutions(ctx, task.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": agents, "total": len(agents)})
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	row, err := s.Store.Q.GetAgentExecution(ctx, db.GetAgentExecutionParams{TaskID: task.ID, ID: r.PathValue("agent_id")})
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "Agent 执行记录不存在")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

// ---------------------------------------------------------------------------
// 资料与证据
// ---------------------------------------------------------------------------

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	limit := intQuery(r, "limit", 100)
	if limit > 500 {
		limit = 500
	}
	q := r.URL.Query()
	sources, err := s.Store.ListSources(ctx, task.ID, q.Get("site"), q.Get("credibility"), q.Get("q"), limit, intQuery(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": sources, "total": len(sources)})
}

func (s *Server) handleGetSource(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	src, err := s.Store.GetSource(ctx, task.ID, r.PathValue("source_id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "资料不存在")
		return
	}
	writeJSON(w, http.StatusOK, src)
}

func (s *Server) handleListEvidence(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	claims, err := s.Store.ListClaims(ctx, task.ID, r.URL.Query().Get("status"), intQuery(r, "limit", 100), intQuery(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": claims, "total": len(claims)})
}

func (s *Server) handleGetClaim(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	claim, err := s.Store.GetClaim(ctx, task.ID, r.PathValue("claim_id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "结论不存在")
		return
	}
	// 附带证据对应的资料详情，方便前端引用跳转。
	for i := range claim.Evidence {
		if src, err := s.Store.GetSource(ctx, task.ID, claim.Evidence[i].SourceID); err == nil {
			claim.Evidence[i].Source = src
		}
	}
	writeJSON(w, http.StatusOK, claim)
}

// ---------------------------------------------------------------------------
// 报告
// ---------------------------------------------------------------------------

func (s *Server) handleGetReport(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	version := intQuery(r, "version", 0)
	var rep *domain.Report
	if version > 0 {
		rep, err = s.Store.GetReportVersion(ctx, task.ID, version)
	} else {
		rep, err = s.Store.GetLatestReport(ctx, task.ID)
	}
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "报告尚未生成")
		return
	}
	versions, _ := s.Store.Q.ListReportVersions(ctx, task.ID)
	writeJSON(w, http.StatusOK, map[string]any{"report": rep, "versions": versions})
}

func (s *Server) handleExportMarkdown(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	rep, err := s.latestReport(ctx, task.ID, intQuery(r, "version", 0))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "报告尚未生成")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="report-%s-v%d.md"`, shortID(task.ID), rep.Version))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(rep.Markdown))
}

func (s *Server) handleExportHTML(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	rep, err := s.latestReport(ctx, task.ID, intQuery(r, "version", 0))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "报告尚未生成")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(report.RenderHTML(rep.Title, rep.Markdown)))
}

func (s *Server) latestReport(ctx context.Context, taskID string, version int) (*domain.Report, error) {
	if version > 0 {
		return s.Store.GetReportVersion(ctx, taskID, version)
	}
	return s.Store.GetLatestReport(ctx, taskID)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// ---------------------------------------------------------------------------
// 工具 / MCP
// ---------------------------------------------------------------------------

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	servers, err := s.Store.ListMCPServers(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	calls, _ := s.Store.ListToolCalls(ctx, r.URL.Query().Get("task_id"), 50)
	writeJSON(w, http.StatusOK, map[string]any{
		"servers": servers,
		"builtin": []map[string]any{
			{"name": "search", "description": "多 Provider 并发检索（GitHub / Hacker News / Wikipedia / Bing / Tavily / SerpAPI / Brave）", "risk": "low"},
			{"name": "fetch", "description": "抓取网页正文，遵守 robots.txt，限制大小与超时", "risk": "medium"},
			{"name": "llm", "description": "调用配置的 LLM 完成规划与结论抽取", "risk": "low"},
		},
		"calls": calls,
	})
}

func (s *Server) handleListToolCalls(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	calls, err := s.Store.ListToolCalls(ctx, r.URL.Query().Get("task_id"), intQuery(r, "limit", 100))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": calls, "total": len(calls)})
}

type mcpServerRequest struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// handleRegisterMCP 注册 MCP 服务（仅登记元数据，不自动拉起进程）。
//
// 高风险工具（写文件/执行命令/发起网络写操作）默认 approved=false，
// 必须由用户显式批准后才允许在后续版本中启动。
func (s *Server) handleRegisterMCP(w http.ResponseWriter, r *http.Request) {
	var req mcpServerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Command) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_body", "name 与 command 必填")
		return
	}
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	row, err := s.Store.UpsertMCPServer(ctx, uuid.NewString(), req.Name, req.Command, req.Args, req.Env, "stopped", false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "register_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"server": row,
		"note":   "已登记，未启动。启动需要显式批准（approved=true）",
	})
}
