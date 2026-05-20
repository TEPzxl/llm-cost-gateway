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
  daily_cost_limit_micro_usd,
  monthly_cost_limit_micro_usd,
  quota_action,
  expires_at,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
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

-- name: GetAPIKeyCostUsedAmount :one
SELECT COALESCE(SUM(cr.total_cost_micro), 0)::bigint AS used_micro_usd
FROM cost_records cr
JOIN usage_records ur
  ON ur.org_id = cr.org_id
 AND ur.id = cr.usage_record_id
WHERE cr.org_id = $1
  AND ur.api_key_id = $2
  AND cr.created_at >= $3
  AND cr.created_at < $4;
