CREATE DATABASE IF NOT EXISTS llmgw;

CREATE TABLE IF NOT EXISTS llmgw.usage_events (
  request_id UUID,
  org_id UUID,
  api_key_id Nullable(UUID),
  provider_id Nullable(UUID),
  model_id Nullable(UUID),
  status LowCardinality(String),
  prompt_tokens UInt64,
  completion_tokens UInt64,
  total_tokens UInt64,
  total_cost_micro_usd Int64,
  latency_ms UInt32,
  created_at DateTime64(3, 'UTC')
) ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (org_id, created_at, request_id);
