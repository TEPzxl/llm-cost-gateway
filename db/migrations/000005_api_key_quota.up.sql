ALTER TABLE api_keys
  ADD COLUMN daily_cost_limit_micro_usd bigint NULL
    CHECK (daily_cost_limit_micro_usd IS NULL OR daily_cost_limit_micro_usd >= 0),
  ADD COLUMN monthly_cost_limit_micro_usd bigint NULL
    CHECK (monthly_cost_limit_micro_usd IS NULL OR monthly_cost_limit_micro_usd >= 0),
  ADD COLUMN quota_action text NOT NULL DEFAULT 'block'
    CHECK (quota_action IN ('warn', 'block'));
