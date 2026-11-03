-- Agent 执行、工具调用、MCP 服务、运行轮次、设置

-- name: InsertAgentExecution :exec
INSERT INTO agent_executions (id, task_id, run_id, node_id, role, name, model, status, token_usage)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'running', $8);

-- name: FinishAgentExecution :exec
UPDATE agent_executions
SET status = $2, error = $3, token_usage = $4,
    duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (now() - started_at)) * 1000)::int)
WHERE id = $1;

-- name: ListAgentExecutions :many
SELECT * FROM agent_executions WHERE task_id = $1 ORDER BY started_at DESC LIMIT 200;

-- name: GetAgentExecution :one
SELECT * FROM agent_executions WHERE task_id = $1 AND id = $2;

-- name: UpdateAgentToolCalls :exec
UPDATE agent_executions SET tool_calls = $2 WHERE id = $1;

-- name: InsertToolInvocation :exec
INSERT INTO tool_invocations (id, task_id, node_id, tool_name, server_id, args, status)
VALUES ($1, $2, $3, $4, $5, $6, 'running');

-- name: FinishToolInvocation :exec
UPDATE tool_invocations
SET status = $2, result = $3, error = $4,
    duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (now() - started_at)) * 1000)::int)
WHERE id = $1;

-- name: ListToolInvocations :many
SELECT * FROM tool_invocations
WHERE (sqlc.narg('task_id')::text IS NULL OR task_id = sqlc.narg('task_id')::text)
ORDER BY started_at DESC LIMIT sqlc.arg('limit')::int;

-- name: ListMCPServers :many
SELECT * FROM mcp_servers ORDER BY created_at;

-- name: UpsertMCPServer :one
INSERT INTO mcp_servers (id, name, command, args, env, status, approved)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (name) DO UPDATE SET command = EXCLUDED.command, args = EXCLUDED.args,
    env = EXCLUDED.env, status = EXCLUDED.status, approved = EXCLUDED.approved
RETURNING *;

-- name: SetMCPServerStatus :exec
UPDATE mcp_servers SET status = $2 WHERE id = $1;

-- name: CreateRun :one
INSERT INTO research_runs (id, task_id, run_number, trigger, status)
VALUES (
    $1, $2,
    (SELECT COALESCE(MAX(run_number), 0) + 1 FROM research_runs WHERE task_id = $2),
    $3, 'running'
)
RETURNING *;

-- name: FinishRun :exec
UPDATE research_runs SET status = $2, finished_at = now() WHERE id = $1;

-- name: GetCurrentRun :one
SELECT * FROM research_runs WHERE task_id = $1 ORDER BY run_number DESC LIMIT 1;

-- name: GetSetting :one
SELECT value FROM app_settings WHERE key = $1;

-- name: SetSetting :exec
INSERT INTO app_settings (key, value, updated_at) VALUES ($1, $2, now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();
