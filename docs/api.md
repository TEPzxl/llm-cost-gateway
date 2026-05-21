# API 参考

本文档记录当前已实现的 HTTP API。所有业务接口都返回 JSON，错误格式统一为：

```json
{
  "error": {
    "code": "invalid_request",
    "message": "Human readable message",
    "request_id": "uuid"
  }
}
```

## Health 与 Metrics

### GET `/healthz`

无需鉴权。

```json
{"status":"ok"}
```

### GET `/metrics`

非生产环境无需鉴权，返回 Prometheus text format。本地地址为 `http://localhost:8080/metrics`。生产环境默认不注册该路由，应通过受保护的网络或反向代理策略暴露采集入口。

## Platform API

鉴权：

```http
Authorization: Bearer <PLATFORM_BOOTSTRAP_TOKEN>
```

本地 Docker Compose 默认 token 为 `dev-bootstrap-token`。

### POST `/api/v1/platform/orgs`

创建 Organization。

```json
{
  "name": "Demo Org",
  "slug": "demo"
}
```

响应包含 `id`、`name`、`slug`、`status`、`created_at`。

### POST `/api/v1/platform/orgs/{org_id}/admin-tokens`

创建租户 Admin Token。明文 token 只在创建时返回一次。

```json
{
  "name": "initial-admin-token",
  "scopes": ["admin:*"],
  "expires_at": null
}
```

## Tenant Admin API

鉴权：

```http
Authorization: Bearer <DEMO_ADMIN_TOKEN>
```

也支持 passwordless session 返回的 session token。`org_id` 从 token/session 解析，调用方不能通过请求参数指定租户。

### POST `/api/v1/admin/sessions/passwordless-mock`

开发和演示用免密登录接口，不需要 Bearer Token。

```json
{
  "org_slug": "demo",
  "email": "owner@example.com"
}
```

响应包含短期 session token、用户、组织和角色信息。

### POST `/api/v1/admin/sessions/passwordless/request`

真实邮件免密登录入口，不需要 Bearer Token。仅在 `AUTH_PASSWORDLESS_EMAIL_ENABLED=true` 且 SMTP sender 已配置时注册。

```json
{
  "org_slug": "demo",
  "email": "owner@example.com"
}
```

响应固定为 accepted，避免邮箱枚举：

```json
{"status":"accepted"}
```

### POST `/api/v1/admin/sessions/passwordless/verify`

消费邮件链接中的一次性 token，返回短期 session token。magic link token 只保存 hash，成功消费后不能再次使用。

```json
{
  "token": "llmgw_magic_..."
}
```

响应结构与 passwordless mock login 相同，包含 session token、用户、组织和角色信息。

### GET `/api/v1/admin/me`

返回当前组织、当前 actor 和角色信息。

### Members

- `GET /api/v1/admin/members`
- `POST /api/v1/admin/members`

角色支持 `owner`、`admin`、`viewer`。Admin Token 作为 service token 兼容旧接口。

### API Key

`POST /api/v1/admin/api-keys`

```json
{
  "name": "docs-assistant-prod",
  "scopes": ["chat.completions"],
  "rpm_limit": 60,
  "daily_cost_limit_micro_usd": null,
  "monthly_cost_limit_micro_usd": null,
  "quota_action": "block",
  "expires_at": null
}
```

响应中的 `key` 明文只返回一次。列表接口 `GET /api/v1/admin/api-keys` 不返回明文 key。Daily/monthly cost quota 在 Gateway 请求前检查，`quota_action=block` 会返回 `api_key_quota_exceeded`。

`POST /api/v1/admin/api-keys/{api_key_id}/revoke` 撤销 key。

### Providers

`POST /api/v1/admin/providers`

Mock Provider：

```json
{
  "name": "mock-provider",
  "type": "mock",
  "base_url": null,
  "api_key": null,
  "timeout_ms": 30000
}
```

OpenAI-compatible Provider：

```json
{
  "name": "compatible-provider",
  "type": "openai_compatible",
  "base_url": "https://api.example.com/v1",
  "api_key": "provider-secret",
  "timeout_ms": 30000
}
```

响应不会返回 `api_key`。Provider API Key 加密存储。

相关接口：

- `GET /api/v1/admin/providers`
- `GET /api/v1/admin/providers/health`
- `POST /api/v1/admin/providers/{provider_id}/health-check`

### Models

`POST /api/v1/admin/models`

```json
{
  "provider_id": "uuid",
  "provider_model_name": "mock-small",
  "display_name": "Mock Small",
  "input_price_micro_usd_per_1k_tokens": 100,
  "output_price_micro_usd_per_1k_tokens": 200,
  "context_window": 8192
}
```

