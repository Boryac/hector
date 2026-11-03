-- 工作流节点（定义 + 当前执行状态）

-- name: CreateWorkflowNode :exec
INSERT INTO workflow_nodes (
    id, task_id, title, node_type, agent_role, agent_name, depends_on,
    position, status, max_attempts, input
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, 'pending', $9, $10
)
ON CONFLICT (task_id, id) DO UPDATE SET
    title = EXCLUDED.title,
    node_type = EXCLUDED.node_type,
    agent_role = EXCLUDED.agent_role,
    agent_name = EXCLUDED.agent_name,
    depends_on = EXCLUDED.depends_on,
    position = EXCLUDED.position,
    max_attempts = EXCLUDED.max_attempts,
    input = EXCLUDED.input,
    updated_at = now();

-- name: ListNodes :many
SELECT * FROM workflow_nodes WHERE task_id = $1 ORDER BY position, id;

-- name: GetNode :one
SELECT * FROM workflow_nodes WHERE task_id = $1 AND id = $2;

-- 认领可执行节点：任务处于 running、节点 pending、依赖全部 completed、未被其它 Worker 租约占用。
-- name: ClaimReadyNodes :many
WITH ready AS (
    SELECT n.task_id, n.id
    FROM workflow_nodes n
    JOIN research_tasks t ON t.id = n.task_id
    WHERE t.status IN ('running','synthesizing')
      AND n.status = 'pending'
      AND (n.next_attempt_at IS NULL OR n.next_attempt_at <= now())
      AND NOT EXISTS (
          SELECT 1
          FROM unnest(n.depends_on) AS dep(id)
          WHERE NOT EXISTS (
              SELECT 1 FROM workflow_nodes d
              WHERE d.task_id = n.task_id AND d.id = dep.id AND d.status = 'completed'
          )
      )
    ORDER BY n.position
    LIMIT sqlc.arg('limit')::int
    FOR UPDATE OF n SKIP LOCKED
)
UPDATE workflow_nodes w
SET status = 'running',
    attempts = w.attempts + 1,
    worker_id = sqlc.arg('worker_id')::text,
    started_at = COALESCE(w.started_at, now()),
    updated_at = now(),
    progress = 0,
    progress_text = 'starting',
    error = '',
    lease_expires_at = now() + make_interval(secs => sqlc.arg('lease_seconds')::int)
FROM ready
WHERE w.task_id = ready.task_id AND w.id = ready.id
RETURNING w.*;

-- name: UpdateNodeProgress :exec
UPDATE workflow_nodes
SET progress = sqlc.arg('progress')::int, progress_text = sqlc.arg('progress_text')::text, updated_at = now(),
    lease_expires_at = now() + make_interval(secs => sqlc.arg('lease_seconds')::int)
WHERE task_id = sqlc.arg('task_id') AND id = sqlc.arg('id') AND status = 'running';

-- name: CompleteNode :exec
UPDATE workflow_nodes
SET status = 'completed', output = sqlc.arg('output')::jsonb, progress = 100, progress_text = 'completed',
    completed_at = now(), updated_at = now(), lease_expires_at = NULL,
    tokens_total = tokens_total + sqlc.arg('tokens')::int, error = ''
WHERE task_id = sqlc.arg('task_id') AND id = sqlc.arg('id');

-- name: FailNode :exec
UPDATE workflow_nodes
SET status = 'failed', error = sqlc.arg('error')::text, completed_at = now(), updated_at = now(), lease_expires_at = NULL
WHERE task_id = sqlc.arg('task_id') AND id = sqlc.arg('id');

-- name: ScheduleNodeRetry :exec
UPDATE workflow_nodes
SET status = 'pending', error = sqlc.arg('error')::text, updated_at = now(), lease_expires_at = NULL,
    next_attempt_at = now() + make_interval(secs => sqlc.arg('backoff_seconds')::int)
WHERE task_id = sqlc.arg('task_id') AND id = sqlc.arg('id');

-- name: ResetNodeForRetry :exec
UPDATE workflow_nodes
SET status = 'pending', next_attempt_at = NULL, error = '', lease_expires_at = NULL,
    completed_at = NULL, updated_at = now(), max_attempts = attempts + sqlc.arg('extra_attempts')::int
WHERE task_id = sqlc.arg('task_id') AND id = sqlc.arg('id')
  AND status IN ('failed','completed','skipped','cancelled');

-- name: ResetStaleNodes :execrows
UPDATE workflow_nodes
SET status = 'pending', next_attempt_at = now(), lease_expires_at = NULL,
    error = 'worker lease expired; requeued', updated_at = now()
WHERE status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at < now();

-- name: CancelPendingNodes :execrows
UPDATE workflow_nodes
SET status = 'cancelled', updated_at = now(), completed_at = now()
WHERE task_id = $1 AND status IN ('pending','queued');

-- name: FailOrphanNodes :execrows
UPDATE workflow_nodes n
SET status = 'failed', error = 'dependency failed', updated_at = now(), completed_at = now()
WHERE n.task_id = $1 AND n.status = 'pending'
  AND EXISTS (
      SELECT 1 FROM unnest(n.depends_on) AS dep(id)
      WHERE EXISTS (
          SELECT 1 FROM workflow_nodes d
          WHERE d.task_id = n.task_id AND d.id = dep.id
            AND d.status IN ('failed','cancelled','skipped')
      )
  );

-- name: NodeStatusCounts :many
SELECT status, COUNT(*)::int AS count FROM workflow_nodes WHERE task_id = $1 GROUP BY status;

-- name: InsertNodeExecution :exec
INSERT INTO node_executions (id, task_id, run_id, node_id, attempt, status, started_at, worker_id)
VALUES ($1, $2, $3, $4, $5, 'running', now(), $6);

-- name: FinishNodeExecution :exec
UPDATE node_executions
SET status = $2, completed_at = now(),
    duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (now() - started_at)) * 1000)::int),
    error = $3, output = $4
WHERE id = $1;

-- name: ListNodeExecutions :many
SELECT * FROM node_executions WHERE task_id = $1 ORDER BY started_at DESC LIMIT 200;

-- name: DeleteNodesByTask :execrows
DELETE FROM workflow_nodes WHERE task_id = $1;

-- name: CountNodes :one
SELECT COUNT(*) FROM workflow_nodes WHERE task_id = $1;
