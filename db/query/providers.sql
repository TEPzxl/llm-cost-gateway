-- name: CreateProvider :one
INSERT INTO providers (
  id,
  org_id,
  name,
  type,
  base_url,
  timeout_ms,
  status,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetProvider :one
SELECT *
FROM providers
WHERE org_id = $1 AND id = $2;

-- name: ListProviders :many
SELECT *
FROM providers
WHERE org_id = $1
ORDER BY created_at DESC, id DESC;

-- name: UpdateProviderHealth :one
UPDATE providers
SET
  last_health_status = $3,
  last_health_checked_at = $4,
  last_error_code = $5,
  last_error_message = $6,
  updated_at = $7
WHERE org_id = $1 AND id = $2
RETURNING *;
