-- name: CreateContentPolicy :one
INSERT INTO content_policies (
  id,
  org_id,
  name,
  pii_action,
  status,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: ListContentPolicies :many
SELECT *
FROM content_policies
WHERE org_id = $1
ORDER BY created_at DESC, id DESC;

-- name: GetActiveContentPolicy :one
SELECT *
FROM content_policies
WHERE org_id = $1
  AND status = 'active'
ORDER BY created_at DESC, id DESC
LIMIT 1;
