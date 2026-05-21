-- name: CreateMagicLinkToken :one
INSERT INTO magic_link_tokens (
  id,
  org_id,
  user_id,
  email,
  token_prefix,
  token_hash,
  status,
  expires_at,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: ConsumeMagicLinkToken :one
UPDATE magic_link_tokens ml
SET status = 'consumed',
    consumed_at = sqlc.arg('consumed_at')::timestamptz
FROM org_memberships m
JOIN users u ON u.id = m.user_id
JOIN organizations o ON o.id = m.org_id
WHERE ml.token_hash = sqlc.arg('token_hash')::text
  AND ml.org_id = m.org_id
  AND ml.user_id = m.user_id
  AND ml.status = 'active'
  AND ml.consumed_at IS NULL
  AND ml.expires_at > sqlc.arg('now')::timestamptz
  AND m.status = 'active'
  AND u.status = 'active'
  AND o.status = 'active'
RETURNING
  ml.id,
  ml.org_id,
  o.slug AS org_slug,
  ml.user_id,
  u.email,
  u.display_name,
  m.id AS membership_id,
  m.role,
  ml.expires_at,
  ml.consumed_at;
