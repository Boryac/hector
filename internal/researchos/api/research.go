package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/verikod/hector/internal/researchos/dag"
	"github.com/verikod/hector/internal/researchos/domain"
	"github.com/verikod/hector/internal/researchos/planner"
)

type createResearchRequest struct {
	Goal               string         `json:"goal"`
	Title              string         `json:"title"`
	Depth              string         `json:"depth"`
	Scope              map[string]any `json:"scope"`
	MaxAgents          int            `json:"max_agents"`
	MaxDurationSeconds int            `json:"max_duration_seconds"`
	EnableVerification *bool          `json:"enable_verification"`
	PlanNow            *bool          `json:"plan_now"`
}

func (s *Server) handleCreateResearch(w http.ResponseWriter, r *http.Request) {
	var req createResearchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body", "请求体不是合法 JSON："+err.Error())
		return
	}
	req.Goal = strings.TrimSpace(req.Goal)
	if len([]rune(req.Goal)) < 4 {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_goal", "研究目标至少需要 4 个字符")
		return
	}
	depth := req.Depth
	switch depth {
	case "", "standard":
		depth = "standard"
	case "quick", "deep":
	default:
		writeErr(w, http.StatusUnprocessableEntity, "invalid_depth", "depth 只能是 quick / standard / deep")
		return
	}
	enableVerify := true
	if req.EnableVerification != nil {
		enableVerify = *req.EnableVerification
	}
	maxAgents := req.MaxAgents
	if maxAgents <= 0 {
		maxAgents = s.Cfg.MaxAgents
	}
	if maxAgents > 8 {
		maxAgents = 8
	}
	maxDuration := req.MaxDurationSeconds
	if maxDuration <= 0 {
		maxDuration = int(s.Cfg.TaskTimeout.Seconds())
	}

	task := &domain.Task{
		ID:                 uuid.NewString(),
		Title:              strings.TrimSpace(req.Title),
		Goal:               req.Goal,
		Depth:              depth,
		Scope:              req.Scope,
		MaxAgents:          maxAgents,
		MaxDurationSeconds: maxDuration,
		EnableVerification: enableVerify,
		Status:             domain.TaskCreated,
		CreatedBy:          s.owner(r),
		PlannerMode:        s.Cfg.PlannerMode(),
	}
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	if err := s.Store.CreateTask(ctx, task); err != nil {
		writeErr(w, http.StatusInternalServerError, "create_failed", "创建任务失败："+err.Error())
		return
	}

	planNow := true
	if req.PlanNow != nil {
		planNow = *req.PlanNow
	}
	if planNow {
		go s.planAsync(task.ID, goalOf(task), depth, enableVerify)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"task":             task,
		"workflow":         nil,
		"planning_started": planNow,
		"next":             "GET /api/v1/research/" + task.ID + " 查看规划结果，批准后执行",
	})
}

func goalOf(t *domain.Task) string { return t.Goal }

