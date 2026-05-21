CREATE TABLE cost_anomaly_policies (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL CHECK (name <> ''),
  rule_type text NOT NULL CHECK (rule_type IN ('daily_cost', 'api_key_cost_spike', 'model_cost_spike', 'error_rate_spike')),
  scope_type text NOT NULL CHECK (scope_type IN ('org', 'api_key', 'model')),
  scope_id uuid NULL,
  model_alias text NULL,
  threshold_micro_usd bigint NULL CHECK (threshold_micro_usd IS NULL OR threshold_micro_usd >= 0),
  threshold_bps integer NULL CHECK (threshold_bps IS NULL OR threshold_bps > 0),
  spike_multiplier_bps integer NOT NULL DEFAULT 20000 CHECK (spike_multiplier_bps > 0),
  current_window_minutes integer NOT NULL DEFAULT 60 CHECK (current_window_minutes > 0),
  baseline_window_minutes integer NOT NULL DEFAULT 1440 CHECK (baseline_window_minutes > 0),
  min_requests integer NOT NULL DEFAULT 1 CHECK (min_requests > 0),
  action text NOT NULL CHECK (action IN ('notify', 'downgrade', 'block')),
  fallback_model text NULL,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  FOREIGN KEY (org_id, scope_id) REFERENCES api_keys(org_id, id),
  CHECK (
    (scope_type = 'org' AND scope_id IS NULL AND model_alias IS NULL)
    OR (scope_type = 'api_key' AND scope_id IS NOT NULL AND model_alias IS NULL)
    OR (scope_type = 'model' AND scope_id IS NULL AND model_alias IS NOT NULL)
  ),
  CHECK (
    (rule_type = 'daily_cost' AND threshold_micro_usd IS NOT NULL)
    OR (rule_type IN ('api_key_cost_spike', 'model_cost_spike'))
    OR (rule_type = 'error_rate_spike' AND threshold_bps IS NOT NULL)
  ),
  CHECK (action <> 'downgrade' OR fallback_model IS NOT NULL)
);

CREATE INDEX cost_anomaly_policies_org_idx ON cost_anomaly_policies(org_id);
CREATE INDEX cost_anomaly_policies_org_status_idx ON cost_anomaly_policies(org_id, status);
CREATE INDEX cost_anomaly_policies_scope_idx ON cost_anomaly_policies(org_id, scope_type, scope_id, model_alias);
