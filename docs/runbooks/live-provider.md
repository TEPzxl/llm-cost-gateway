# 真实 Provider 联调 Runbook

本文档用于在受控环境中验证 `openai_compatible` Provider 能真实调用外部 LLM 服务。默认测试不会访问外网，只有显式开启后才会发起请求。

## 适用范围

- 验证非流式 `/chat/completions` 调用能返回 response id 和 usage。
- 验证流式 SSE 调用能收到数据事件和 `[DONE]`。
- 验证 Gateway Provider Adapter 与 OpenAI-compatible API 的基础兼容性。

不覆盖：

- 供应商账号余额、配额和账单准确性。
- 生产流量压测。
- Gateway 端到端租户、路由和计费链路。端到端链路仍使用 `make smoke-test` 和真实环境灰度验证。

## 前置条件

- 已获得测试专用 Provider API Key。
- API Key 只允许最小权限和低额度预算。
- 联调环境允许访问目标 Provider 的公网地址。
- 不要在终端输出、截图、issue、PR 或文档中粘贴真实 API Key。

## 配置

复制模板：

```bash
cp configs/live-provider.example.env .live-provider.env
```

编辑 `.live-provider.env`：

```bash
vim .live-provider.env
```

示例字段：

```bash
LIVE_PROVIDER_TESTS_ENABLED=true
LIVE_PROVIDER_BASE_URL=https://api.openai.com/v1
LIVE_PROVIDER_API_KEY=...
LIVE_PROVIDER_MODEL=gpt-4.1-mini
LIVE_PROVIDER_TIMEOUT_MS=30000
LIVE_PROVIDER_MAX_TOKENS=16
```

`.live-provider.env` 已加入 `.gitignore`，不得提交。

## 执行

运行：

```bash
make live-provider-test
```

脚本会先检查：

- `LIVE_PROVIDER_TESTS_ENABLED=true`
- `LIVE_PROVIDER_BASE_URL` 非空
- `LIVE_PROVIDER_API_KEY` 非空
- `LIVE_PROVIDER_MODEL` 非空

检查通过后执行：

```bash
go test ./internal/provider/openai_compatible -run TestLiveOpenAICompatibleProvider -count=1 -v
```

## 成功标准

- 非流式用例通过，并且 `usage.total_tokens > 0`。
- 流式用例通过，并且至少收到一个数据事件和一个 `[DONE]` 事件。
- 测试输出不得包含 API Key。

## 常见失败

`LIVE_PROVIDER_TESTS_ENABLED must be true`

表示没有显式开启真实联调。确认 `.live-provider.env` 存在，且设置了 `LIVE_PROVIDER_TESTS_ENABLED=true`。

`provider returned status 401` 或 `403`

通常是 API Key 无效、权限不足，或目标 Provider 不接受当前 base URL。

`provider returned status 429`

通常是账号限流、余额不足或测试模型配额不足。降低执行频率或更换测试账号。

`provider response missing usage`

当前非流式适配器要求 Provider 返回 usage，因为成本计算依赖 token 用量。若某个兼容供应商默认不返回 usage，需要先确认其 API 是否支持开启 usage 返回，再决定是否扩展适配器。

流式用例没有 usage 不会失败。当前流式路径只验证 SSE 数据和结束事件，因为不同供应商对流式 usage 的返回策略不一致。

## 安全要求

- 使用测试专用 API Key，不复用生产主密钥。
- API Key 必须存放在 `.live-provider.env`、CI secret 或密钥管理系统中。
- 测试完成后如不再需要，应在 Provider 控制台撤销该 Key。
- 任何失败日志只允许保留状态码、错误码、request id，不允许保留密钥或 Prompt/Response 原文。
