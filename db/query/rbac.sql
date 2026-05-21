-- name: UpsertUserByEmail :one
INSERT INTO users (
  id,
  email,
  display_name,
  status,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6
)
ON CONFLICT (email)
DO UPDATE SET
  display_name = EXCLUDED.display_name,
  status = CASE WHEN users.status = 'disabled' THEN 'active' ELSE users.status END,
  updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1;

-- name: UpsertOrgMembership :one
INSERT INTO org_memberships (
  id,
  org_id,
  user_id,
  role,
  status,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (org_id, user_id)
DO UPDATE SET
  role = EXCLUDED.role,
  status = EXCLUDED.status,
  updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: ListOrgMembers :many
SELECT
  m.id AS membership_id,
  m.org_id,
  m.user_id,
  u.email,
  u.display_name,
  m.role,
  m.status,
  m.created_at,
  m.updated_at
FROM org_memberships m
JOIN users u ON u.id = m.user_id
WHERE m.org_id = $1
ORDER BY m.created_at DESC, m.id DESC;

-- name: GetMembershipByOrgAndUser :one
SELECT *
FROM org_memberships
WHERE org_id = $1 AND user_id = $2;

-- name: GetActiveMembershipByOrgSlugAndEmail :one
SELECT
  o.id AS org_id,
  o.slug AS org_slug,
  u.id AS user_id,
  u.email,
  u.display_name,
  m.id AS membership_id,
  m.role
FROM organizations o
JOIN org_memberships m ON m.org_id = o.id
JOIN users u ON u.id = m.user_id
WHERE o.slug = sqlc.arg('org_slug')::text
  AND lower(u.email) = lower(sqlc.arg('email')::text)
  AND o.status = 'active'
  AND u.status = 'active'
  AND m.status = 'active';

-- name: CreateUserSession :one
INSERT INTO user_sessions (
  id,
  org_id,
  user_id,
  token_prefix,
  token_hash,
  status,
  expires_at,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetUserSessionByHash :one
SELECT
  s.id,
  s.org_id,
  s.user_id,
  s.token_prefix,
  s.token_hash,
  s.status,
  s.expires_at,
  s.last_used_at,
  s.created_at,
  s.revoked_at,
  m.id AS membership_id,
  m.role AS membership_role,
  m.status AS membership_status,
  u.email AS user_email,
  u.status AS user_status
FROM user_sessions s
JOIN org_memberships m
  ON m.org_id = s.org_id
 AND m.user_id = s.user_id
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1;

-- name: TouchUserSessionLastUsed :exec
UPDATE user_sessions
SET last_used_at = $3
WHERE org_id = $1 AND id = $2;

-- name: RevokeUserSession :one
UPDATE user_sessions
SET status = 'revoked',
    revoked_at = $3
WHERE org_id = $1 AND id = $2
RETURNING *;
