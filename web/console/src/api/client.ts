import type {
  APIKey,
  APIKeyCreateResponse,
  AdminMe,
  AdminAuditLog,
  AnomalyPolicy,
  AnalyticsDailyCostResponse,
  AnalyticsErrorRateResponse,
  AnalyticsModelCostResponse,
  AnalyticsProviderLatencyResponse,
  Budget,
  BudgetAlert,
  BudgetAlertDelivery,
  BudgetStatus,
  CacheEvent,
  ContentPolicy,
  CreateAPIKeyRequest,
  CreateAnomalyPolicyRequest,
  CreateBudgetAlertRequest,
  CreateBudgetRequest,
  CreateContentPolicyRequest,
  CreateMemberRequest,
  CreateModelRequest,
  CreateProviderRequest,
  CreateRoutePolicyRequest,
  ListResponse,
  Member,
  Model,
  PasswordlessMagicLinkRequest,
  PasswordlessMagicLinkRequestResponse,
  PasswordlessMagicLinkVerifyRequest,
  PasswordlessMockLoginRequest,
  PasswordlessMockLoginResponse,
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

  async function request<T>(path: string, init: RequestInit = {}, authenticated = true): Promise<T> {
    const token = authenticated ? options.getToken() : null;
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
    let data: unknown = null;
    if (text) {
      try {
        data = JSON.parse(text);
      } catch {
        throw new Error(`API returned non-JSON response with status ${response.status}`);
      }
    }
    if (!response.ok) {
      const message =
        apiErrorMessage(data) ?? `request failed with status ${response.status}`;
      throw new Error(message);
    }
    return data as T;
  }

  return {
    getMe: () => request<AdminMe>("/api/v1/admin/me"),
    passwordlessMockLogin: (body: PasswordlessMockLoginRequest) =>
      request<PasswordlessMockLoginResponse>(
        "/api/v1/admin/sessions/passwordless-mock",
        {
          method: "POST",
          body: JSON.stringify(body)
        },
        false
      ),
    requestPasswordlessMagicLink: (body: PasswordlessMagicLinkRequest) =>
      request<PasswordlessMagicLinkRequestResponse>(
        "/api/v1/admin/sessions/passwordless/request",
        {
          method: "POST",
          body: JSON.stringify(body)
        },
        false
      ),
    verifyPasswordlessMagicLink: (body: PasswordlessMagicLinkVerifyRequest) =>
      request<PasswordlessMockLoginResponse>(
        "/api/v1/admin/sessions/passwordless/verify",
        {
          method: "POST",
          body: JSON.stringify(body)
        },
        false
      ),
    listMembers: () => request<ListResponse<Member>>("/api/v1/admin/members"),
    createMember: (body: CreateMemberRequest) =>
      request<Member>("/api/v1/admin/members", {
        method: "POST",
        body: JSON.stringify(body)
      }),
    listAnomalyPolicies: () =>
      request<ListResponse<AnomalyPolicy>>("/api/v1/admin/anomaly-policies"),
    createAnomalyPolicy: (body: CreateAnomalyPolicyRequest) =>
      request<AnomalyPolicy>("/api/v1/admin/anomaly-policies", {
        method: "POST",
        body: JSON.stringify(body)
      }),
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
    listContentPolicies: () =>
      request<ListResponse<ContentPolicy>>("/api/v1/admin/content-policies"),
    createContentPolicy: (body: CreateContentPolicyRequest) =>
      request<ContentPolicy>("/api/v1/admin/content-policies", {
        method: "POST",
        body: JSON.stringify(body)
      }),
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

function apiErrorMessage(data: unknown): string | null {
  if (!data || typeof data !== "object" || !("error" in data)) {
    return null;
  }
  const error = (data as { error?: unknown }).error;
  if (!error || typeof error !== "object" || !("message" in error)) {
    return null;
  }
  const message = (error as { message?: unknown }).message;
  return typeof message === "string" ? message : null;
}
