-- name: CreateBudget :one
INSERT INTO budgets (
  id,
  org_id,
  name,
  scope_type,
  scope_id,
  period,
  limit_micro_usd,
  action,
  status,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: GetBudget :one
SELECT *
FROM budgets
WHERE org_id = $1 AND id = $2;

-- name: ListBudgets :many
SELECT *
FROM budgets
WHERE org_id = $1
ORDER BY created_at DESC, id DESC;

-- name: GetBudgetUsedAmount :one
SELECT COALESCE(SUM(total_cost_micro), 0)::bigint AS used_micro_usd
FROM cost_records
WHERE org_id = $1
  AND created_at >= $2
  AND created_at < $3;
