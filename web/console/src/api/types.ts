export type ID = string;

export type ListResponse<T> = {
  items: T[];
};

export type AdminRole = "owner" | "admin" | "viewer" | string;

export type AdminMe = {
  org: {
    id: ID;
    name: string;
    slug: string;
  };
  actor_type: "service_token" | "user" | string;
  role: AdminRole;
  admin_token?: {
    id: ID;
    name: string;
    scopes: string[];
  } | null;
  user?: {
    id: ID;
    email: string;
    display_name: string;
    membership_id?: ID | null;
    session_id?: ID | null;
  } | null;
};

export type Member = {
  membership_id: ID;
  org_id: ID;
  user_id: ID;
  email: string;
  display_name: string;
  role: AdminRole;
  status: "active" | "disabled" | string;
  created_at: string;
  updated_at: string;
};

export type CreateMemberRequest = {
  email: string;
  display_name: string;
  role: "owner" | "admin" | "viewer";
};

export type PasswordlessMockLoginRequest = {
  org_slug: string;
  email: string;
};

export type PasswordlessMagicLinkRequest = {
  org_slug: string;
  email: string;
};

export type PasswordlessMagicLinkRequestResponse = {
  status: "accepted" | string;
};

export type PasswordlessMagicLinkVerifyRequest = {
  token: string;
};

export type PasswordlessMockLoginResponse = {
  token: string;
  token_type: "session" | string;
  expires_at: string;
  org_id: ID;
  org_slug: string;
  user_id: ID;
  email: string;
  display_name: string;
  membership_id: ID;
  role: AdminRole;
};

export type AnomalyRuleType =
  | "daily_cost"
  | "api_key_cost_spike"
  | "model_cost_spike"
  | "error_rate_spike"
  | string;

export type AnomalyPolicy = {
  id: ID;
  name: string;
  rule_type: AnomalyRuleType;
  scope_type: "org" | "api_key" | "model" | string;
  scope_id?: ID | null;
  model_alias?: string | null;
  threshold_micro_usd?: number | null;
  threshold_bps?: number | null;
  spike_multiplier_bps: number;
  current_window_minutes: number;
  baseline_window_minutes: number;
  min_requests: number;
  action: "notify" | "downgrade" | "block" | string;
  fallback_model?: string | null;
  status: "active" | "disabled" | string;
  created_at: string;
  updated_at: string;
};

export type CreateAnomalyPolicyRequest = {
  name: string;
  rule_type: "daily_cost" | "api_key_cost_spike" | "model_cost_spike" | "error_rate_spike";
  scope_type: "org" | "api_key" | "model";
  scope_id?: ID | null;
  model_alias?: string;
  threshold_micro_usd?: number | null;
  threshold_bps?: number | null;
  spike_multiplier_bps?: number;
  current_window_minutes?: number;
  baseline_window_minutes?: number;
  min_requests?: number;
  action: "notify" | "downgrade" | "block";
  fallback_model?: string;
};

export type APIKey = {
  id: ID;
  name: string;
  key_prefix: string;
  status: string;
  rpm_limit: number;
  daily_cost_limit_micro_usd?: number | null;
  monthly_cost_limit_micro_usd?: number | null;
  quota_action: "warn" | "block" | string;
  expires_at?: string | null;
  last_used_at?: string | null;
  created_at: string;
};

export type CreateAPIKeyRequest = {
  name: string;
  scopes: string[];
  rpm_limit: number;
  daily_cost_limit_micro_usd?: number | null;
  monthly_cost_limit_micro_usd?: number | null;
  quota_action: "warn" | "block";
  expires_at?: string | null;
};

export type APIKeyCreateResponse = APIKey & {
  key: string;
};

export type Provider = {
  id: ID;
  name: string;
  type: "mock" | "openai_compatible" | string;
  base_url?: string | null;
  api_key?: string | null;
  status: string;
  timeout_ms: number;
  last_health_status?: "healthy" | "unhealthy" | string | null;
  last_health_checked_at?: string | null;
  last_error_code?: string | null;
  last_error_message?: string | null;
  created_at: string;
};

export type CreateProviderRequest = {
  name: string;
  type: "mock" | "openai_compatible";
  base_url?: string | null;
  api_key?: string | null;
  timeout_ms: number;
};

export type Model = {
  id: ID;
  provider_id: ID;
  provider_model_name: string;
  display_name: string;
  input_price_micro_usd_per_1k_tokens: number;
  output_price_micro_usd_per_1k_tokens: number;
  context_window?: number | null;
  status: string;
  created_at: string;
};

export type CreateModelRequest = {
  provider_id: ID;
  provider_model_name: string;
  display_name: string;
  input_price_micro_usd_per_1k_tokens: number;
  output_price_micro_usd_per_1k_tokens: number;
  context_window?: number | null;
};