金额字段使用 micro USD 整数，不使用浮点金额。列表接口为 `GET /api/v1/admin/models`。

价格版本接口：

- `PATCH /api/v1/admin/models/{model_id}/pricing`
- `GET /api/v1/admin/models/{model_id}/pricing-versions`

### Route Policies

`POST /api/v1/admin/route-policies`

```json
{
  "name": "fast-chat-policy",
  "match_model": "fast-chat",
  "strategy": "single",
  "targets": [
    {
      "provider_id": "uuid",
      "model_id": "uuid",
      "priority": 1,
      "weight": 100
    }
  ]
}
```

支持 `single`、`fallback`、`lowest_cost`、`lowest_latency` strategy。列表接口为 `GET /api/v1/admin/route-policies`。

`lowest_cost` 可使用 `config.max_estimated_cost_micro_usd` 和 `config.fallback_to_priority`。`lowest_latency` 可使用 `config.latency_window_minutes`。

### Budgets

`POST /api/v1/admin/budgets`

```json
{
  "name": "monthly-budget",
  "scope_type": "org",
  "scope_id": null,
  "period": "monthly",
  "limit_micro_usd": 100000000,
  "action": "block"
}
```

列表接口为 `GET /api/v1/admin/budgets`。预算状态接口为 `GET /api/v1/admin/budgets/status`。

### Budget Alerts

- `POST /api/v1/admin/budget-alerts`
- `GET /api/v1/admin/budget-alerts`
- `GET /api/v1/admin/budget-alert-deliveries`

预算告警支持 webhook delivery。Webhook secret 不会在响应中返回。

### Content Policies

- `POST /api/v1/admin/content-policies`
- `GET /api/v1/admin/content-policies`

PII 策略支持 `allow`、`redact`、`block`。

### Anomaly Policies

- `POST /api/v1/admin/anomaly-policies`
- `GET /api/v1/admin/anomaly-policies`

规则支持每日成本阈值、API Key 成本突增、模型成本突增和错误率突增。动作支持 `notify`、`downgrade`、`block`。

### Audit Logs

`GET /api/v1/admin/audit-logs`

返回 Admin API 变更操作审计。

### Cache Events

`GET /api/v1/admin/cache-events`

支持 exact cache 与 semantic cache 相关事件查询。

### Request Logs

`GET /api/v1/admin/request-logs`

支持 `from`、`to`、`status`、`error_code`、`request_model`、`api_key_id`、`provider_id`、`model_id`、`limit`、`cursor` 查询参数。分页使用 cursor，基于 `started_at` 和 `id`。

RequestLog 只保存元数据、hash、状态、延迟和资源 ID，不保存 Prompt 或 Response 原文。

### Usage Summary

`GET /api/v1/admin/usage/summary`

支持 `group_by=provider|model|api_key`，以及 `from`、`to` 时间窗口。

返回 `request_count`、`success_count`、`error_count`、token 聚合、`total_cost_micro_usd` 和平均延迟。

### Analytics

- `GET /api/v1/admin/analytics/daily-cost`
- `GET /api/v1/admin/analytics/model-cost-breakdown`
- `GET /api/v1/admin/analytics/provider-latency`
- `GET /api/v1/admin/analytics/error-rate`

ClickHouse analytics 可选启用；未启用时使用 PostgreSQL 查询。

## Gateway API

鉴权：

```http
Authorization: Bearer <DEMO_API_KEY>
```

### POST `/v1/chat/completions`

```json
{
  "model": "fast-chat",
  "messages": [
    {"role": "user", "content": "hello"}
  ],
  "temperature": 0.2,
  "max_tokens": 512,
  "stream": false
}
```

非流式响应包含 OpenAI-compatible 风格的 `choices`、`usage`，以及网关追加的 `provider`、`cost`、`request_id`。

当前行为：

- `stream=true` 返回 OpenAI-compatible SSE。
- route 不存在返回 `route_not_found`。
- API Key 超 RPM 返回 `rate_limit_exceeded`。
- block budget 超限返回 `budget_exceeded`。
- API Key quota 超限返回 `api_key_quota_exceeded`。
- 内容策略阻断返回 `content_policy_blocked`。
- 异常成本阻断返回 `cost_anomaly_blocked`。
- fallback/retry 只对可重试上游错误生效，不对认证错误重试。
- cache hit 不调用上游 Provider，仍记录 request log 和 cache event。
- 成本以 micro USD 整数计算和返回。
