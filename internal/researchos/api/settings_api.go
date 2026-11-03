package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/verikod/hector/internal/researchos/config"
	"github.com/verikod/hector/internal/researchos/db"
	"github.com/verikod/hector/internal/researchos/llm"
	"github.com/verikod/hector/internal/researchos/search"
	"github.com/verikod/hector/internal/researchos/settings"
)

// ---------------------------------------------------------------------------
// 设置中心
// ---------------------------------------------------------------------------

// handleGetSettings 返回分组配置。
//
// 敏感项只返回掩码与 configured 标记，任何情况下都不返回明文——
// 这条约束由 secrets.Mask 与 settings.Snapshot 保证，并有测试守着。
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeErr(w, http.StatusServiceUnavailable, "settings_unavailable", "设置中心未启用")
		return
	}
	writeJSON(w, http.StatusOK, s.Settings.Snapshot())
}

// handlePutSection 保存某个分组的若干项。
//
// 请求体既接受扁平形式 {"model.provider":"openai"}，也接受 {"values":{...}}，
// 前者省事、后者语义更明确，两种都收下不增加前端心智负担。
func (s *Server) handlePutSection(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeErr(w, http.StatusServiceUnavailable, "settings_unavailable", "设置中心未启用")
		return
	}
	section := r.PathValue("section")

	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if nested, ok := body["values"].(map[string]any); ok {
		body = nested
	}
	if len(body) == 0 {
		writeErr(w, http.StatusBadRequest, "empty_patch", "没有提交任何配置项")
		return
	}

	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()

	res, err := s.Settings.Patch(ctx, section, body)
	if err != nil {
		switch {
		case errors.Is(err, settings.ErrDeployField):
			writeErr(w, http.StatusForbidden, "deploy_field_readonly", err.Error())
		case errors.Is(err, settings.ErrUnknownSetting):
			writeErr(w, http.StatusUnprocessableEntity, "unknown_setting", err.Error())
		default:
			// 校验类错误是用户输入问题，用 422 而不是 500。
			writeErr(w, http.StatusUnprocessableEntity, "invalid_value", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"result":   res,
		"settings": s.Settings.Snapshot(),
	})
}

// handleClearSetting 清除一项 Web 覆盖，使其回落到环境变量或默认值。
func (s *Server) handleClearSetting(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeErr(w, http.StatusServiceUnavailable, "settings_unavailable", "设置中心未启用")
		return
	}
	section := r.PathValue("section")
	key := r.PathValue("key")

	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()

	res, err := s.Settings.Patch(ctx, section, map[string]any{key: "__clear__"})
	if err != nil {
		switch {
		case errors.Is(err, settings.ErrDeployField):
			writeErr(w, http.StatusForbidden, "deploy_field_readonly", err.Error())
		default:
			writeErr(w, http.StatusUnprocessableEntity, "invalid_value", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res, "settings": s.Settings.Snapshot()})
}

// llmTestRequest 允许用表单值试连，不必先保存。
type llmTestRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
}

