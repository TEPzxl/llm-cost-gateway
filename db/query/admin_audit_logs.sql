-- name: InsertAdminAuditLog :one
INSERT INTO admin_audit_logs (
  id,
  org_id,
  actor_admin_token_id,
  action,
  resource_type,
  resource_id,
  request_id,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: ListAdminAuditLogs :many
SELECT *
FROM admin_audit_logs
WHERE org_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;
