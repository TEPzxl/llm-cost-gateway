# 运维与本地开发指南

本文档记录本地开发、演示和验收所需的常用操作。

## 环境要求

- Go 1.22+
- Node.js 22+
- npm
- Docker 与 Docker Compose
- `curl`
- `jq`

## 本地服务

启动本地依赖和服务：

```bash
make up
```

服务地址：

| 服务 | 地址 |
|---|---|
| Gateway | `http://localhost:8080` |
| Console | `http://localhost:3000` |
| Prometheus | `http://localhost:9090` |
| PostgreSQL | `localhost:5432` |
| Redis | `localhost:6379` |

停止服务：

```bash
make down
```

## 数据库迁移

应用迁移：

```bash
make migrate-up
```

回滚迁移：

```bash
make migrate-down
```

验证数据库租户约束和值约束：

```bash
make verify-migration
```

## 演示数据

创建一套可演示的租户、Admin Token、Gateway API Key、Mock Provider、Model、RoutePolicy 和 Budget：

```bash
make seed-demo
```

脚本会写入本地 `.demo.env`，其中包含一次性返回的明文 Admin Token 和 Gateway API Key。该文件已被 `.gitignore` 忽略，不应提交。

可覆盖的环境变量：

| Variable | Default |
|---|---|
| `GATEWAY_URL` | `http://localhost:8080` |
| `PLATFORM_BOOTSTRAP_TOKEN` | `dev-bootstrap-token` |
| `DEMO_ENV_FILE` | `.demo.env` |
| `DEMO_SLUG` | `llmgw-demo-<timestamp>` |
| `DEMO_MATCH_MODEL` | `fast-chat` |

## Smoke Test

运行端到端 smoke test：

```bash
make smoke-test
```

它会读取 `.demo.env`，完成：

1. 使用 Gateway API Key 调用 `/v1/chat/completions`。
2. 验证响应中的 `usage.total_tokens >= 0`。
3. 验证响应中的 `cost.total_cost_micro >= 0`。
4. 使用 Admin Token 查询 request logs。
5. 使用 Admin Token 查询 usage summary。

## 测试

后端测试：

```bash
make test
```

仓库包含前端依赖目录，日常后端测试建议使用 `make test`。如需运行全量 Go 测试：

```bash
go test ./...
```

管理后台 API client 测试：

```bash
cd web/console && npm test
```

管理后台生产构建：

```bash
make console-build
```

## Provider Secret 密钥轮换

Provider API Key 使用 AES-GCM 加密，并通过 `provider_secrets.key_version` 记录密钥版本。Admin Token、Gateway API Key、user session 和 magic link token 支持 `TOKEN_HASH_SECRET_KEYRING` 双读单写。轮换前先配置 `SECRET_ENCRYPTION_KEY_VERSION`、`SECRET_ENCRYPTION_KEYRING`、`TOKEN_HASH_SECRET_VERSION` 和 `TOKEN_HASH_SECRET_KEYRING`，再执行 dry-run：

```bash
go run ./cmd/secret-rotate -config configs/config.example.yaml -dry-run
```

确认结果后去掉 `-dry-run` 执行实际 re-encryption。详细流程见 `docs/runbooks/secret-rotation.md`。

## Load Test

本地 baseline 压测：

```bash
LOAD_TEST_SCENARIO=baseline LOAD_TEST_REQUESTS=20 LOAD_TEST_VUS=2 make load-test
```

报告会写入 `.loadtest/`。详细流程见 `docs/runbooks/load-testing.md`。

## Console

Docker Compose 启动后，管理后台地址为：

```text
http://localhost:3000
```

登录时输入 `.demo.env` 中的 `DEMO_ADMIN_TOKEN`。前端使用 `sessionStorage` 保存当前会话 token，不写入 `localStorage`。

生产环境可启用真实邮件免密登录：

```bash
AUTH_PASSWORDLESS_EMAIL_ENABLED=true
AUTH_MAGIC_LINK_BASE_URL=https://console.example.com
AUTH_MAGIC_LINK_TTL_SECONDS=900
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USERNAME=llmgw
SMTP_PASSWORD=...
SMTP_FROM=login@example.com
SMTP_TLS_MODE=starttls
NEXT_PUBLIC_ENABLE_PASSWORDLESS_EMAIL=true
```

`APP_ENV=production` 时 magic link base URL 必须使用 HTTPS。开发和测试环境仍可使用 passwordless mock；生产环境不会注册 mock 登录接口。

## Metrics

Prometheus UI：

```text
http://localhost:9090
```

Gateway metrics endpoint：

```bash
curl http://localhost:8080/metrics
```

生产环境默认不注册 `/metrics` 路由；如需生产采集，应通过受保护网络或反向代理策略显式暴露。

Metrics label 不包含 request_id，也不包含 Prompt 或 Response 原文。
