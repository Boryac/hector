// Package worker 是 ResearchOS 的执行引擎进程。
//
// 复用上游 Hector 的队列设计思想（租约 + 重试 + 死信 + 崩溃恢复），
// 但队列载体改为 PostgreSQL 中的 workflow_nodes 表（SKIP LOCKED 认领），
// 因为研究节点是 DAG 依赖驱动而非 FIFO，直接复用 task_queue 会丢失依赖语义。
// 该取舍记录在 DECISIONS.md（第 16 条）。
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/verikod/hector/internal/researchos/domain"
	"github.com/verikod/hector/internal/researchos/exec"
	"github.com/verikod/hector/internal/researchos/store"
)

// Config 是 Worker 参数。
type Config struct {
	Workers     int
	NodeTimeout time.Duration
	TaskTimeout time.Duration
	Lease       time.Duration
	Tick        time.Duration
	MaxAttempts int
}

func (c *Config) setDefaults() {
	if c.Workers <= 0 {
		c.Workers = 2
	}
	if c.NodeTimeout <= 0 {
		c.NodeTimeout = 3 * time.Minute
	}
	if c.TaskTimeout <= 0 {
		c.TaskTimeout = 30 * time.Minute
	}
	if c.Lease <= 0 {
		c.Lease = 3 * time.Minute
	}
	if c.Tick <= 0 {
		c.Tick = 500 * time.Millisecond
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
}

// Worker 执行 DAG 节点。
type Worker struct {
	Store  *store.Store
	Engine *exec.Engine
	Cfg    Config
	ID     string

	sem      chan struct{}
	wg       sync.WaitGroup
	lastGC   time.Time
	stopOnce sync.Once
}

// New 创建 Worker。
func New(st *store.Store, eng *exec.Engine, cfg Config) *Worker {
	cfg.setDefaults()
	return &Worker{Store: st, Engine: eng, Cfg: cfg, ID: "worker-" + uuid.NewString()[:8], sem: make(chan struct{}, cfg.Workers)}
}

// Run 启动调度循环，直到 ctx 取消。
func (w *Worker) Run(ctx context.Context) error {
	slog.Info("research worker started", "worker_id", w.ID, "workers", w.Cfg.Workers)
	ticker := time.NewTicker(w.Cfg.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.wg.Wait()
			return ctx.Err()
		case <-ticker.C:
			if err := w.tick(ctx); err != nil {
				slog.Error("worker tick failed", "err", err)
			}
		}
	}
}

func (w *Worker) tick(ctx context.Context) error {
	// 1. 回收租约过期的节点（Worker 崩溃恢复）
	if time.Since(w.lastGC) > 10*time.Second {
		w.lastGC = time.Now()
		if n, err := w.Store.ResetStaleNodes(ctx); err != nil {
			slog.Warn("reset stale nodes failed", "err", err)
		} else if n > 0 {
			slog.Warn("requeued stale nodes", "count", n)
		}
		// 看门狗：节点全部终态但任务仍处于 running 时推进任务状态，避免永久挂起。
		if ids, err := w.Store.Q.ListRunningTaskIDs(ctx); err == nil {
			for _, id := range ids {
				w.finalizeTask(ctx, id)
			}
		}
	}

	// 2. 计算空闲槽位并认领节点
	free := w.Cfg.Workers - len(w.sem)
	if free <= 0 {
		return nil
	}
	nodes, err := w.Store.ClaimNodes(ctx, w.ID, free, w.Cfg.Lease)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		w.sem <- struct{}{}
		w.wg.Add(1)
		go func(n *domain.Node) {
			defer w.wg.Done()
			defer func() { <-w.sem }()
			w.execute(ctx, n)
		}(node)
	}
	return nil
}

