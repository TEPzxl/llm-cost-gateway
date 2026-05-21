-- name: ProviderModelLatencyStats :many
SELECT
  provider_id,
  model_id,
  count(*)::bigint AS request_count,
  COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY latency_ms), 0)::double precision AS p50_latency_ms,
  COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms), 0)::double precision AS p95_latency_ms
FROM request_logs
WHERE org_id = $1
  AND completed_at >= $2
  AND completed_at < $3
  AND provider_id IS NOT NULL
  AND model_id IS NOT NULL
  AND status IN ('success', 'budget_warned')
GROUP BY provider_id, model_id;
