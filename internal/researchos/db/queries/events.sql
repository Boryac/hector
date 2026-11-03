-- 事件（幂等：event_id 冲突则忽略）

-- name: InsertEvent :exec
INSERT INTO task_events (event_id, task_id, run_id, node_id, event_type, sequence, payload, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (event_id) DO NOTHING;

-- name: ListEventsAfter :many
SELECT * FROM task_events
WHERE task_id = $1 AND sequence > $2
ORDER BY sequence
LIMIT $3;

-- name: ListRecentEvents :many
SELECT * FROM task_events
WHERE task_id = $1
ORDER BY sequence DESC
LIMIT $2;

-- name: CountEvents :one
SELECT COUNT(*) FROM task_events WHERE task_id = $1;
