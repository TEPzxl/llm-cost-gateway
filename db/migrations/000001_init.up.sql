CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE organizations (
  id uuid PRIMARY KEY,
  name text NOT NULL CHECK (name <> ''),
  slug text NOT NULL UNIQUE CHECK (slug <> ''),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
CREATE INDEX organizations_status_idx ON organizations(status);

CREATE TABLE admin_tokens (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL CHECK (name <> ''),
  token_prefix text NOT NULL CHECK (token_prefix <> ''),
  token_hash text NOT NULL UNIQUE CHECK (token_hash <> ''),
  scopes text[] NOT NULL DEFAULT '{}',
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
  expires_at timestamptz NULL,
  last_used_at timestamptz NULL,
  created_at timestamptz NOT NULL,
  revoked_at timestamptz NULL,
  UNIQUE(org_id, id)
);
CREATE INDEX admin_tokens_org_idx ON admin_tokens(org_id);
CREATE INDEX admin_tokens_prefix_idx ON admin_tokens(token_prefix);
CREATE INDEX admin_tokens_org_status_idx ON admin_tokens(org_id, status);

CREATE TABLE api_keys (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL CHECK (name <> ''),
  key_prefix text NOT NULL CHECK (key_prefix <> ''),
  key_hash text NOT NULL UNIQUE CHECK (key_hash <> ''),
  scopes text[] NOT NULL DEFAULT '{}',
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
  rpm_limit integer NOT NULL DEFAULT 60 CHECK (rpm_limit > 0),
  expires_at timestamptz NULL,
  last_used_at timestamptz NULL,
  created_at timestamptz NOT NULL,
  revoked_at timestamptz NULL,
  UNIQUE(org_id, id)
);
CREATE INDEX api_keys_org_idx ON api_keys(org_id);
CREATE INDEX api_keys_prefix_idx ON api_keys(key_prefix);
CREATE INDEX api_keys_org_status_idx ON api_keys(org_id, status);

CREATE TABLE providers (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL CHECK (name <> ''),
  type text NOT NULL CHECK (type IN ('mock', 'openai_compatible')),
  base_url text NULL,
  timeout_ms integer NOT NULL DEFAULT 30000 CHECK (timeout_ms > 0),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  UNIQUE(org_id, name),
  CHECK (type = 'mock' OR base_url IS NOT NULL)
);
CREATE INDEX providers_org_idx ON providers(org_id);
CREATE INDEX providers_org_status_idx ON providers(org_id, status);
CREATE INDEX providers_type_idx ON providers(type);

CREATE TABLE provider_secrets (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  provider_id uuid NOT NULL,
  encrypted_api_key text NOT NULL CHECK (encrypted_api_key <> ''),
  nonce text NOT NULL CHECK (nonce <> ''),
  key_version integer NOT NULL DEFAULT 1 CHECK (key_version > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  UNIQUE(org_id, provider_id),
  FOREIGN KEY (org_id, provider_id) REFERENCES providers(org_id, id)
);
CREATE INDEX provider_secrets_org_idx ON provider_secrets(org_id);
CREATE INDEX provider_secrets_provider_idx ON provider_secrets(provider_id);

CREATE TABLE models (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  provider_id uuid NOT NULL,
  provider_model_name text NOT NULL CHECK (provider_model_name <> ''),
  display_name text NOT NULL CHECK (display_name <> ''),
  input_price_micro_usd_per_1k_tokens bigint NOT NULL CHECK (input_price_micro_usd_per_1k_tokens >= 0),
  output_price_micro_usd_per_1k_tokens bigint NOT NULL CHECK (output_price_micro_usd_per_1k_tokens >= 0),
  context_window integer NULL CHECK (context_window IS NULL OR context_window > 0),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  UNIQUE(org_id, provider_id, id),
  UNIQUE(org_id, provider_id, provider_model_name),
  FOREIGN KEY (org_id, provider_id) REFERENCES providers(org_id, id)
);
CREATE INDEX models_org_idx ON models(org_id);
CREATE INDEX models_provider_idx ON models(provider_id);
CREATE INDEX models_org_status_idx ON models(org_id, status);

CREATE TABLE route_policies (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL CHECK (name <> ''),
  match_model text NOT NULL CHECK (match_model <> ''),
  strategy text NOT NULL DEFAULT 'single' CHECK (strategy IN ('single')),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  UNIQUE(org_id, match_model)
);
CREATE INDEX route_policies_org_idx ON route_policies(org_id);
CREATE INDEX route_policies_match_idx ON route_policies(org_id, match_model);
CREATE INDEX route_policies_status_idx ON route_policies(status);

CREATE TABLE route_targets (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  route_policy_id uuid NOT NULL,
  provider_id uuid NOT NULL,
  model_id uuid NOT NULL,
  priority integer NOT NULL DEFAULT 1 CHECK (priority > 0),
  weight integer NOT NULL DEFAULT 100 CHECK (weight > 0),
  created_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  UNIQUE(org_id, route_policy_id, priority),
  FOREIGN KEY (org_id, route_policy_id) REFERENCES route_policies(org_id, id),
  FOREIGN KEY (org_id, provider_id) REFERENCES providers(org_id, id),
  FOREIGN KEY (org_id, provider_id, model_id) REFERENCES models(org_id, provider_id, id)
);
CREATE INDEX route_targets_org_idx ON route_targets(org_id);
CREATE INDEX route_targets_policy_idx ON route_targets(route_policy_id);
CREATE INDEX route_targets_provider_idx ON route_targets(provider_id);
CREATE INDEX route_targets_model_idx ON route_targets(model_id);

CREATE TABLE budgets (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL CHECK (name <> ''),
  scope_type text NOT NULL CHECK (scope_type IN ('org')),
  scope_id uuid NULL,
  period text NOT NULL CHECK (period IN ('daily', 'monthly')),
  limit_micro_usd bigint NOT NULL CHECK (limit_micro_usd >= 0),
  action text NOT NULL DEFAULT 'block' CHECK (action IN ('warn', 'block')),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  CHECK (scope_type <> 'org' OR scope_id IS NULL)
);
CREATE INDEX budgets_org_idx ON budgets(org_id);
CREATE INDEX budgets_org_status_idx ON budgets(org_id, status);
CREATE INDEX budgets_scope_idx ON budgets(org_id, scope_type, scope_id);

CREATE TABLE request_logs (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  api_key_id uuid NULL,
  provider_id uuid NULL,
  model_id uuid NULL,
  route_policy_id uuid NULL,
  method text NOT NULL CHECK (method <> ''),
  path text NOT NULL CHECK (path <> ''),
  request_model text NULL,
  status text NOT NULL CHECK (status IN ('success', 'error', 'rate_limited', 'budget_warned', 'budget_blocked')),
  status_code integer NOT NULL CHECK (status_code BETWEEN 100 AND 599),
  error_code text NULL,
  request_hash text NULL,
  response_hash text NULL,
  latency_ms integer NOT NULL CHECK (latency_ms >= 0),
  provider_latency_ms integer NULL CHECK (provider_latency_ms IS NULL OR provider_latency_ms >= 0),
  metadata jsonb NULL,
  started_at timestamptz NOT NULL,
  completed_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  UNIQUE(org_id, api_key_id, provider_id, model_id, id),
  FOREIGN KEY (org_id, api_key_id) REFERENCES api_keys(org_id, id),
  FOREIGN KEY (org_id, provider_id) REFERENCES providers(org_id, id),
  FOREIGN KEY (org_id, provider_id, model_id) REFERENCES models(org_id, provider_id, id),
  FOREIGN KEY (org_id, route_policy_id) REFERENCES route_policies(org_id, id),
  CHECK ((provider_id IS NULL AND model_id IS NULL) OR (provider_id IS NOT NULL AND model_id IS NOT NULL)),
  CHECK (completed_at >= started_at)
);
CREATE INDEX request_logs_org_time_idx ON request_logs(org_id, started_at DESC);
CREATE INDEX request_logs_api_key_time_idx ON request_logs(api_key_id, started_at DESC);
CREATE INDEX request_logs_provider_time_idx ON request_logs(provider_id, started_at DESC);
CREATE INDEX request_logs_status_idx ON request_logs(status);
CREATE INDEX request_logs_error_code_idx ON request_logs(error_code);

CREATE TABLE usage_records (
  id uuid PRIMARY KEY,
  request_log_id uuid NOT NULL UNIQUE,
  org_id uuid NOT NULL REFERENCES organizations(id),
  api_key_id uuid NOT NULL,
  provider_id uuid NOT NULL,
  model_id uuid NOT NULL,
  prompt_tokens integer NOT NULL CHECK (prompt_tokens >= 0),
  completion_tokens integer NOT NULL CHECK (completion_tokens >= 0),
  total_tokens integer NOT NULL CHECK (total_tokens >= 0),
  provider_usage_json jsonb NULL,
  created_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  UNIQUE(org_id, request_log_id),
  UNIQUE(org_id, provider_id, model_id, id),
  FOREIGN KEY (org_id, api_key_id) REFERENCES api_keys(org_id, id),
  FOREIGN KEY (org_id, provider_id) REFERENCES providers(org_id, id),
  FOREIGN KEY (org_id, provider_id, model_id) REFERENCES models(org_id, provider_id, id),
  FOREIGN KEY (org_id, api_key_id, provider_id, model_id, request_log_id)
    REFERENCES request_logs(org_id, api_key_id, provider_id, model_id, id),
  CHECK (total_tokens = prompt_tokens + completion_tokens)
);
CREATE INDEX usage_records_org_time_idx ON usage_records(org_id, created_at DESC);
CREATE INDEX usage_records_api_key_time_idx ON usage_records(api_key_id, created_at DESC);
CREATE INDEX usage_records_provider_time_idx ON usage_records(provider_id, created_at DESC);
CREATE INDEX usage_records_model_time_idx ON usage_records(model_id, created_at DESC);

CREATE TABLE cost_records (
  id uuid PRIMARY KEY,
  usage_record_id uuid NOT NULL UNIQUE,
  org_id uuid NOT NULL REFERENCES organizations(id),
  provider_id uuid NOT NULL,
  model_id uuid NOT NULL,
  currency text NOT NULL DEFAULT 'USD' CHECK (currency = 'USD'),
  input_cost_micro bigint NOT NULL CHECK (input_cost_micro >= 0),
  output_cost_micro bigint NOT NULL CHECK (output_cost_micro >= 0),
  total_cost_micro bigint NOT NULL CHECK (total_cost_micro >= 0),
  pricing_snapshot jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  FOREIGN KEY (org_id, provider_id, model_id, usage_record_id)
    REFERENCES usage_records(org_id, provider_id, model_id, id),
  CHECK (total_cost_micro = input_cost_micro + output_cost_micro)
);
CREATE INDEX cost_records_org_time_idx ON cost_records(org_id, created_at DESC);
CREATE INDEX cost_records_provider_time_idx ON cost_records(provider_id, created_at DESC);
CREATE INDEX cost_records_model_time_idx ON cost_records(model_id, created_at DESC);
CREATE INDEX cost_records_org_total_idx ON cost_records(org_id, total_cost_micro);
