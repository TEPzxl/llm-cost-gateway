# Phase 1 Demo

本流程用于从本地空环境跑通阶段 1 MVP 的核心链路。

## 1. Start Services

```bash
make up
```

确认 Gateway 健康：

```bash
curl http://localhost:8080/healthz
```

期望响应：

```json
{"status":"ok"}
```

## 2. Run Migrations

```bash
make migrate-up
```

如果迁移已经应用过，命令会保持数据库在最新版本。

## 3. Seed Demo Data

```bash
make seed-demo
```

该命令会创建：

- Organization
- Admin Token
- Gateway API Key
- Mock Provider
- Mock Model
- `single` RoutePolicy
- Monthly block Budget

生成的本地凭据写入 `.demo.env`。

## 4. Run Smoke Test

```bash
make smoke-test
```

成功时会输出类似：

```text
Smoke test passed.
request_id=<uuid> total_tokens=50 total_cost_micro=8
```

这证明：

- 业务方可以通过 Gateway API Key 调用 `/v1/chat/completions`。
- 请求被路由到 Mock Provider。
- Gateway 返回 usage 和 cost。
- RequestLog、UsageRecord、CostRecord 已落库。
- Admin API 能查询 request logs 和 usage summary。
- 成本以 micro USD 整数形式返回。

## 5. Open Console

打开：

```text
http://localhost:3000
```

使用 `.demo.env` 中的 `DEMO_ADMIN_TOKEN` 登录。可以查看和创建：

- API Keys
- Providers
- Models
- Route Policies
- Budgets
- Request Logs
- Usage Summary

## 6. Open Metrics

打开 Prometheus：

```text
http://localhost:9090
```

或直接查看 Gateway metrics：

```bash
curl http://localhost:8080/metrics
```

## 7. Cleanup

停止服务：

```bash
make down
```

如需清空数据库卷，可执行：

```bash
docker compose -f deploy/docker-compose.yml --project-directory . down -v
```

注意：`down -v` 会删除本地 PostgreSQL 和 Redis 数据。
