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
