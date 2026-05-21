-- name: CreateCostAnomalyPolicy :one
INSERT INTO cost_anomaly_policies (
  id,
  org_id,
  name,
  rule_type,
  scope_type,
  scope_id,
  model_alias,
  threshold_micro_usd,
  threshold_bps,
  spike_multiplier_bps,
  current_window_minutes,
  baseline_window_minutes,
  min_requests,
  action,
  fallback_model,
  status,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
) RETURNING *;

-- name: ListCostAnomalyPolicies :many
SELECT *
FROM cost_anomaly_policies
WHERE org_id = $1
ORDER BY created_at DESC, id DESC;

-- name: ListActiveCostAnomalyPolicies :many
SELECT *
FROM cost_anomaly_policies
WHERE org_id = $1 AND status = 'active'
ORDER BY created_at ASC, id ASC;
