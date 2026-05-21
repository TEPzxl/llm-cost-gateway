# LLM Cost Gateway

SaaS 多租户 LLM 网关与 CostOps 平台。

当前实现状态：阶段 1、阶段 2、阶段 3 任务已完成，项目具备 v0.3 CostOps 平台化能力。生产上线准备事项已整理为阶段 4 SDD，见 `docs/10_PRODUCTION_READINESS_SDD.md`。

## 环境要求

- Go 1.25+（生产镜像使用 Go 1.26.3 或更新补丁版本）
- Node.js 22+
- npm
- Docker 与 Docker Compose
- `curl`
- `jq`

## 快速演示

启动本地服务、执行迁移、写入演示数据并运行端到端 smoke test：

```bash
make up
make migrate-up
make seed-demo
make smoke-test
```

`make seed-demo` 会把本地演示凭据写入 `.demo.env`。管理后台登录使用其中的 `DEMO_ADMIN_TOKEN`。

## 本地服务

| 服务 | 地址 |
|---|---|
| Gateway | `http://localhost:8080` |
| Console | `http://localhost:3000` |
| Prometheus | `http://localhost:9090` |
| PostgreSQL | `localhost:5432` |
| Redis | `localhost:6379` |

健康检查：

```bash
curl http://localhost:8080/healthz
```

期望响应：

```json
{"status":"ok"}
```

## 常用命令

启动本地栈：

```bash
make up
```

停止本地栈：

```bash
make down
```

运行数据库迁移：

```bash
make migrate-up
```

回滚本地迁移：

```bash
make migrate-down
```

验证迁移中的租户隔离与数值约束：

```bash
make verify-migration
```

写入演示资源：

```bash
make seed-demo
```

运行 smoke test：

```bash
make smoke-test
```

运行后端测试：

```bash
make test
```

构建管理后台：

```bash
make console-build
```

运行前端 API client 测试：

```bash
cd web/console && npm test
```

## 管理后台

管理后台地址：

```text
http://localhost:3000
```

使用 `.demo.env` 中的 `DEMO_ADMIN_TOKEN` 登录。管理后台把 token 存在当前浏览器会话的 `sessionStorage` 中，不写入 `localStorage`。生产环境可通过 `AUTH_PASSWORDLESS_EMAIL_ENABLED=true` 和 SMTP 配置启用真实邮件免密登录。

已实现页面：

- 总览
- 成员
- API 密钥
- 供应商
- 模型
- 路由策略
- 异常策略
- 内容策略
- 预算
- 预算告警
- 审计日志
- 缓存事件
- 请求日志
- 使用汇总

## Gateway API

使用 `.demo.env` 中的 `DEMO_API_KEY`：

```bash
curl -sS http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $DEMO_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "fast-chat",
    "messages": [{"role": "user", "content": "hello"}],
    "stream": false
  }'
```

响应包含 usage 与 cost。成本使用 integer micro USD，不使用浮点金额。

## 当前能力

- 多租户 Organization、Admin Token 与 user session。
- owner/admin/viewer RBAC。
- API Key 创建、撤销、RPM 限流与 daily/monthly cost quota。
- Mock Provider 与 OpenAI-compatible Provider。
- Provider health check、timeout、retry。
- single、fallback、lowest_cost、lowest_latency 路由策略。
- 非流式与流式 `/v1/chat/completions`。
- 请求日志、用量记录、成本记录与定价版本快照。
- org 级预算、warn/block 策略与 webhook 告警。
- Prompt exact cache 与可选 semantic cache。
- 内容策略：PII allow/redact/block。
- 异常成本检测：notify/downgrade/block。
- Admin 操作审计、缓存事件、成本/延迟/错误率分析。
- 可选 Kafka usage event、ClickHouse analytics、OpenTelemetry tracing。
- Prometheus metrics。

安全边界：

- 不保存 Prompt / Response 原文。
- Admin Token、Gateway API Key、Provider API Key 不明文落库。
- Provider API Key 使用 AES-GCM 加密。
- 租户资源查询必须带 `org_id`。
- 测试不依赖真实外部 LLM Provider。

## Metrics

Prometheus UI：

```text
http://localhost:9090
```

Gateway metrics endpoint：

```bash
curl http://localhost:8080/metrics
```

生产环境默认不注册 `/metrics` 路由；如需生产采集，应在受保护网络或反向代理策略下显式暴露。

## 文档

- `docs/api.md`：当前已实现 HTTP API 参考。
- `docs/operations.md`：本地运行与运维操作。
- `docs/demo.md`：本地演示流程。
- `docs/00_MASTER_SDD.md`：总体软件设计文档。
- `docs/01_PHASE_1_V0_1_SAAS_MVP.md`：阶段 1 任务计划。
- `docs/02_PHASE_2_V0_2_PRODUCTION_READY.md`：阶段 2 任务计划。
- `docs/03_PHASE_3_V0_3_COSTOPS_PLATFORM.md`：阶段 3 任务计划。
- `docs/10_PRODUCTION_READINESS_SDD.md`：阶段 4 生产上线准备任务计划。

## 配置

示例配置位于 `configs/config.example.yaml`。

环境变量会覆盖配置文件：

- `APP_ENV`
- `SERVER_PORT`
- `DATABASE_URL`
- `REDIS_URL`
- `PLATFORM_BOOTSTRAP_TOKEN`
- `TOKEN_HASH_SECRET`
- `TOKEN_HASH_SECRET_VERSION`
- `TOKEN_HASH_SECRET_KEYRING`
- `SECRET_ENCRYPTION_KEY`
- `SECRET_ENCRYPTION_KEY_VERSION`
- `SECRET_ENCRYPTION_KEYRING`
- `LOG_LEVEL`
- `MAX_RETRIES`
- `RETRY_BACKOFF_MS`
- `KAFKA_ENABLED`
- `KAFKA_BROKERS`
- `KAFKA_USAGE_TOPIC`
- `CLICKHOUSE_ENABLED`
- `CLICKHOUSE_URL`
- `CLICKHOUSE_DATABASE`
- `CLICKHOUSE_USERNAME`
- `CLICKHOUSE_PASSWORD`
- `PROMPT_CACHE_ENABLED`
- `PROMPT_CACHE_TTL_SECONDS`
- `SEMANTIC_CACHE_ENABLED`
- `SEMANTIC_CACHE_THRESHOLD`
- `SEMANTIC_CACHE_MAX_TEMPERATURE`
- `TRACING_ENABLED`
- `TRACING_OTLP_ENDPOINT`
- `TRACING_INSECURE`
- `TRACING_SERVICE_NAME`
- `AUTH_PASSWORDLESS_EMAIL_ENABLED`
- `AUTH_MAGIC_LINK_BASE_URL`
- `AUTH_MAGIC_LINK_TTL_SECONDS`
- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USERNAME`
- `SMTP_PASSWORD`
- `SMTP_FROM`
- `SMTP_TLS_MODE`

`SECRET_ENCRYPTION_KEY` 必须是 32 字节，用于 AES-256。