// planAsync 异步规划：状态机 planning → awaiting_approval/running，失败则 failed 并保留可读错误。
func (s *Server) planAsync(taskID, goal, depth string, verify bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	task, err := s.Store.GetTaskRow(ctx, taskID)
	if err != nil {
		slog.Error("plan: load task failed", "task", taskID, "err", err)
		return
	}
	if task.Status != domain.TaskCreated && task.Status != domain.TaskAwaitingApproval && task.Status != domain.TaskFailed {
		slog.Warn("plan: task 状态不允许规划", "task", taskID, "status", task.Status)
		return
	}
	if _, err := s.Store.TransitionTask(ctx, taskID, domain.TaskPlanning, domain.EvtResearchPlanning, nil); err != nil {
		slog.Error("plan: set planning failed", "err", err)
		return
	}

	plan, usage, perr := s.Planner.Plan(ctx, goal, planner.Options{
		Depth: depth, EnableVerification: verify, MaxQuestions: questionsForDepth(depth), Scope: task.Scope,
	})
	if perr != nil {
		_ = s.Store.SetTaskPlan(ctx, taskID, nil, s.Cfg.PlannerMode(), perr.Error())
		_ = s.Store.SetTaskError(ctx, taskID, "规划失败："+perr.Error())
		_, _ = s.Store.TransitionTask(ctx, taskID, domain.TaskFailed, domain.EvtResearchFailed,
			map[string]any{"phase": "planning", "error": perr.Error()})
		slog.Error("plan failed", "task", taskID, "err", perr)
		return
	}

	if plan.Goal == "" {
		plan.Goal = goal
	}
	if len(plan.ResearchQuestions) == 0 {
		_ = s.Store.SetTaskPlan(ctx, taskID, nil, s.Cfg.PlannerMode(), "计划缺少研究问题")
		_, _ = s.Store.TransitionTask(ctx, taskID, domain.TaskFailed, domain.EvtResearchFailed,
			map[string]any{"phase": "planning", "error": "计划缺少研究问题"})
		return
	}
	if err := s.Store.SetTaskPlan(ctx, taskID, plan, s.Cfg.PlannerMode(), ""); err != nil {
		slog.Error("plan: save failed", "err", err)
		return
	}

	if err := s.materializeNodes(ctx, taskID, plan, task); err != nil {
		_ = s.Store.SetTaskError(ctx, taskID, "工作流生成失败："+err.Error())
		_, _ = s.Store.TransitionTask(ctx, taskID, domain.TaskFailed, domain.EvtResearchFailed,
			map[string]any{"phase": "workflow", "error": err.Error()})
		return
	}

	if plan.Goal != "" && task.Title == "" {
		_ = s.Store.SetTaskTitle(ctx, taskID, plan.Goal)
	}
	next := domain.TaskAwaitingApproval
	if !s.requireApproval {
		next = domain.TaskRunning
	}
	payload := map[string]any{
		"nodes":     len(plan.Subtasks),
		"questions": len(plan.ResearchQuestions),
		"planner":   s.Cfg.PlannerMode(),
		"tokens":    usage.TotalTokens,
	}
	if _, err := s.Store.TransitionTask(ctx, taskID, next, domain.EvtWorkflowCreated, payload); err != nil {
		slog.Error("plan: set status failed", "err", err)
		return
	}
	if next == domain.TaskRunning {
		_, _ = s.Store.Emit(ctx, &domain.Event{TaskID: taskID, Type: domain.EvtResearchStarted, Payload: map[string]any{"auto": true}})
	}
	slog.Info("plan ready", "task", taskID, "nodes", len(plan.Subtasks), "mode", s.Cfg.PlannerMode())
}

// materializeNodes 把计划写入 worklow_nodes（先清空旧节点，支持重规划）。
func (s *Server) materializeNodes(ctx context.Context, taskID string, plan *domain.Plan, task *domain.Task) error {
	workflow := planner.BuildWorkflow(plan)
	if err := dag.Validate(workflow.Nodes); err != nil {
		return err
	}
	if _, err := s.Store.Q.DeleteNodesByTask(ctx, taskID); err != nil {
		return err
	}
	maxAttempts := s.Cfg.NodeMaxAttempts
	searchIdx := 0
	questions := map[string]string{}
	for _, q := range plan.ResearchQuestions {
		questions[q.ID] = q.Question
	}
	for i, n := range workflow.Nodes {
		input := map[string]any{}
		if st := findSubtask(plan, n.ID); st != nil {
			for _, in := range st.Inputs {
				if q, ok := questions[in]; ok {
					input["question"] = q
					input["question_id"] = in
				}
			}
			// search 节点：question 用于展示，query 用于真实检索。
			// 规划器（LLM 模式）给了检索式就用它，否则按目标套模板。
			if n.Type == domain.NodeTypeSearch {
				q := stringValue(input["question"])
				if q == "" {
					q = n.Title
				}
				input["query"] = planner.NodeSearchQuery(task.Goal, st.Query, searchIdx)
				input["fallback_query"] = planner.FallbackQuery(task.Goal)
				searchIdx++
			}
			if n.Type == domain.NodeTypeAnalyze {
				if stringValue(input["question"]) == "" {
					input["question"] = n.Title
				}
			}
			if n.Type == domain.NodeTypeVerify || n.Type == domain.NodeTypeSynthesize {
				input["question"] = task.Goal
			}
		}
		node := &domain.Node{
			ID: n.ID, TaskID: taskID, Title: n.Title, Type: n.Type,
			AgentRole: n.AgentRole, DependsOn: n.DependsOn, Position: i,
			MaxAttempts: maxAttempts, Input: input,
		}
		if err := s.Store.CreateNode(ctx, node); err != nil {
			return err
		}
	}
	return s.Store.SetTaskWorkflow(ctx, taskID, &workflow)
}

