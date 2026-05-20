-- name: CreateBudgetAlert :one
INSERT INTO budget_alerts (
  id,
  org_id,
  budget_id,
  webhook_url,
  webhook_secret,
  status,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: ListBudgetAlerts :many
SELECT *
FROM budget_alerts
WHERE org_id = $1
ORDER BY created_at DESC, id DESC;

-- name: ListActiveBudgetAlerts :many
SELECT *
FROM budget_alerts
WHERE org_id = $1 AND status = 'active'
ORDER BY created_at ASC, id ASC;

-- name: GetBudgetAlert :one
SELECT *
FROM budget_alerts
WHERE org_id = $1 AND id = $2;

-- name: ListBudgetAlertDeliveries :many
SELECT *
FROM budget_alert_deliveries
WHERE org_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: ListBudgetAlertDeliveriesByBudgetWindow :many
SELECT *
FROM budget_alert_deliveries
WHERE org_id = $1
  AND budget_id = $2
  AND period_window_start = $3
ORDER BY threshold ASC;

-- name: InsertBudgetAlertDelivery :one
INSERT INTO budget_alert_deliveries (
  id,
  org_id,
  budget_alert_id,
  budget_id,
  threshold,
  period,
  period_window_start,
  period_window_end,
  used_micro_usd,
  limit_micro_usd,
  webhook_url,
  status,
  http_status,
  error_message,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
) RETURNING *;
