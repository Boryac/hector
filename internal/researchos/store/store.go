// Package store 是 ResearchOS 的持久化层。
//
// 查询由 sqlc 从 internal/researchos/db/queries 生成（见 sqlc.yaml）；
// 本包负责：事务边界、领域模型转换、以及「状态变更 + 事件写入」的原子性。
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/verikod/hector/internal/researchos/db"
	"github.com/verikod/hector/internal/researchos/domain"
)

// ErrNotFound 表示资源不存在（或不属于该任务）。
var ErrNotFound = errors.New("not found")

// Store 聚合数据库连接与查询对象。
type Store struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

// New 创建 Store。
func New(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool, Q: db.New(pool)}
}

// Tx 在事务中执行 fn；fn 内部必须使用传入的 *db.Queries。
func (s *Store) Tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// 事件
// ---------------------------------------------------------------------------

// Emit 在独立事务中写入一条事件。
func (s *Store) Emit(ctx context.Context, ev *domain.Event) (int64, error) {
	var seq int64
	err := s.Tx(ctx, func(q *db.Queries) error {
		n, err := s.emitTx(ctx, q, ev)
		seq = n
		return err
	})
	return seq, err
}

// EmitTx 在调用方事务中写入事件，保证「状态变更 + 事件」原子性。
func (s *Store) EmitTx(ctx context.Context, q *db.Queries, ev *domain.Event) (int64, error) {
	return s.emitTx(ctx, q, ev)
}

func (s *Store) emitTx(ctx context.Context, q *db.Queries, ev *domain.Event) (int64, error) {
	if ev.EventID == "" {
		ev.EventID = uuid.NewString()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	seq, err := q.BumpSequence(ctx, ev.TaskID)
	if err != nil {
		return 0, fmt.Errorf("bump sequence: %w", err)
	}
	payload := ev.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	ev.Sequence = seq
	// 事件 payload 禁止写入凭据：上游调用方只允许放业务字段（EVENT_PROTOCOL.md）。
	if err := q.InsertEvent(ctx, db.InsertEventParams{
		EventID:   ev.EventID,
		TaskID:    ev.TaskID,
		RunID:     ev.RunID,
		NodeID:    ev.NodeID,
		EventType: ev.Type,
		Sequence:  seq,
		Payload:   raw,
		CreatedAt: ev.Timestamp,
	}); err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}
	return seq, nil
}

