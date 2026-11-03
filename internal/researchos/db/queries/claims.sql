-- 结论（Claim）与证据（Evidence）

-- name: InsertClaim :one
INSERT INTO claims (
    id, task_id, node_id, statement, normalized_key, status, confidence,
    conflict_group, verifier_notes, source_count, numeric_value
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: UpdateClaimVerification :exec
UPDATE claims
SET status = $2, confidence = $3, conflict_group = $4, verifier_notes = $5,
    source_count = $6, updated_at = now()
WHERE task_id = $7 AND id = $1;

-- name: ListClaims :many
SELECT * FROM claims WHERE task_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY confidence DESC, created_at
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: GetClaim :one
SELECT * FROM claims WHERE task_id = $1 AND id = $2;

-- name: CountClaims :one
SELECT COUNT(*) FROM claims WHERE task_id = $1;

-- name: InsertEvidence :one
INSERT INTO evidence (id, task_id, claim_id, source_id, node_id, quote, stance, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (claim_id, source_id) DO UPDATE SET
    quote = CASE WHEN EXCLUDED.quote <> '' THEN EXCLUDED.quote ELSE evidence.quote END,
    stance = EXCLUDED.stance,
    confidence = EXCLUDED.confidence
RETURNING *;

-- name: ListEvidenceByTask :many
SELECT * FROM evidence WHERE task_id = $1 ORDER BY created_at LIMIT 500;

-- name: ListEvidenceByClaim :many
SELECT * FROM evidence WHERE claim_id = $1 ORDER BY confidence DESC;

-- name: SetClaimSourceCount :exec
UPDATE claims SET source_count = sqlc.arg('source_count')::int, updated_at = now()
WHERE id = sqlc.arg('id')::text;
