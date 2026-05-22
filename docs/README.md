# LLM Cost Gateway：Codex 实施文档包

本目录是一套可直接交给 Codex 分阶段实现的工程文档，目标是产出一个 SaaS 多租户形态的 **LLM 网关与 CostOps 平台**。

## 项目最终定位

项目名：**LLM Cost Gateway**

一句话说明：

> 一个面向 SaaS 团队的多租户 LLM CostOps Gateway，用于统一管理 OpenAI-compatible Provider 的调用、鉴权、路由、限流、用量统计、成本分析、预算控制与调用审计。

## 已确认产品假设

1. 部署形态：SaaS 多租户。
2. 测试 Provider：Mock Provider。
3. 真实 Provider：OpenAI-compatible Provider。
4. 包含管理后台。
5. 预算控制策略由管理员后台配置。
6. 不记录 Prompt / Response 原文，只记录 hash、token、成本、状态和审计元数据。
7. 成本币种：USD。
8. 管理后台使用管理员提供的租户级 Admin Token 鉴权。

## 文档文件说明

| 文件 | 作用 |
|---|---|
| `00_MASTER_SDD.md` | 总体 SDD，定义项目定位、架构、边界、核心模型与阶段目标 |
| `01_PHASE_1_V0_1_SAAS_MVP.md` | 阶段 1：v0.1 SaaS MVP，Codex 可执行任务清单 |
| `02_PHASE_2_V0_2_PRODUCTION_READY.md` | 阶段 2：v0.2 生产可用增强，Codex 可执行任务清单 |
| `03_PHASE_3_V0_3_COSTOPS_PLATFORM.md` | 阶段 3：v0.3 CostOps 平台化能力，Codex 可执行任务清单 |
| `04_DOMAIN_DATABASE_DESIGN.md` | 领域模型与数据库设计，包含 v0.1 表结构与迁移要求 |
| `05_API_DESIGN.md` | 初始 API 契约，包含 Platform API、Admin API、Gateway API |
| `06_BACKEND_ARCHITECTURE.md` | Go 后端架构、目录结构、模块职责、请求流转 |
| `07_MANAGEMENT_CONSOLE_DESIGN.md` | 管理后台设计，页面、状态管理、接口调用、前端目录结构 |
| `08_TESTING_AND_ACCEPTANCE.md` | 测试策略、集成测试、Provider mock、成本测试、验收脚本 |
| `09_CODEX_EXECUTION_GUIDE.md` | Codex 执行约束、阶段推进方式、全局提示词模板 |
| `10_PRODUCTION_READINESS_SDD.md` | 阶段 4：v0.4 生产上线准备，真实登录、密钥轮换、压测、告警、备份恢复、Provider 联调和上线检查 |

## 当前实现状态

阶段 1、阶段 2、阶段 3 的任务清单均已完成，当前仓库实现已覆盖 v0.3 CostOps 平台化能力。阶段 4 的生产上线准备能力已补齐为脚本、runbook 和检查清单；进入真实生产前仍需按 `runbooks/production-launch.md` 完成目标环境证据和签核。

已具备的核心能力：

- 多租户 Gateway、Admin API 与 Management Console。
- API Key、Provider、Model、RoutePolicy、Budget、RequestLog、Usage Summary。
- Streaming、fallback、retry、provider health、预算告警、API Key cost quota、定价版本。
- Admin 操作审计、cursor request logs、Dashboard 分析。
- Kafka usage event、ClickHouse analytics、成本/延迟感知路由。
- Prompt exact cache、semantic cache、PII policy、OpenTelemetry tracing。
- owner/admin/viewer RBAC 与 passwordless mock session。
- 异常成本检测与自动降级。
- 生产 magic link 邮件登录、密钥轮换、压测、告警、备份恢复、真实 Provider 联调和上线/回滚 runbook。

## 推荐交给 Codex 的执行顺序

1. 先阅读 `00_MASTER_SDD.md`。
2. 再阅读 `04_DOMAIN_DATABASE_DESIGN.md`、`05_API_DESIGN.md`、`06_BACKEND_ARCHITECTURE.md`。
3. 执行阶段 1：`01_PHASE_1_V0_1_SAAS_MVP.md`。
4. 完成阶段 1 验收后，再执行阶段 2。
5. 阶段 3 只在真实请求量、成本治理、缓存、路由优化需求出现后实施。

说明：以上是从零实现时的推荐顺序；当前仓库已经完成到阶段 3。

## 阶段 1 最终验收目标

阶段 1 完成后，项目必须具备以下能力：

- 通过 Docker Compose 启动 PostgreSQL、Redis、Gateway、Management Console。
- 使用平台 Bootstrap Token 创建 Organization 与租户级 Admin Token。
- 管理员通过 Admin Token 进入管理后台。
- 在后台创建 API Key、Provider、Model、RoutePolicy、Budget。
- 业务系统使用 Gateway API Key 调用 `/v1/chat/completions`。
- 测试环境调用 Mock Provider。
- 真实环境可配置 OpenAI-compatible Provider。
- 每次请求记录 RequestLog、UsageRecord、CostRecord。
- 能查询 token 用量、USD 成本、延迟、错误率。
- 能按后台配置执行预算策略：warn / block。
- 能按 API Key 执行 Redis RPM 限流。
- 不记录 Prompt / Response 原文。
- 暴露 Prometheus metrics。

## v0.1 推荐验收命令

```bash
make up
make migrate-up
make seed-demo
make smoke-test
make test
```

## 当前推荐验收命令

```bash
make production-checks
make verify-migration
make smoke-test
make load-test
make backup-demo
make restore-drill
```

## 给 Codex 的核心原则

- 每次只实现一个 Task。
- 每个 Task 完成后必须运行测试。
- 严禁使用 GORM AutoMigrate 管理数据库结构。
- 数据库迁移使用 golang-migrate。
- 数据访问使用 sqlc。
- 所有租户数据查询必须带 `org_id`。
- API Key、Admin Token、Provider API Key 不得明文存储。
- Prompt / Response 原文不得落库。
- 测试不得依赖真实外部 LLM Provider。