func (w *Worker) execute(ctx context.Context, node *domain.Node) {
	task, err := w.Store.GetTaskRow(ctx, node.TaskID)
	if err != nil {
		slog.Error("load task failed", "task", node.TaskID, "err", err)
		return
	}
	runID, err := w.Store.GetOrCreateRun(ctx, node.TaskID)
	if err != nil {
		slog.Error("get run failed", "task", node.TaskID, "err", err)
	}

	// 任务超时保护
	if task.MaxDurationSeconds > 0 && task.StartedAt != nil &&
		time.Since(*task.StartedAt) > time.Duration(task.MaxDurationSeconds)*time.Second {
		_ = w.Store.FailNode(ctx, node.TaskID, node.ID, runID, "任务超出最大执行时长")
		w.finalizeTask(ctx, node.TaskID)
		return
	}

	nodeExecID := uuid.NewString()
	_ = w.Store.StartNodeExecution(ctx, nodeExecID, node.TaskID, runID, node.ID, node.Attempts, w.ID)

	// 状态推进：synthesize 开始时任务进入 synthesizing
	if node.Type == domain.NodeTypeSynthesize && task.Status == domain.TaskRunning {
		_, _ = w.Store.TransitionTask(ctx, node.TaskID, domain.TaskSynthesizing,
			domain.EvtResearchSynthesizing, map[string]any{"phase": "synthesizing"})
	}

	agentID := uuid.NewString()
	_ = w.Store.InsertAgentExecution(ctx, &domain.AgentExecution{
		ID: agentID, TaskID: node.TaskID, RunID: runID, NodeID: node.ID,
		Role: node.AgentRole, Name: agentName(node), Model: w.modelName(),
	})
	if _, err := w.Store.Emit(ctx, &domain.Event{
		TaskID: node.TaskID, RunID: runID, NodeID: node.ID, Type: domain.EvtNodeStarted,
		Payload: map[string]any{
			"title": node.Title, "type": node.Type, "attempt": node.Attempts,
			"agent_role": node.AgentRole, "agent_id": agentID, "worker_id": w.ID,
		},
	}); err != nil {
		slog.Warn("emit node.started failed", "err", err)
	}
	_, _ = w.Store.Emit(ctx, &domain.Event{
		TaskID: node.TaskID, RunID: runID, NodeID: node.ID, Type: domain.EvtAgentStarted,
		Payload: map[string]any{"role": node.AgentRole, "name": agentName(node), "agent_id": agentID},
	})

	start := time.Now()
	nodeCtx, cancel := context.WithTimeout(ctx, w.Cfg.NodeTimeout)
	output, tokens, runErr := w.Engine.Run(nodeCtx, task, node, runID)
	cancel()
	elapsed := time.Since(start)

	if runErr == nil {
		if err := w.Store.CompleteNode(ctx, node.TaskID, node.ID, runID, output, tokens, map[string]any{
			"title": node.Title, "duration_ms": int(elapsed.Milliseconds()), "tokens": tokens,
		}); err != nil {
			slog.Error("complete node failed", "node", node.ID, "err", err)
		}
		_ = w.Store.FinishNodeExecution(ctx, nodeExecID, "completed", "", output)
		_ = w.Store.FinishAgentExecution(ctx, agentID, "completed", "", map[string]any{
			"prompt_tokens": tokens, "total_tokens": tokens,
		})
		_, _ = w.Store.Emit(ctx, &domain.Event{
			TaskID: node.TaskID, RunID: runID, NodeID: node.ID, Type: domain.EvtAgentCompleted,
			Payload: map[string]any{"role": node.AgentRole, "duration_ms": int(elapsed.Milliseconds()), "agent_id": agentID},
		})
		slog.Info("node completed", "task", node.TaskID, "node", node.ID, "ms", elapsed.Milliseconds())
		w.finalizeTask(ctx, node.TaskID)
		return
	}

	msg := runErr.Error()
	if errors.Is(runErr, context.DeadlineExceeded) {
		msg = fmt.Sprintf("节点执行超时（>%s）", w.Cfg.NodeTimeout)
	}
	_ = w.Store.FinishNodeExecution(ctx, nodeExecID, "failed", msg, nil)
	_ = w.Store.FinishAgentExecution(ctx, agentID, "failed", msg, map[string]any{"total_tokens": tokens})
	_, _ = w.Store.Emit(ctx, &domain.Event{
		TaskID: node.TaskID, RunID: runID, NodeID: node.ID, Type: domain.EvtAgentFailed,
		Payload: map[string]any{"role": node.AgentRole, "error": truncate(msg, 300), "agent_id": agentID},
	})

	if node.Attempts < node.MaxAttempts {
		backoff := backoffFor(node.Attempts)
		if err := w.Store.ScheduleNodeRetry(ctx, node.TaskID, node.ID, runID, truncate(msg, 500), backoff, node.Attempts); err != nil {
			slog.Error("schedule retry failed", "err", err)
		}
		slog.Warn("node retry scheduled", "task", node.TaskID, "node", node.ID, "attempt", node.Attempts, "backoff", backoff)
	} else {
		if err := w.Store.FailNode(ctx, node.TaskID, node.ID, runID, truncate(msg, 500)); err != nil {
			slog.Error("fail node failed", "err", err)
		}
		slog.Error("node failed permanently", "task", node.TaskID, "node", node.ID, "err", msg)
	}
	w.finalizeTask(ctx, node.TaskID)
}

