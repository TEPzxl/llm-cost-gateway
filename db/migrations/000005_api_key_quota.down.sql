ALTER TABLE api_keys
  DROP COLUMN quota_action,
  DROP COLUMN monthly_cost_limit_micro_usd,
  DROP COLUMN daily_cost_limit_micro_usd;
