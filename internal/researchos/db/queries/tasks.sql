-- 研究任务相关查询

-- name: CreateTask :one
INSERT INTO research_tasks (
    id, title, goal, depth, scope, max_agents, max_duration_seconds,
    enable_verification, status, created_by, planner_mode
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: GetTask :one
SELECT * FROM research_tasks WHERE id = $1;

-- name: ListTasks :many
SELECT * FROM research_tasks
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('created_by')::text IS NULL OR created_by = sqlc.narg('created_by')::text)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountTasks :one
SELECT COUNT(*) FROM research_tasks
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('created_by')::text IS NULL OR created_by = sqlc.narg('created_by')::text);

-- name: SetTaskStatus :exec
UPDATE research_tasks
SET status = sqlc.arg('status')::text, updated_at = now(),
    started_at = CASE WHEN sqlc.arg('status')::text = 'running' AND started_at IS NULL THEN now() ELSE started_at END,
    finished_at = CASE WHEN sqlc.arg('status')::text IN ('completed','failed','cancelled') THEN now() ELSE finished_at END
WHERE id = sqlc.arg('id');

-- name: SetTaskStatusIf :execrows
UPDATE research_tasks
SET status = sqlc.arg('status')::text, updated_at = now(),
    started_at = CASE WHEN sqlc.arg('status')::text = 'running' AND started_at IS NULL THEN now() ELSE started_at END,
    finished_at = CASE WHEN sqlc.arg('status')::text IN ('completed','failed','cancelled') THEN now() ELSE finished_at END
WHERE id = sqlc.arg('id')
  AND status = ANY(sqlc.arg('allowed_from')::text[]);

-- name: SetTaskTitle :exec
UPDATE research_tasks SET title = $2, updated_at = now() WHERE id = $1;

-- name: SetTaskPlan :exec
UPDATE research_tasks
SET plan = $2, plan_error = $3, planner_mode = $4, updated_at = now()
WHERE id = $1;

-- name: SetTaskWorkflow :exec
UPDATE research_tasks
SET workflow_definition = $2, updated_at = now()
WHERE id = $1;

-- name: SetTaskStats :exec
UPDATE research_tasks SET stats = $2, updated_at = now() WHERE id = $1;

-- name: SetTaskError :exec
UPDATE research_tasks SET error = $2, updated_at = now() WHERE id = $1;

-- name: BumpSequence :one
UPDATE research_tasks
SET latest_sequence = latest_sequence + 1, updated_at = now()
WHERE id = $1
RETURNING latest_sequence;

-- name: GetSequence :one
SELECT latest_sequence FROM research_tasks WHERE id = $1;

-- name: DeleteTask :exec
DELETE FROM research_tasks WHERE id = $1;

-- name: ListRunningTaskIDs :many
SELECT id FROM research_tasks WHERE status = 'running' ORDER BY created_at;

-- name: ListUnfinishedTaskIDs :many
SELECT id FROM research_tasks
WHERE status IN ('created','planning','awaiting_approval','running','paused','synthesizing')
ORDER BY created_at;