// ListEventsAfter 返回 sequence 大于 after 的事件（用于 SSE 续接与重放）。
func (s *Store) ListEventsAfter(ctx context.Context, taskID string, after int64, limit int32) ([]*domain.Event, error) {
	rows, err := s.Q.ListEventsAfter(ctx, db.ListEventsAfterParams{TaskID: taskID, Sequence: after, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Event, 0, len(rows))
	for _, r := range rows {
		out = append(out, eventFromRow(r))
	}
	return out, nil
}

func eventFromRow(r db.TaskEvent) *domain.Event {
	var payload map[string]any
	if len(r.Payload) > 0 {
		_ = json.Unmarshal(r.Payload, &payload)
	}
	return &domain.Event{
		EventID:   r.EventID,
		TaskID:    r.TaskID,
		RunID:     r.RunID,
		NodeID:    r.NodeID,
		Type:      r.EventType,
		Timestamp: r.CreatedAt,
		Sequence:  r.Sequence,
		Payload:   payload,
	}
}

// ---------------------------------------------------------------------------
// 任务
// ---------------------------------------------------------------------------

// CreateTask 写入任务并发出 research.created 事件。
func (s *Store) CreateTask(ctx context.Context, t *domain.Task) error {
	scope, _ := json.Marshal(orEmptyMap(t.Scope))
	err := s.Tx(ctx, func(q *db.Queries) error {
		row, err := q.CreateTask(ctx, db.CreateTaskParams{
			ID:                 t.ID,
			Title:              t.Title,
			Goal:               t.Goal,
			Depth:              t.Depth,
			Scope:              scope,
			MaxAgents:          int32(t.MaxAgents),
			MaxDurationSeconds: int32(t.MaxDurationSeconds),
			EnableVerification: t.EnableVerification,
			Status:             t.Status,
			CreatedBy:          t.CreatedBy,
			PlannerMode:        t.PlannerMode,
		})
		if err != nil {
			return err
		}
		t.CreatedAt = row.CreatedAt
		t.UpdatedAt = row.UpdatedAt
		_, err = s.emitTx(ctx, q, &domain.Event{
			TaskID: t.ID,
			Type:   domain.EvtResearchCreated,
			Payload: map[string]any{
				"goal":   t.Goal,
				"depth":  t.Depth,
				"status": t.Status,
			},
		})
		return err
	})
	return err
}

// GetTask 读取任务（含节点）。
func (s *Store) GetTask(ctx context.Context, id string) (*domain.Task, error) {
	row, err := s.Q.GetTask(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t := taskFromRow(row)
	nodes, err := s.ListNodes(ctx, id)
	if err != nil {
		return nil, err
	}
	t.Nodes = nodes
	return t, nil
}

// GetTaskRow 只读任务行，不加载节点。
func (s *Store) GetTaskRow(ctx context.Context, id string) (*domain.Task, error) {
	row, err := s.Q.GetTask(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return taskFromRow(row), nil
}

// ListTasks 分页查询任务。
func (s *Store) ListTasks(ctx context.Context, status, createdBy string, limit, offset int) ([]*domain.Task, int64, error) {
	params := db.ListTasksParams{Limit: int32(limit), Offset: int32(offset)}
	if status != "" {
		params.Status = pgtype.Text{String: status, Valid: true}
	}
	if createdBy != "" {
		params.CreatedBy = pgtype.Text{String: createdBy, Valid: true}
	}
	rows, err := s.Q.ListTasks(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.Q.CountTasks(ctx, db.CountTasksParams{Status: params.Status, CreatedBy: params.CreatedBy})
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, taskFromRow(r))
	}
	return out, total, nil
}

// SetTaskStatus 变更任务状态并写事件（原子）。
func (s *Store) SetTaskStatus(ctx context.Context, taskID, from, to, eventType string, payload map[string]any) error {
	return s.Tx(ctx, func(q *db.Queries) error {
		if from != "" {
			n, err := q.SetTaskStatusIf(ctx, db.SetTaskStatusIfParams{
				Status:      to,
				ID:          taskID,
				AllowedFrom: []string{from},
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("task %s 当前状态不允许从 %q 变更到 %q", taskID, from, to)
			}
		} else if err := q.SetTaskStatus(ctx, db.SetTaskStatusParams{ID: taskID, Status: to}); err != nil {
			return err
		}
		if eventType == "" {
			return nil
		}
		if payload == nil {
			payload = map[string]any{}
		}
		if _, ok := payload["status"]; !ok {
			payload["status"] = to
		}
		_, err := s.emitTx(ctx, q, &domain.Event{TaskID: taskID, Type: eventType, Payload: payload})
		return err
	})
}

// TransitionTask 是任务状态变更的唯一入口：先过状态机白名单，再做条件更新。
//
// 语义：
//   - 目标状态与当前一致 → 幂等返回，不写事件；
//   - 白名单不含该跃迁 → 写 research.transition_rejected 审计事件并返回
//     *domain.ErrInvalidTransition（API 层映射为 409），状态保持不变；
//   - 通过校验 → SetTaskStatus 带 from 条件更新，避免并发覆盖。
func (s *Store) TransitionTask(ctx context.Context, taskID, to, eventType string, payload map[string]any) (*domain.Task, error) {
	cur, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if cur.Status == to {
		return cur, nil
	}
	if !domain.CanTransition(cur.Status, to) {
		rejected := map[string]any{"from": cur.Status, "to": to}
		for k, v := range payload {
			rejected[k] = v
		}
		_, _ = s.Emit(ctx, &domain.Event{
			TaskID:  taskID,
			Type:    domain.EvtResearchTransitionRejected,
			Payload: rejected,
		})
		return nil, &domain.ErrInvalidTransition{TaskID: taskID, From: cur.Status, To: to}
	}
	if eventType == "" {
		eventType = domain.EventForTransition(to)
	}
	if err := s.SetTaskStatus(ctx, taskID, cur.Status, to, eventType, payload); err != nil {
		return nil, err
	}
	return s.GetTask(ctx, taskID)
}

// SetTaskPlan 保存 Planner 输出。
func (s *Store) SetTaskPlan(ctx context.Context, taskID string, plan *domain.Plan, plannerMode, planErr string) error {
	var raw []byte
	if plan != nil {
		b, err := json.Marshal(plan)
		if err != nil {
			return err
		}
		raw = b
	}
	return s.Q.SetTaskPlan(ctx, db.SetTaskPlanParams{
		ID:          taskID,
		Plan:        raw,
		PlanError:   planErr,
		PlannerMode: plannerMode,
	})
}

// SetTaskWorkflow 保存工作流定义。
func (s *Store) SetTaskWorkflow(ctx context.Context, taskID string, wf *domain.Workflow) error {
	raw, err := json.Marshal(wf)
	if err != nil {
		return err
	}
	return s.Q.SetTaskWorkflow(ctx, db.SetTaskWorkflowParams{ID: taskID, WorkflowDefinition: raw})
}

// SetTaskStats 更新统计信息。
func (s *Store) SetTaskStats(ctx context.Context, taskID string, stats map[string]any) error {
	raw, err := json.Marshal(orEmptyMap(stats))
	if err != nil {
		return err
	}
	return s.Q.SetTaskStats(ctx, db.SetTaskStatsParams{ID: taskID, Stats: raw})
}

// SetTaskError 记录失败原因。
func (s *Store) SetTaskError(ctx context.Context, taskID, msg string) error {
	return s.Q.SetTaskError(ctx, db.SetTaskErrorParams{ID: taskID, Error: msg})
}

// SetTaskTitle 更新标题。
func (s *Store) SetTaskTitle(ctx context.Context, taskID, title string) error {
	return s.Q.SetTaskTitle(ctx, db.SetTaskTitleParams{ID: taskID, Title: title})
}

// DeleteTask 删除任务及其级联数据。
func (s *Store) DeleteTask(ctx context.Context, taskID string) error {
	return s.Q.DeleteTask(ctx, taskID)
}

// ListUnfinishedTaskIDs 返回未结束任务，用于 Worker 启动时恢复。
func (s *Store) ListUnfinishedTaskIDs(ctx context.Context) ([]string, error) {
	return s.Q.ListUnfinishedTaskIDs(ctx)
}

func taskFromRow(r db.ResearchTask) *domain.Task {
	t := &domain.Task{
		ID:                 r.ID,
		Title:              r.Title,
		Goal:               r.Goal,
		Depth:              r.Depth,
		MaxAgents:          int(r.MaxAgents),
		MaxDurationSeconds: int(r.MaxDurationSeconds),
		EnableVerification: r.EnableVerification,
		Status:             r.Status,
		CreatedBy:          r.CreatedBy,
		PlannerMode:        r.PlannerMode,
		PlanError:          r.PlanError,
		Error:              r.Error,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
		StartedAt:          tsPtr(r.StartedAt),
		FinishedAt:         tsPtr(r.FinishedAt),
		LatestSequence:     r.LatestSequence,
		Stats:              map[string]any{},
	}
	if len(r.Scope) > 0 {
		_ = json.Unmarshal(r.Scope, &t.Scope)
	}
	if len(r.Stats) > 0 {
		_ = json.Unmarshal(r.Stats, &t.Stats)
	}
	if len(r.Plan) > 0 {
		var p domain.Plan
		if err := json.Unmarshal(r.Plan, &p); err == nil {
			t.Plan = &p
		}
	}
	if len(r.WorkflowDefinition) > 0 {
		var wf domain.Workflow
		if err := json.Unmarshal(r.WorkflowDefinition, &wf); err == nil {
			t.Workflow = &wf
		}
	}
	return t
}

// ---------------------------------------------------------------------------
// 节点
// ---------------------------------------------------------------------------

// CreateNode 写入工作流节点定义。
func (s *Store) CreateNode(ctx context.Context, n *domain.Node) error {
	input, err := json.Marshal(orEmptyMap(n.Input))
	if err != nil {
		return err
	}
	maxAttempts := n.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	deps := n.DependsOn
	if deps == nil {
		deps = []string{}
	}
	return s.Q.CreateWorkflowNode(ctx, db.CreateWorkflowNodeParams{
		ID:          n.ID,
		TaskID:      n.TaskID,
		Title:       n.Title,
		NodeType:    n.Type,
		AgentRole:   n.AgentRole,
		AgentName:   n.AgentName,
		DependsOn:   deps,
		Position:    int32(n.Position),
		MaxAttempts: int32(maxAttempts),
		Input:       input,
	})
}

// ListNodes 读取任务的节点快照。
func (s *Store) ListNodes(ctx context.Context, taskID string) ([]*domain.Node, error) {
	rows, err := s.Q.ListNodes(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Node, 0, len(rows))
	for _, r := range rows {
		out = append(out, nodeFromRow(r))
	}
	return out, nil
}

// GetNode 读取单个节点。
func (s *Store) GetNode(ctx context.Context, taskID, nodeID string) (*domain.Node, error) {
	r, err := s.Q.GetNode(ctx, db.GetNodeParams{TaskID: taskID, ID: nodeID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return nodeFromRow(r), nil
}

// ClaimNodes 认领可执行节点（多 Worker 安全，SKIP LOCKED + 租约）。
func (s *Store) ClaimNodes(ctx context.Context, workerID string, limit int, lease time.Duration) ([]*domain.Node, error) {
	rows, err := s.Q.ClaimReadyNodes(ctx, db.ClaimReadyNodesParams{
		Limit:        int32(limit),
		WorkerID:     workerID,
		LeaseSeconds: int32(lease.Seconds()),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Node, 0, len(rows))
	for _, r := range rows {
		out = append(out, nodeFromRow(r))
	}
	return out, nil
}

// UpdateNodeProgress 更新进度（不写事件时由调用方决定，避免事件风暴）。
func (s *Store) UpdateNodeProgress(ctx context.Context, taskID, nodeID string, progress int, text string, lease time.Duration) error {
	return s.Q.UpdateNodeProgress(ctx, db.UpdateNodeProgressParams{
		TaskID:       taskID,
		ID:           nodeID,
		Progress:     int32(progress),
		ProgressText: text,
		LeaseSeconds: int32(lease.Seconds()),
	})
}

// CompleteNode 标记节点完成并写事件（原子）。
func (s *Store) CompleteNode(ctx context.Context, taskID, nodeID, runID string, output map[string]any, tokens int, payload map[string]any) error {
	raw, err := json.Marshal(orEmptyMap(output))
	if err != nil {
		return err
	}
	return s.Tx(ctx, func(q *db.Queries) error {
		if err := q.CompleteNode(ctx, db.CompleteNodeParams{
			TaskID: taskID,
			ID:     nodeID,
			Output: raw,
			Tokens: int32(tokens),
		}); err != nil {
			return err
		}
		if payload == nil {
			payload = map[string]any{}
		}
		if _, ok := payload["progress"]; !ok {
			payload["progress"] = 100
		}
		_, err := s.emitTx(ctx, q, &domain.Event{TaskID: taskID, RunID: runID, NodeID: nodeID, Type: domain.EvtNodeCompleted, Payload: payload})
		return err
	})
}

// FailNode 标记节点失败并写事件（原子）。
func (s *Store) FailNode(ctx context.Context, taskID, nodeID, runID, msg string) error {
	return s.Tx(ctx, func(q *db.Queries) error {
		if err := q.FailNode(ctx, db.FailNodeParams{TaskID: taskID, ID: nodeID, Error: msg}); err != nil {
			return err
		}
		_, err := s.emitTx(ctx, q, &domain.Event{
			TaskID: taskID, RunID: runID, NodeID: nodeID, Type: domain.EvtNodeFailed,
			Payload: map[string]any{"error": msg},
		})
		return err
	})
}

// ScheduleNodeRetry 安排节点重试并写事件（原子）。
func (s *Store) ScheduleNodeRetry(ctx context.Context, taskID, nodeID, runID, msg string, backoff time.Duration, attempt int) error {
	return s.Tx(ctx, func(q *db.Queries) error {
		if err := q.ScheduleNodeRetry(ctx, db.ScheduleNodeRetryParams{
			TaskID: taskID, ID: nodeID, Error: msg, BackoffSeconds: int32(backoff.Seconds()),
		}); err != nil {
			return err
		}
		_, err := s.emitTx(ctx, q, &domain.Event{
			TaskID: taskID, RunID: runID, NodeID: nodeID, Type: domain.EvtNodeRetrying,
			Payload: map[string]any{"error": msg, "attempt": attempt, "backoff_seconds": int(backoff.Seconds())},
		})
		return err
	})
}

// ResetNodeForRetry 手动重试节点（API 触发）。
func (s *Store) ResetNodeForRetry(ctx context.Context, taskID, nodeID, runID string, extraAttempts int) error {
	return s.Tx(ctx, func(q *db.Queries) error {
		if err := q.ResetNodeForRetry(ctx, db.ResetNodeForRetryParams{
			TaskID: taskID, ID: nodeID, ExtraAttempts: int32(extraAttempts),
		}); err != nil {
			return err
		}
		_, err := s.emitTx(ctx, q, &domain.Event{
			TaskID: taskID, RunID: runID, NodeID: nodeID, Type: domain.EvtNodeQueued,
			Payload: map[string]any{"reason": "manual retry"},
		})
		return err
	})
}

// ResetStaleNodes 回收租约过期的运行中节点（Worker 崩溃恢复）。
func (s *Store) ResetStaleNodes(ctx context.Context) (int64, error) {
	return s.Q.ResetStaleNodes(ctx)
}

// CancelPendingNodes 取消任务下所有待执行节点。
func (s *Store) CancelPendingNodes(ctx context.Context, taskID string) (int64, error) {
	return s.Q.CancelPendingNodes(ctx, taskID)
}

// FailOrphanNodes 把依赖失败/取消的待执行节点标记为失败，避免任务永久挂起。
func (s *Store) FailOrphanNodes(ctx context.Context, taskID string) (int64, error) {
	return s.Q.FailOrphanNodes(ctx, taskID)
}

// NodeStatusCounts 返回节点状态统计。
func (s *Store) NodeStatusCounts(ctx context.Context, taskID string) (map[string]int, error) {
	rows, err := s.Q.NodeStatusCounts(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.Status] = int(r.Count)
	}
	return out, nil
}

func nodeFromRow(r db.WorkflowNode) *domain.Node {
	n := &domain.Node{
		ID:           r.ID,
		TaskID:       r.TaskID,
		Title:        r.Title,
		Type:         r.NodeType,
		AgentRole:    r.AgentRole,
		AgentName:    r.AgentName,
		DependsOn:    r.DependsOn,
		Position:     int(r.Position),
		Status:       r.Status,
		Attempts:     int(r.Attempts),
		MaxAttempts:  int(r.MaxAttempts),
		Error:        r.Error,
		Progress:     int(r.Progress),
		ProgressText: r.ProgressText,
		TokensTotal:  int(r.TokensTotal),
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
		StartedAt:    tsPtr(r.StartedAt),
		CompletedAt:  tsPtr(r.CompletedAt),
		Input:        map[string]any{},
	}
	if len(r.Input) > 0 {
		_ = json.Unmarshal(r.Input, &n.Input)
	}
	if len(r.Output) > 0 {
		n.Output = map[string]any{}
		_ = json.Unmarshal(r.Output, &n.Output)
	}
	return n
}

// ---------------------------------------------------------------------------
// 辅助
// ---------------------------------------------------------------------------

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
