-- name: InsertCacheEvent :one
INSERT INTO cache_events (
  id,
  org_id,
  request_log_id,
  event_type,
  requested_model,
  cache_key_hash,
  messages_hash,
  reason,
  metadata,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: ListCacheEvents :many
SELECT *
FROM cache_events
WHERE org_id = sqlc.arg('org_id')
  AND created_at >= sqlc.arg('from_at')
  AND created_at < sqlc.arg('to_at')
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type')::text)
  AND (sqlc.narg('requested_model')::text IS NULL OR requested_model = sqlc.narg('requested_model')::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit');
