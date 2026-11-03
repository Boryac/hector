-- 资料（Source）

-- name: UpsertSource :one
INSERT INTO sources (
    id, task_id, node_id, url, normalized_url, title, site, provider,
    snippet, excerpt, content_hash, http_status, credibility, status, error, published_at, fetched_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14, $15, $16, $17
)
ON CONFLICT (task_id, normalized_url) DO UPDATE SET
    title = CASE WHEN EXCLUDED.title <> '' THEN EXCLUDED.title ELSE sources.title END,
    excerpt = CASE WHEN length(EXCLUDED.excerpt) > length(sources.excerpt) THEN EXCLUDED.excerpt ELSE sources.excerpt END,
    snippet = CASE WHEN EXCLUDED.snippet <> '' THEN EXCLUDED.snippet ELSE sources.snippet END,
    content_hash = CASE WHEN EXCLUDED.content_hash <> '' THEN EXCLUDED.content_hash ELSE sources.content_hash END,
    http_status = CASE WHEN EXCLUDED.http_status > 0 THEN EXCLUDED.http_status ELSE sources.http_status END,
    status = EXCLUDED.status,
    error = EXCLUDED.error,
    provider = CASE WHEN EXCLUDED.provider <> '' THEN EXCLUDED.provider ELSE sources.provider END,
    node_id = CASE WHEN sources.node_id = '' THEN EXCLUDED.node_id ELSE sources.node_id END,
    published_at = COALESCE(EXCLUDED.published_at, sources.published_at),
    fetched_at = COALESCE(EXCLUDED.fetched_at, sources.fetched_at)
RETURNING *;

-- name: ListSources :many
SELECT s.*, COUNT(e.id)::int AS evidence_count
FROM sources s
LEFT JOIN evidence e ON e.source_id = s.id
WHERE s.task_id = $1
  AND (sqlc.narg('site')::text IS NULL OR s.site = sqlc.narg('site')::text)
  AND (sqlc.narg('credibility')::text IS NULL OR s.credibility = sqlc.narg('credibility')::text)
  AND (sqlc.narg('query')::text IS NULL OR s.title ILIKE '%' || sqlc.narg('query')::text || '%' OR s.url ILIKE '%' || sqlc.narg('query')::text || '%')
GROUP BY s.id
ORDER BY s.created_at
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: GetSource :one
SELECT * FROM sources WHERE task_id = $1 AND id = $2;

-- name: CountSources :one
SELECT COUNT(*) FROM sources WHERE task_id = $1;

-- name: ListTaskSourceURLs :many
SELECT url, normalized_url, status FROM sources WHERE task_id = $1;

-- name: ListSourcesByNode :many
SELECT * FROM sources
WHERE task_id = sqlc.arg('task_id') AND node_id = sqlc.arg('node_id')
ORDER BY created_at;

-- name: ListSourcesByIDs :many
SELECT * FROM sources
WHERE task_id = sqlc.arg('task_id') AND id = ANY(sqlc.arg('ids')::text[])
ORDER BY created_at;

-- name: ListClaimsFull :many
SELECT * FROM claims WHERE task_id = $1 ORDER BY created_at;
