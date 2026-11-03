-- 设置中心查询。
--
-- 说明：既有的 GetSetting / SetSetting 保留给旧调用方（迁移期不删），
-- 这里新增的查询携带 section / encrypted / updated_by。

-- name: GetSettingWithMeta :one
SELECT key, value, section, encrypted, updated_by, updated_at
FROM app_settings WHERE key = $1;

-- name: ListSettingRows :many
SELECT key, value, section, encrypted, updated_by, updated_at
FROM app_settings ORDER BY section, key;

-- name: UpsertSettingRow :exec
INSERT INTO app_settings (key, value, section, encrypted, updated_by, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (key) DO UPDATE SET
    value      = EXCLUDED.value,
    section    = EXCLUDED.section,
    encrypted  = EXCLUDED.encrypted,
    updated_by = EXCLUDED.updated_by,
    updated_at = now();

-- name: DeleteSettingRow :exec
DELETE FROM app_settings WHERE key = $1;

-- name: DeleteSettingsBySection :execrows
DELETE FROM app_settings WHERE section = $1;

-- name: UpdateMCPServerTools :exec
UPDATE mcp_servers
SET tools = $2, status = $3, last_error = $4, updated_at = now()
WHERE id = $1;

-- name: SetMCPServerApproved :exec
UPDATE mcp_servers
SET approved = $2, status = $3, updated_at = now()
WHERE id = $1;

-- name: DeleteMCPServer :exec
DELETE FROM mcp_servers WHERE id = $1;

-- name: ListMCPServersByStatus :many
SELECT * FROM mcp_servers ORDER BY name;
