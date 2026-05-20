-- name: InsertRequestLog :one
INSERT INTO request_logs (
  id,
  org_id,
  api_key_id,
  provider_id,
  model_id,
  route_policy_id,
  method,
  path,
  request_model,
  status,
  status_code,
  error_code,
  request_hash,
  response_hash,
  latency_ms,
  provider_latency_ms,
  metadata,
  started_at,
  completed_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
) RETURNING *;

-- name: ListRequestLogs :many
SELECT *
FROM request_logs
WHERE org_id = sqlc.arg('org_id')
  AND started_at >= sqlc.arg('from_at')
  AND started_at < sqlc.arg('to_at')
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('error_code')::text IS NULL OR error_code = sqlc.narg('error_code')::text)
  AND (sqlc.narg('request_model')::text IS NULL OR request_model = sqlc.narg('request_model')::text)
  AND (sqlc.narg('api_key_id')::uuid IS NULL OR api_key_id = sqlc.narg('api_key_id')::uuid)
  AND (sqlc.narg('provider_id')::uuid IS NULL OR provider_id = sqlc.narg('provider_id')::uuid)
  AND (sqlc.narg('model_id')::uuid IS NULL OR model_id = sqlc.narg('model_id')::uuid)
  AND (
    sqlc.narg('cursor_started_at')::timestamptz IS NULL
    OR (started_at, id) < (sqlc.narg('cursor_started_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY started_at DESC, id DESC
LIMIT sqlc.arg('limit');