func findSubtask(plan *domain.Plan, id string) *domain.Subtask {
	for i := range plan.Subtasks {
		if plan.Subtasks[i].ID == id {
			return &plan.Subtasks[i]
		}
	}
	return nil
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func questionsForDepth(depth string) int {
	switch depth {
	case "quick":
		return 2
	case "deep":
		return 4
	default:
		return 3
	}
}

func (s *Server) handleListResearch(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 20)
	if limit > 100 {
		limit = 100
	}
	offset := intQuery(r, "offset", 0)
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	tasks, total, err := s.Store.ListTasks(ctx, r.URL.Query().Get("status"), s.owner(r), limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tasks, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) handleGetResearch(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	task, err := s.Store.GetTask(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	resp := map[string]any{"task": task, "nodes": task.Nodes, "workflow": task.Workflow}
	if counts, err := s.Store.NodeStatusCounts(ctx, task.ID); err == nil {
		resp["node_status"] = counts
	}
	if rep, err := s.Store.GetLatestReport(ctx, task.ID); err == nil {
		resp["report_summary"] = map[string]any{
			"id": rep.ID, "version": rep.Version, "title": rep.Title,
			"citation_count": rep.CitationCount, "word_count": rep.WordCount, "created_at": rep.CreatedAt,
		}
	}
	if srcCount, err := s.Store.CountSources(ctx, task.ID); err == nil {
		resp["source_count"] = srcCount
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeleteResearch(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	if err := s.Store.DeleteTask(ctx, task.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": task.ID})
}

// transition 统一走状态机：非法跃迁返回 409 invalid_transition 并已写审计事件。
func (s *Server) transition(ctx context.Context, w http.ResponseWriter, taskID, to, eventType string, payload map[string]any) *domain.Task {
	task, err := s.Store.TransitionTask(ctx, taskID, to, eventType, payload)
	if err != nil {
		var inv *domain.ErrInvalidTransition
		if errors.As(err, &inv) {
			writeErr(w, http.StatusConflict, "invalid_transition", inv.Error())
			return nil
		}
		writeErr(w, http.StatusConflict, "transition_failed", err.Error())
		return nil
	}
	return task
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	task, err := s.Store.GetTask(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	if len(task.Nodes) == 0 {
		writeErr(w, http.StatusConflict, "no_workflow", "任务尚未生成工作流，无法批准")
		return
	}
	if task.Status == domain.TaskRunning {
		writeJSON(w, http.StatusOK, map[string]any{"task": task, "already_running": true})
		return
	}
	updated := s.transition(ctx, w, task.ID, domain.TaskRunning, domain.EvtResearchStarted,
		map[string]any{"approved_by": s.owner(r)})
	if updated == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": updated})
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	if s.transition(ctx, w, task.ID, domain.TaskPaused, domain.EvtResearchPaused, nil) == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": domain.TaskPaused})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	if s.transition(ctx, w, task.ID, domain.TaskRunning, domain.EvtResearchResumed, nil) == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": domain.TaskRunning})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	if s.transition(ctx, w, task.ID, domain.TaskCancelled, domain.EvtResearchCancelled,
		map[string]any{"reason": "cancelled by user"}) == nil {
		return
	}
	n, _ := s.Store.CancelPendingNodes(ctx, task.ID)
	writeJSON(w, http.StatusOK, map[string]any{"status": domain.TaskCancelled, "cancelled_nodes": n})
}

func (s *Server) handleReplan(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 15*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	if task.Status == domain.TaskRunning || task.Status == domain.TaskSynthesizing {
		writeErr(w, http.StatusConflict, "invalid_state", "任务运行中，不能重新规划")
		return
	}
	go s.planAsync(task.ID, task.Goal, task.Depth, task.EnableVerification)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "planning", "task_id": task.ID})
}

func (s *Server) handleRetryNode(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 20*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	nodeID := r.PathValue("node_id")
	node, err := s.Store.GetNode(ctx, task.ID, nodeID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "节点不存在")
		return
	}
	if node.Status != domain.NodeFailed && node.Status != domain.NodeCancelled && node.Status != domain.NodeSkipped {
		writeErr(w, http.StatusConflict, "invalid_state", "只有失败/取消/跳过的节点可以重试")
		return
	}
	if task.Status == domain.TaskCompleted {
		writeErr(w, http.StatusConflict, "invalid_state", "任务已完成；如需重跑请新建任务")
		return
	}
	runID, _ := s.Store.GetOrCreateRun(ctx, task.ID)
	if err := s.Store.ResetNodeForRetry(ctx, task.ID, nodeID, runID, 1); err != nil {
		writeErr(w, http.StatusInternalServerError, "retry_failed", err.Error())
		return
	}
	// 任务若已失败，重新拉起为 running，使重试节点可被调度（paused 保持暂停）。
	if task.Status == domain.TaskFailed {
		if s.transition(ctx, w, task.ID, domain.TaskRunning, domain.EvtResearchResumed,
			map[string]any{"reason": "node retry"}) == nil {
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"node_id": nodeID, "status": domain.NodePending})
}
