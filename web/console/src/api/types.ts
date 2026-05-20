export type ID = string;

export type ListResponse<T> = {
  items: T[];
};

export type APIKey = {
  id: ID;
  name: string;
  key_prefix: string;
  status: string;
  rpm_limit: number;
  expires_at?: string | null;
  last_used_at?: string | null;
  created_at: string;
};

export type CreateAPIKeyRequest = {
  name: string;
  scopes: string[];
  rpm_limit: number;
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
  strategy: "single" | "fallback" | string;
  status: string;
  created_at: string;
};

export type CreateRoutePolicyRequest = {
  name: string;
  match_model: string;
  strategy: "single" | "fallback";
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
