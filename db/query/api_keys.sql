-- name: CreateAPIKey :one
INSERT INTO api_keys (
  id,
  org_id,
  name,
  key_prefix,
  key_hash,
  scopes,
  status,
  rpm_limit,
  expires_at,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: GetAPIKeyByHash :one
SELECT *
FROM api_keys
WHERE key_hash = $1;

-- name: GetAPIKey :one
SELECT *
FROM api_keys
WHERE org_id = $1 AND id = $2;

-- name: ListAPIKeys :many
SELECT *
FROM api_keys
WHERE org_id = $1
ORDER BY created_at DESC, id DESC;

-- name: RevokeAPIKey :one
UPDATE api_keys
SET status = 'revoked',
    revoked_at = $3
WHERE org_id = $1 AND id = $2
RETURNING *;

-- name: TouchAPIKeyLastUsed :exec
UPDATE api_keys
SET last_used_at = $3
WHERE org_id = $1 AND id = $2;