export type RoutePolicy = {
  id: ID;
  name: string;
  match_model: string;
  strategy: "single" | "fallback" | "lowest_cost" | "lowest_latency" | string;
  config?: RoutePolicyConfig;
  status: string;
  created_at: string;
};

export type RoutePolicyConfig = {
  max_estimated_cost_micro_usd?: number;
  fallback_to_priority?: boolean;
  latency_window_minutes?: number;
};

export type CreateRoutePolicyRequest = {
  name: string;
  match_model: string;
  strategy: "single" | "fallback" | "lowest_cost" | "lowest_latency";
  config?: RoutePolicyConfig;
  targets: Array<{
    provider_id: ID;
    model_id: ID;
    priority: number;
    weight: number;
  }>;
};

export type Budget = {
  id: ID;
  name: string;
  scope_type: "org";
  period: "daily" | "monthly";
  limit_micro_usd: number;
  action: "warn" | "block";
  status: string;
};

export type CreateBudgetRequest = {
  name: string;
  scope_type: "org";
  scope_id?: null;
  period: "daily" | "monthly";
  limit_micro_usd: number;
  action: "warn" | "block";
};

export type BudgetStatus = {
  budget_id: ID;
  name: string;
  period: "daily" | "monthly";
  limit_micro_usd: number;
  used_micro_usd: number;
  remaining_micro_usd: number;
  exceeded: boolean;
  action: "warn" | "block";
};

export type BudgetAlert = {
  id: ID;
  budget_id: ID;
  webhook_url: string;
  status: "active" | "disabled" | string;
  created_at: string;
};

export type CreateBudgetAlertRequest = {
  budget_id: ID;
  webhook_url: string;
  webhook_secret?: string;
  status: "active" | "disabled";
};

export type BudgetAlertDelivery = {
  id: ID;
  budget_alert_id: ID;
  budget_id: ID;
  threshold: number;
  period: "daily" | "monthly" | string;
  period_window_start: string;
  period_window_end: string;
  used_micro_usd: number;
  limit_micro_usd: number;
  webhook_url: string;
  status: "success" | "failed" | string;
  http_status?: number | null;
  error_message?: string | null;
  created_at: string;
};

export type AdminAuditLog = {
  id: ID;
  actor_admin_token_id?: ID | null;
  actor_user_id?: ID | null;
  action: string;
  resource_type: string;
  resource_id?: ID | null;
  request_id: string;
  created_at: string;
};

export type CacheEvent = {
  id: ID;
  request_log_id?: ID | null;
  event_type:
    | "hit"
    | "miss"
    | "store"
    | "skip"
    | "read_error"
    | "write_error"
    | "semantic_hit"
    | "semantic_miss"
    | "semantic_skip"
    | "semantic_store"
    | "semantic_read_error"
    | "semantic_write_error"
    | string;
  requested_model: string;
  cache_key_hash: string;
  messages_hash: string;
  reason?: string | null;
  created_at: string;
};

export type ContentPolicy = {
  id: ID;
  name: string;
  pii_action: "allow" | "redact" | "block" | string;
  status: string;
  created_at: string;
};

export type CreateContentPolicyRequest = {
  name: string;
  pii_action: "allow" | "redact" | "block";
};

export type RequestLog = {
  id: ID;
  api_key_id?: ID | null;
  provider_id?: ID | null;
  model_id?: ID | null;
  request_model?: string | null;
  status: string;
  status_code: number;
  error_code?: string | null;
  latency_ms: number;
  provider_latency_ms?: number | null;
  started_at: string;
  completed_at: string;
};

export type RequestLogsResponse = {
  items: RequestLog[];
  next_cursor: string | null;
};

export type UsageSummaryItem = {
  provider_id?: ID;
  provider_name?: string;
  model_id?: ID;
  model_name?: string;
  api_key_id?: ID;
  api_key_name?: string;
  request_count: number;
  success_count: number;
  error_count: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  avg_latency_ms: number;
};

export type UsageSummaryResponse = {
  group_by: "provider" | "model" | "api_key";
  items: UsageSummaryItem[];
};

export type AnalyticsDailyCostPoint = {
  day: string;
  request_count: number;
  total_cost_micro_usd: number;
};

export type AnalyticsModelCostItem = {
  model_id: ID;
  request_count: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
};

export type AnalyticsProviderLatencyItem = {
  provider_id: ID;
  request_count: number;
  avg_latency_ms: number;
  p50_latency_ms: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
};

export type AnalyticsErrorRatePoint = {
  day: string;
  request_count: number;
  error_count: number;
  error_rate: number;
};

export type AnalyticsDailyCostResponse = {
  items: AnalyticsDailyCostPoint[];
};

export type AnalyticsModelCostResponse = {
  items: AnalyticsModelCostItem[];
};

export type AnalyticsProviderLatencyResponse = {
  items: AnalyticsProviderLatencyItem[];
};

export type AnalyticsErrorRateResponse = {
  items: AnalyticsErrorRatePoint[];
};
