# 生产上线与试运行 Runbook

本文档用于把 v0.4 生产上线准备项转化为可签核的 go/no-go 门槛。所有自动化检查必须保留命令输出或报告文件；所有人工项必须填写负责人、证据和签核结果。

## 职责边界

| 角色 | 职责 | 签核人 |
|---|---|---|
| Release Owner | 组织上线窗口、汇总证据、作出 go/no-go 建议 |  |
| Backend Owner | Gateway、Admin API、迁移、密钥轮换和 Provider 联调 |  |
| Frontend Owner | Console 构建、登录流程和生产配置 |  |
| SRE Owner | 部署、监控告警、压测、备份恢复和回滚演练 |  |
| Security Owner | 密钥强度、访问边界、日志脱敏和权限审查 |  |

## 上线前配置

必须确认：

| 项目 | 要求 | 负责人 | 证据 | 签核 |
|---|---|---|---|---|
| `APP_ENV` | 生产为 `production` |  |  |  |
| `DATABASE_URL` | 指向生产 PostgreSQL，TLS/网络策略符合要求 |  |  |  |
| `REDIS_URL` | 指向生产 Redis，持久化策略已确认 |  |  |  |
| `TOKEN_HASH_SECRET` | 非默认强密钥 |  |  |  |
| `TOKEN_HASH_SECRET_KEYRING` | 包含 active version，旧版本在 grace period 内保留 |  |  |  |
| `SECRET_ENCRYPTION_KEY` | 32 字节强密钥 |  |  |  |
| `SECRET_ENCRYPTION_KEYRING` | 包含 active version，可解密旧 Provider secret 与 webhook secret |  |  |  |
| `AUTH_PASSWORDLESS_EMAIL_ENABLED` | 生产启用真实邮件登录 |  |  |  |
| `AUTH_MAGIC_LINK_BASE_URL` | HTTPS Console URL |  |  |  |
| SMTP 配置 | host、port、username、password、from、TLS mode 完整 |  |  |  |
| `/metrics` 暴露 | 仅受保护网络或反向代理可访问 |  |  |  |
| Provider API Key | 使用测试/生产分离密钥，权限和额度受控 |  |  |  |

## 自动化验收

在候选版本上执行：

```bash
make production-checks
```

该目标覆盖：

- 后端测试：`make test`
- Console API client 测试：`cd web/console && npm test`
- Console 生产构建：`make console-build`
- Prometheus 规则校验：`make verify-alert-rules`

需要运行服务栈的验收单独执行：

```bash
make up
make migrate-up
make verify-migration
make seed-demo
make smoke-test
```

压测与容量基线：

```bash
LOAD_TEST_SCENARIO=baseline LOAD_TEST_REQUESTS=100 LOAD_TEST_VUS=5 make load-test
```

备份恢复演练：

```bash
make backup-demo
make restore-drill
```

密钥轮换 dry-run：

```bash
make secret-rotate
```

真实 Provider 联调在隔离测试账号中执行：

```bash
cp configs/live-provider.example.env .live-provider.env
vim .live-provider.env
make live-provider-test
make live-gateway-test
```

## Go/No-Go Checklist

| 检查项 | 自动化命令或人工证据 | 负责人 | 证据 | 结果 |
|---|---|---|---|---|
| 配置校验通过 | 应用启动日志无配置错误 |  |  |  |
| 数据库迁移状态正确 | `make migrate-up`、`make verify-migration` |  |  |  |
| 生产 mock 登录不可用 | 生产环境无 mock session route，Console 不展示 mock 登录 |  |  |  |
| 真实 magic link 登录可用 | 使用测试成员完成邮件登录 |  |  |  |
| Admin Token 登录保留应急入口 | 受控 Admin Token 可登录，存储位置安全 |  |  |  |
| Provider secret 可解密 | Provider health check 成功 |  |  |  |
| 密钥轮换 dry-run 通过 | `make secret-rotate` |  |  |  |
| 后端测试通过 | `make test` |  |  |  |
| Console 测试和构建通过 | `cd web/console && npm test`、`make console-build` |  |  |  |
| Smoke test 通过 | `make smoke-test` |  |  |  |
| 压测完成 | `.loadtest/report-*.txt` |  |  |  |
| 告警规则加载 | `make verify-alert-rules`，Prometheus UI 可见规则 |  |  |  |
| 备份可生成 | `make backup-demo` 或生产备份任务结果 |  |  |  |
| 恢复演练通过 | `make restore-drill` 或隔离恢复报告 |  |  |  |
| 真实 Provider 联调通过 | `make live-provider-test`、`make live-gateway-test` 输出 |  |  |  |
| Prompt/Response 不落库 | 抽查 request_logs、usage_records、cost_records |  |  |  |
| 金额精度 | 成本字段保持 integer micro USD |  |  |  |
| 回滚方案已演练 | `docs/runbooks/rollback.md` 记录 |  |  |  |

只有所有 P0/P1 项通过，且 P2 项有明确风险接受人时，才允许进入试运行。

## 试运行策略

试运行范围：

- 初始只开放给内部租户或低风险租户。
- 每个租户使用独立 Gateway API Key 和 Provider quota。
- 单租户先限制 RPM、每日成本和每月成本。

观察窗口：

- 第一阶段：2 小时，只允许低并发请求。
- 第二阶段：24 小时，允许受控业务流量。
- 第三阶段：7 天，观察成本、错误率、延迟和告警噪音。

必须观察：

- Gateway 5xx rate。
- p95 latency。
- Provider timeout、429、5xx。
- budget block 和 rate limit spike。
- usage event publish failures。
- analytics write failures。
- PostgreSQL、Redis、ClickHouse 资源水位。
- 实际 Provider 账单与 `cost_records.total_cost_micro` 的趋势差异。

异常退出条件：

- Gateway 5xx 持续超过告警阈值。
- Provider 错误或超时持续放大，且 fallback 无法缓解。
- usage 或 cost 记录缺失。
- 真实成本明显偏离预算或账单趋势。
- magic link 登录不可用且 Admin Token 应急登录失败。
- 备份任务失败或恢复演练证据缺失。

触发异常退出时，停止扩大流量并执行 `docs/runbooks/rollback.md`。

## 上线步骤

1. 冻结候选版本并记录 commit SHA。
2. 执行自动化验收和需要服务栈的验收命令。
3. 汇总压测、告警、备份恢复、密钥轮换和 Provider 联调证据。
4. Release Owner 组织 go/no-go 审查。
5. 部署候选版本到生产。
6. 执行生产迁移。
7. 验证 `/healthz`、Console 登录、Admin API、Provider health。
8. 对内部租户开启试运行。
9. 按观察窗口逐步扩大流量。

## 证据记录

| 证据 | 路径或链接 | 负责人 | 时间 |
|---|---|---|---|
| 候选 commit SHA |  |  |  |
| `make production-checks` 输出 |  |  |  |
| smoke test 输出 |  |  |  |
| 压测报告 |  |  |  |
| 告警规则校验 |  |  |  |
| 备份文件与恢复演练报告 |  |  |  |
| 真实 Provider 联调输出 |  |  |  |
| 密钥轮换 dry-run 输出 |  |  |  |
| 试运行观察记录 |  |  |  |

## 最终签核

| 角色 | Go/No-Go | 备注 | 签名 | 时间 |
|---|---|---|---|---|
| Release Owner |  |  |  |  |
| Backend Owner |  |  |  |  |
| Frontend Owner |  |  |  |  |
| SRE Owner |  |  |  |  |
| Security Owner |  |  |  |  |