// handleTestLLM 发起一次真实的最小补全请求，验证配置是否可用。
//
// 不写入任何设置；api_key 允许留空（表示沿用已保存的值），
// 也允许传掩码（前端把 GET 拿到的掩码原样回传，不能当真的 Key 用）。
func (s *Server) handleTestLLM(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeErr(w, http.StatusServiceUnavailable, "settings_unavailable", "设置中心未启用")
		return
	}
	var req llmTestRequest
	// 允许空请求体：直接测试当前生效配置。
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
	}

	rt := s.Settings.Runtime()
	cfg := rt.Cfg
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider != "" {
		cfg.LLMProvider = config.LLMProviderKind(provider)
	}
	if v := strings.TrimSpace(req.APIKey); v != "" && !isMasked(v) {
		cfg.LLMAPIKey = v
	}
	if v := strings.TrimSpace(req.BaseURL); v != "" {
		cfg.LLMBaseURL = v
	}
	if v := strings.TrimSpace(req.Model); v != "" {
		cfg.LLMModel = v
	}

	if cfg.LLMProvider == config.LLMNone {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "reason": "provider_none",
			"message": "当前 Provider 为 none（不使用大模型）。要测试连接请先选择 openai / deepseek / ollama。",
		})
		return
	}

	// 试连超时取配置值与 30s 的较小者：界面点一下等 90 秒不合适。
	timeout := cfg.LLMTimeout
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	client := llm.New(llm.Config{
		Provider: string(cfg.LLMProvider), APIKey: cfg.LLMAPIKey,
		Model: cfg.LLMModel, BaseURL: cfg.LLMBaseURL, Timeout: timeout,
	})
	if !client.Available() {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "reason": "missing_credentials",
			"message": "缺少 API Key，无法发起真实请求",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout+2*time.Second)
	defer cancel()
	start := time.Now()
	out, usage, err := client.Complete(ctx, "你是连通性测试助手。只回复 OK 两个字符，不要解释。", "ping")
	elapsed := time.Since(start).Milliseconds()

	resp := map[string]any{
		"provider":   string(cfg.LLMProvider),
		"model":      client.Model(),
		"base_url":   cfg.LLMBaseURL,
		"latency_ms": elapsed,
		"usage":      usage,
	}
	if err != nil {
		resp["ok"] = false
		resp["reason"] = "request_failed"
		resp["message"] = truncateForUI(err.Error(), 400)
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["ok"] = true
	resp["reply"] = truncateForUI(strings.TrimSpace(out), 80)
	resp["message"] = "连接正常"
	writeJSON(w, http.StatusOK, resp)
}

// searchTestRequest 指定要测试的 Provider（省略则测全部已启用的）。
type searchTestRequest struct {
	Provider string `json:"provider"`
	Query    string `json:"query"`
}

// handleTestSearch 用真实关键词打一次检索，返回每个 Provider 的结果数与耗时。
func (s *Server) handleTestSearch(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeErr(w, http.StatusServiceUnavailable, "settings_unavailable", "设置中心未启用")
		return
	}
	var req searchTestRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		query = "vector database benchmark"
	}

	cfg := s.Settings.Runtime().Cfg
	opt := search.ProviderOptions{
		UserAgent: cfg.UserAgent, Timeout: cfg.SearchTimeout,
		TavilyKey: cfg.TavilyAPIKey, SerpAPIKey: cfg.SerpAPIKey,
		BraveKey: cfg.BraveAPIKey, GitHubToken: cfg.GitHubToken,
	}
	names := cfg.SearchProviders
	if p := strings.ToLower(strings.TrimSpace(req.Provider)); p != "" {
		names = []string{p}
	}
	if len(names) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "reason": "no_provider", "message": "没有启用任何搜索 Provider",
		})
		return
	}

	engine, unknown := search.NewDefault(names, opt)
	ctx, cancel := context.WithTimeout(r.Context(), cfg.SearchTimeout+2*time.Second)
	defer cancel()
	start := time.Now()
	results, traces := engine.SearchAll(ctx, query, 3)
	elapsed := time.Since(start).Milliseconds()

	perProvider := make([]map[string]any, 0, len(traces))
	anyOK := false
	for _, t := range traces {
		item := map[string]any{
			"provider":   t.Provider,
			"ok":         t.Error == "",
			"count":      t.Count,
			"latency_ms": t.DurationMS,
		}
		if t.Error != "" {
			item["error"] = truncateForUI(t.Error, 300)
		} else {
			anyOK = true
		}
		perProvider = append(perProvider, item)
	}

	resp := map[string]any{
		"ok":         anyOK,
		"query":      query,
		"total":      len(results),
		"latency_ms": elapsed,
		"providers":  perProvider,
		"message":    "检索可用",
	}
	if len(unknown) > 0 {
		resp["unknown"] = unknown
	}
	if !anyOK {
		resp["message"] = "所有 Provider 均失败，请检查密钥与网络"
	}
	writeJSON(w, http.StatusOK, resp)
}

