-- name: InsertUsageRecord :one
INSERT INTO usage_records (
  id,
  request_log_id,
  org_id,
  api_key_id,
  provider_id,
  model_id,
  prompt_tokens,
  completion_tokens,
  total_tokens,
  provider_usage_json,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: InsertCostRecord :one
INSERT INTO cost_records (
  id,
  usage_record_id,
  org_id,
  provider_id,
  model_id,
  currency,
  input_cost_micro,
  output_cost_micro,
  total_cost_micro,
  pricing_snapshot,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: UsageSummaryByProvider :many
SELECT
  ur.provider_id,
  count(*)::bigint AS request_count,
  count(*) FILTER (WHERE rl.status IN ('success', 'budget_warned'))::bigint AS success_count,
  count(*) FILTER (WHERE rl.status NOT IN ('success', 'budget_warned'))::bigint AS error_count,
  COALESCE(sum(ur.prompt_tokens), 0)::bigint AS prompt_tokens,
  COALESCE(sum(ur.completion_tokens), 0)::bigint AS completion_tokens,
  COALESCE(sum(ur.total_tokens), 0)::bigint AS total_tokens,
  COALESCE(sum(cr.total_cost_micro), 0)::bigint AS total_cost_micro_usd,
  COALESCE(avg(rl.latency_ms), 0)::double precision AS avg_latency_ms
FROM usage_records ur
JOIN request_logs rl
  ON rl.org_id = ur.org_id
 AND rl.id = ur.request_log_id
LEFT JOIN cost_records cr
  ON cr.org_id = ur.org_id
 AND cr.usage_record_id = ur.id
WHERE ur.org_id = $1
  AND ur.created_at >= $2
  AND ur.created_at < $3
GROUP BY ur.provider_id
ORDER BY total_cost_micro_usd DESC, request_count DESC;

-- name: UsageSummaryByModel :many
SELECT
  ur.model_id,
  count(*)::bigint AS request_count,
  count(*) FILTER (WHERE rl.status IN ('success', 'budget_warned'))::bigint AS success_count,
  count(*) FILTER (WHERE rl.status NOT IN ('success', 'budget_warned'))::bigint AS error_count,
  COALESCE(sum(ur.prompt_tokens), 0)::bigint AS prompt_tokens,
  COALESCE(sum(ur.completion_tokens), 0)::bigint AS completion_tokens,
  COALESCE(sum(ur.total_tokens), 0)::bigint AS total_tokens,
  COALESCE(sum(cr.total_cost_micro), 0)::bigint AS total_cost_micro_usd,
  COALESCE(avg(rl.latency_ms), 0)::double precision AS avg_latency_ms
FROM usage_records ur
JOIN request_logs rl
  ON rl.org_id = ur.org_id
 AND rl.id = ur.request_log_id
LEFT JOIN cost_records cr
  ON cr.org_id = ur.org_id
 AND cr.usage_record_id = ur.id
WHERE ur.org_id = $1
  AND ur.created_at >= $2
  AND ur.created_at < $3
GROUP BY ur.model_id
ORDER BY total_cost_micro_usd DESC, request_count DESC;

-- name: UsageSummaryByAPIKey :many
SELECT
  ur.api_key_id,
  count(*)::bigint AS request_count,
  count(*) FILTER (WHERE rl.status IN ('success', 'budget_warned'))::bigint AS success_count,
  count(*) FILTER (WHERE rl.status NOT IN ('success', 'budget_warned'))::bigint AS error_count,
  COALESCE(sum(ur.prompt_tokens), 0)::bigint AS prompt_tokens,
  COALESCE(sum(ur.completion_tokens), 0)::bigint AS completion_tokens,
  COALESCE(sum(ur.total_tokens), 0)::bigint AS total_tokens,
  COALESCE(sum(cr.total_cost_micro), 0)::bigint AS total_cost_micro_usd,
  COALESCE(avg(rl.latency_ms), 0)::double precision AS avg_latency_ms
FROM usage_records ur
JOIN request_logs rl
  ON rl.org_id = ur.org_id
 AND rl.id = ur.request_log_id
LEFT JOIN cost_records cr
  ON cr.org_id = ur.org_id
 AND cr.usage_record_id = ur.id
WHERE ur.org_id = $1
  AND ur.created_at >= $2
  AND ur.created_at < $3
GROUP BY ur.api_key_id
ORDER BY total_cost_micro_usd DESC, request_count DESC;
