# API Reference

本文档记录当前 v0.1 MVP 已实现的 HTTP API。所有业务接口都返回 JSON，错误格式统一为：

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

无需鉴权，返回 Prometheus text format。本地地址为 `http://localhost:8080/metrics`。

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

`org_id` 从 Admin Token 解析，调用方不能通过请求参数指定租户。

### GET `/api/v1/admin/me`

返回当前组织和 Admin Token 元信息。

### API Key

`POST /api/v1/admin/api-keys`

```json
{
  "name": "docs-assistant-prod",
  "scopes": ["chat.completions"],
  "rpm_limit": 60,
  "expires_at": null
}
```

响应中的 `key` 明文只返回一次。列表接口 `GET /api/v1/admin/api-keys` 不返回明文 key。

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

响应不会返回 `api_key`。列表接口为 `GET /api/v1/admin/providers`。

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

v0.1 仅支持 `single` strategy。列表接口为 `GET /api/v1/admin/route-policies`。

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

### Request Logs

`GET /api/v1/admin/request-logs`

支持 `from`、`to`、`status`、`api_key_id`、`provider_id`、`model_id`、`limit`、`offset` 查询参数。

RequestLog 只保存元数据、hash、状态、延迟和资源 ID，不保存 Prompt 或 Response 原文。

### Usage Summary

`GET /api/v1/admin/usage/summary`

支持 `group_by=provider|model|api_key`，以及 `from`、`to` 时间窗口。

返回 `request_count`、`success_count`、`error_count`、token 聚合、`total_cost_micro_usd` 和平均延迟。

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

响应包含 OpenAI-compatible 风格的 `choices`、`usage`，以及网关追加的 `provider`、`cost`、`request_id`。

v0.1 行为：

- `stream=true` 返回 `stream_not_supported`。
- route 不存在返回 `route_not_found`。
- API Key 超 RPM 返回 `rate_limit_exceeded`。
- block budget 超限返回 `budget_exceeded`。
- 成本以 micro USD 整数计算和返回。