// isMasked 判断前端回传的是不是掩码。
func isMasked(v string) bool {
	return strings.Contains(v, "****")
}

func truncateForUI(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ---------------------------------------------------------------------------
// 系统信息与统计
// ---------------------------------------------------------------------------

// handleSystemInfo 返回运行环境、数据库、服务与版本信息。
//
// 与 /health 的区别：health 面向探活（只关心活没活），这里面向「设置页要知道
// 自己在什么环境里跑」，因此包含平台、归档版本、配置来源等信息。
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 5*time.Second)
	defer cancel()

	cfg := s.cfg()
	dbInfo := map[string]any{"ok": false}
	if s.Pool != nil {
		start := time.Now()
		if err := s.Pool.Ping(ctx); err != nil {
			dbInfo["ok"] = false
			dbInfo["error"] = truncateForUI(err.Error(), 300)
		} else {
			dbInfo["ok"] = true
			dbInfo["latency_ms"] = time.Since(start).Milliseconds()
			dbInfo["dsn"] = config.RedactDSN(cfg.DatabaseURL)
			if migs, err := db.AppliedMigrations(ctx, s.Pool); err == nil {
				dbInfo["migrations"] = migs
			}
			stats := s.Pool.Stat()
			dbInfo["pool"] = map[string]any{
				"total": stats.TotalConns(), "idle": stats.IdleConns(),
				"acquired": stats.AcquiredConns(), "max": stats.MaxConns(),
			}
		}
	}

	rt := s.runtime()
	llmInfo := map[string]any{
		"enabled":  cfg.LLMEnabled(),
		"provider": string(cfg.LLMProvider),
		"model":    cfg.LLMModel,
		"base_url": cfg.LLMBaseURL,
		"key_set":  strings.TrimSpace(cfg.LLMAPIKey) != "",
	}
	searchInfo := map[string]any{
		"configured": cfg.SearchProviders,
		"available":  nil,
	}
	if rt != nil {
		searchInfo["available"] = rt.Search.AvailableNames()
		llmInfo["model"] = rt.LLM.Model()
	}

	resp := map[string]any{
		"version":    s.Version,
		"uptime_s":   int(time.Since(s.StartedAt).Seconds()),
		"started_at": s.StartedAt.UTC().Format(time.RFC3339),
		"time":       time.Now().UTC().Format(time.RFC3339),
		"runtime": map[string]any{
			"go":         runtime.Version(),
			"os":         runtime.GOOS,
			"arch":       runtime.GOARCH,
			"cpus":       runtime.NumCPU(),
			"goroutines": runtime.NumGoroutine(),
		},
		"service": map[string]any{
			"addr":     cfg.Addr,
			"web_dir":  cfg.WebDir,
			"data_dir": cfg.DataDir,
			"auth":     "single-user-local",
			"user":     cfg.LocalUser,
		},
		"database": dbInfo,
		"llm":      llmInfo,
		"search":   searchInfo,
		"settings": map[string]any{
			"enabled":        s.Settings != nil,
			"key_source":     keySource(s),
			"revision":       revision(s),
			"restart_needed": restartNeeded(s),
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func keySource(s *Server) string {
	if s.Settings == nil {
		return ""
	}
	return s.Settings.KeySource()
}

func revision(s *Server) int64 {
	if rt := s.runtime(); rt != nil {
		return rt.Revision
	}
	return 0
}

func restartNeeded(s *Server) []string {
	if s.Settings == nil {
		return nil
	}
	return s.Settings.Snapshot().PendingRestart
}

// handleStats 汇总任务、节点、证据与 Agent 的执行情况，供 Dashboard 使用。
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()

	if s.Pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "no_database", "数据库不可用")
		return
	}

	tasksByStatus := map[string]int{}
	if rows, err := s.Pool.Query(ctx, `SELECT status, count(*) FROM research_tasks WHERE created_by=$1 GROUP BY status`, s.cfg().LocalUser); err == nil {
		defer rows.Close()
		for rows.Next() {
			var st string
			var n int
			if err := rows.Scan(&st, &n); err == nil {
				tasksByStatus[st] = n
			}
		}
	}

	countOf := func(q string, args ...any) int {
		var n int
		if err := s.Pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			return 0
		}
		return n
	}

	nodesByStatus := map[string]int{}
	if rows, err := s.Pool.Query(ctx, `
		SELECT n.status, count(*) FROM workflow_nodes n
		JOIN research_tasks t ON t.id = n.task_id
		WHERE t.created_by=$1 GROUP BY n.status`, s.cfg().LocalUser); err == nil {
		defer rows.Close()
		for rows.Next() {
			var st string
			var n int
			if err := rows.Scan(&st, &n); err == nil {
				nodesByStatus[st] = n
			}
		}
	}

	agentsByRole := map[string]int{}
	if rows, err := s.Pool.Query(ctx, `
		SELECT a.role, count(*) FROM agent_executions a
		JOIN research_tasks t ON t.id = a.task_id
		WHERE t.created_by=$1 GROUP BY a.role ORDER BY count(*) DESC LIMIT 12`, s.cfg().LocalUser); err == nil {
		defer rows.Close()
		for rows.Next() {
			var role string
			var n int
			if err := rows.Scan(&role, &n); err == nil {
				agentsByRole[role] = n
			}
		}
	}

	recent := make([]map[string]any, 0, 8)
	if rows, err := s.Pool.Query(ctx, `
		SELECT id, title, status, depth, planner_mode, created_at, finished_at
		FROM research_tasks WHERE created_by=$1 ORDER BY created_at DESC LIMIT 8`, s.cfg().LocalUser); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, title, status, depth, planner string
			var createdAt time.Time
			var finishedAt *time.Time
			if err := rows.Scan(&id, &title, &status, &depth, &planner, &createdAt, &finishedAt); err == nil {
				item := map[string]any{
					"id": id, "title": title, "status": status, "depth": depth,
					"planner_mode": planner, "created_at": createdAt.UTC().Format(time.RFC3339),
				}
				if finishedAt != nil {
					item["finished_at"] = finishedAt.UTC().Format(time.RFC3339)
				}
				recent = append(recent, item)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tasks": map[string]any{
			"by_status": tasksByStatus,
			"total":     sumInts(tasksByStatus),
		},
		"nodes":   map[string]any{"by_status": nodesByStatus, "total": sumInts(nodesByStatus)},
		"agents":  map[string]any{"by_role": agentsByRole},
		"sources": countOf(`SELECT count(*) FROM sources s JOIN research_tasks t ON t.id=s.task_id WHERE t.created_by=$1`, s.cfg().LocalUser),
		"claims":  countOf(`SELECT count(*) FROM claims c JOIN research_tasks t ON t.id=c.task_id WHERE t.created_by=$1`, s.cfg().LocalUser),
		"reports": countOf(`SELECT count(*) FROM reports r JOIN research_tasks t ON t.id=r.task_id WHERE t.created_by=$1`, s.cfg().LocalUser),
		"recent":  recent,
		"llm": map[string]any{
			"enabled":  s.cfg().LLMEnabled(),
			"provider": string(s.cfg().LLMProvider),
			"model":    llmModel(s),
		},
	})
}

func llmModel(s *Server) string {
	if rt := s.runtime(); rt != nil && rt.LLM != nil {
		return rt.LLM.Model()
	}
	return s.cfg().LLMModel
}

func sumInts(m map[string]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

var _ = fmt.Sprintf
