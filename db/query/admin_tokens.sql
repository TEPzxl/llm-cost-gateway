-- name: CreateAdminToken :one
INSERT INTO admin_tokens (
  id,
  org_id,
  name,
  token_prefix,
  token_hash,
  scopes,
  status,
  expires_at,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetAdminTokenByHash :one
SELECT *
FROM admin_tokens
WHERE token_hash = $1;

-- name: GetAdminToken :one
SELECT *
FROM admin_tokens
WHERE org_id = $1 AND id = $2;

-- name: ListAdminTokens :many
SELECT *
FROM admin_tokens
WHERE org_id = $1
ORDER BY created_at DESC, id DESC;

-- name: RevokeAdminToken :one
UPDATE admin_tokens
SET status = 'revoked',
    revoked_at = $3
WHERE org_id = $1 AND id = $2
RETURNING *;

-- name: TouchAdminTokenLastUsed :exec
UPDATE admin_tokens
SET last_used_at = $3
WHERE org_id = $1 AND id = $2;
