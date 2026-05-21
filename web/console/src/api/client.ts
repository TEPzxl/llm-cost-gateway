import type {
  APIKey,
  APIKeyCreateResponse,
  AdminAuditLog,
  AnalyticsDailyCostResponse,
  AnalyticsErrorRateResponse,
  AnalyticsModelCostResponse,
  AnalyticsProviderLatencyResponse,
  Budget,
  BudgetAlert,
  BudgetAlertDelivery,
  BudgetStatus,
  CacheEvent,
  CreateAPIKeyRequest,
  CreateBudgetAlertRequest,
  CreateBudgetRequest,
  CreateModelRequest,
  CreateProviderRequest,
  CreateRoutePolicyRequest,
  ListResponse,
  Model,
  Provider,
  RequestLogsResponse,
  RoutePolicy,
  UsageSummaryResponse
} from "./types";

export type FetchLike = (
  input: string,
  init?: RequestInit
) => Promise<Response>;

export type ApiClientOptions = {
  baseUrl?: string;
  getToken: () => string | null;
  fetchImpl?: FetchLike;
};

export type ApiClient = ReturnType<typeof createApiClient>;

export function createApiClient(options: ApiClientOptions) {
  const baseUrl = (
    options.baseUrl ??
    process.env.NEXT_PUBLIC_API_BASE_URL ??
    ""
  ).replace(/\/$/, "");
  const fetchImpl = options.fetchImpl ?? fetch;

  async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const token = options.getToken();
    const headers: Record<string, string> = {
      Accept: "application/json",
      ...(init.body ? { "Content-Type": "application/json" } : {}),
      ...((init.headers as Record<string, string> | undefined) ?? {})
    };
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }

    const response = await fetchImpl(`${baseUrl}${path}`, { ...init, headers });
    const text = await response.text();
    const data = text ? JSON.parse(text) : null;
    if (!response.ok) {
      const message =
        data?.error?.message ?? `request failed with status ${response.status}`;
      throw new Error(message);
    }
    return data as T;
  }

  return {
    listAPIKeys: () => request<ListResponse<APIKey>>("/api/v1/admin/api-keys"),
    createAPIKey: (body: CreateAPIKeyRequest) =>
      request<APIKeyCreateResponse>("/api/v1/admin/api-keys", {
        method: "POST",
        body: JSON.stringify(body)
      }),
    revokeAPIKey: (id: string) =>
      request<APIKey>(`/api/v1/admin/api-keys/${id}/revoke`, {
        method: "POST"
      }),
    listProviders: () =>
      request<ListResponse<Provider>>("/api/v1/admin/providers"),
    createProvider: (body: CreateProviderRequest) =>
      request<Provider>("/api/v1/admin/providers", {
        method: "POST",
        body: JSON.stringify(body)
      }),
    listProviderHealth: () =>
      request<ListResponse<Provider>>("/api/v1/admin/providers/health"),
    checkProviderHealth: (id: string) =>
      request<Provider>(`/api/v1/admin/providers/${id}/health-check`, {
        method: "POST"
      }),
    listModels: () => request<ListResponse<Model>>("/api/v1/admin/models"),
    createModel: (body: CreateModelRequest) =>
      request<Model>("/api/v1/admin/models", {
        method: "POST",
        body: JSON.stringify(body)
      }),
    listRoutePolicies: () =>
      request<ListResponse<RoutePolicy>>("/api/v1/admin/route-policies"),
    createRoutePolicy: (body: CreateRoutePolicyRequest) =>
      request<RoutePolicy>("/api/v1/admin/route-policies", {
        method: "POST",
        body: JSON.stringify(body)
      }),
    listBudgets: () => request<ListResponse<Budget>>("/api/v1/admin/budgets"),
    createBudget: (body: CreateBudgetRequest) =>
      request<Budget>("/api/v1/admin/budgets", {
        method: "POST",
        body: JSON.stringify(body)
      }),
    budgetStatus: () =>
      request<ListResponse<BudgetStatus>>("/api/v1/admin/budgets/status"),
    listBudgetAlerts: () =>
      request<ListResponse<BudgetAlert>>("/api/v1/admin/budget-alerts"),
    createBudgetAlert: (body: CreateBudgetAlertRequest) =>
      request<BudgetAlert>("/api/v1/admin/budget-alerts", {
        method: "POST",
        body: JSON.stringify(body)
      }),
    listBudgetAlertDeliveries: () =>
      request<ListResponse<BudgetAlertDelivery>>(
        "/api/v1/admin/budget-alert-deliveries"
      ),
    listAuditLogs: () =>
      request<ListResponse<AdminAuditLog>>("/api/v1/admin/audit-logs"),
    listCacheEvents: (query = "") =>
      request<ListResponse<CacheEvent>>(`/api/v1/admin/cache-events${query}`),
    listRequestLogs: (query = "") =>
      request<RequestLogsResponse>(`/api/v1/admin/request-logs${query}`),
    usageSummary: (query = "") =>
      request<UsageSummaryResponse>(`/api/v1/admin/usage/summary${query}`),
    analyticsDailyCost: (query = "") =>
      request<AnalyticsDailyCostResponse>(
        `/api/v1/admin/analytics/daily-cost${query}`
      ),
    analyticsModelCostBreakdown: (query = "") =>
      request<AnalyticsModelCostResponse>(
        `/api/v1/admin/analytics/model-cost-breakdown${query}`
      ),
    analyticsProviderLatency: (query = "") =>
      request<AnalyticsProviderLatencyResponse>(
        `/api/v1/admin/analytics/provider-latency${query}`
      ),
    analyticsErrorRate: (query = "") =>
      request<AnalyticsErrorRateResponse>(
        `/api/v1/admin/analytics/error-rate${query}`
      )
  };
}
