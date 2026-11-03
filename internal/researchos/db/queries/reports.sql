-- 报告（多版本）

-- name: InsertReport :one
INSERT INTO reports (
    id, task_id, version, title, summary, markdown, sections, citations,
    citation_count, word_count, model
) VALUES (
    $1, $2,
    (SELECT COALESCE(MAX(version), 0) + 1 FROM reports WHERE task_id = $2),
    $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: GetLatestReport :one
SELECT * FROM reports WHERE task_id = $1 ORDER BY version DESC LIMIT 1;

-- name: GetReportVersion :one
SELECT * FROM reports WHERE task_id = $1 AND version = $2;

-- name: GetReportByID :one
SELECT * FROM reports WHERE task_id = $1 AND id = $2;

-- name: ListReportVersions :many
SELECT id, task_id, version, title, summary, citation_count, word_count, model, created_at
FROM reports WHERE task_id = $1 ORDER BY version DESC;