// finalizeTask 根据节点状态推进任务状态（完成/失败）。
func (w *Worker) finalizeTask(ctx context.Context, taskID string) {
	task, err := w.Store.GetTaskRow(ctx, taskID)
	if err != nil {
		return
	}
	switch task.Status {
	case domain.TaskCancelled, domain.TaskPaused, domain.TaskCompleted, domain.TaskFailed:
		return
	}
	counts, err := w.Store.NodeStatusCounts(ctx, taskID)
	if err != nil {
		return
	}
	pending := counts[domain.NodePending] + counts[domain.NodeQueued]
	running := counts[domain.NodeRunning]
	failed := counts[domain.NodeFailed]
	completed := counts[domain.NodeCompleted]
	cancelled := counts[domain.NodeCancelled]

	// 依赖失败的节点标记失败，避免任务永久挂起。
	if pending > 0 {
		if n, err := w.Store.FailOrphanNodes(ctx, taskID); err == nil && n > 0 {
			pending -= int(n)
			failed += int(n)
		}
	}

	if running > 0 || pending > 0 {
		return
	}
	if failed > 0 || cancelled > 0 {
		_ = w.Store.SetTaskError(ctx, taskID, fmt.Sprintf("有 %d 个节点失败、%d 个节点被取消", failed, cancelled))
		_, _ = w.Store.TransitionTask(ctx, taskID, domain.TaskFailed, domain.EvtResearchFailed,
			map[string]any{"failed_nodes": failed, "cancelled_nodes": cancelled})
		return
	}
	if completed == 0 {
		return
	}
	stats := map[string]any{
		"nodes_total": completed + failed + cancelled,
		"nodes_done":  completed,
		"finished_at": time.Now().UTC().Format(time.RFC3339),
	}
	if srcCount, err := w.Store.CountSources(ctx, taskID); err == nil {
		stats["sources"] = srcCount
	}
	if claimCount, err := w.Store.CountClaims(ctx, taskID); err == nil {
		stats["claims"] = claimCount
	}
	_ = w.Store.SetTaskStats(ctx, taskID, stats)
	_, _ = w.Store.TransitionTask(ctx, taskID, domain.TaskCompleted, domain.EvtResearchCompleted, stats)
	slog.Info("task completed", "task", taskID, "nodes", completed)
}

func (w *Worker) modelName() string {
	if w.Engine.LLM == nil {
		return ""
	}
	return w.Engine.LLM.Model()
}

func agentName(n *domain.Node) string {
	if n.AgentName != "" {
		return n.AgentName
	}
	switch n.AgentRole {
	case domain.RoleResearcher:
		return "Open Source Researcher"
	case domain.RoleAnalyst:
		return "Evidence Analyst"
	case domain.RoleVerifier:
		return "Cross-Validator"
	case domain.RoleSynthesizer:
		return "Report Synthesizer"
	default:
		return n.AgentRole
	}
}

func backoffFor(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
