# 告警 Runbook

## 验证规则

```bash
make verify-alert-rules
```

生产环境采集 `/metrics` 时，应通过受保护网络或反向代理暴露，避免公开访问。

## LLMGatewayHigh5xxRate

结论：Gateway error 请求率升高。

排查：

- 查看 Gateway 日志中的 error code 分布。
- 检查最近部署、数据库、Redis 和 Provider 可用性。
- 对比 `llmgw_provider_errors_total` 判断是否由上游 Provider 引起。

## LLMGatewayHighLatencyP95

结论：Gateway p95 延迟超过阈值。

排查：

- 检查 Provider latency、数据库连接和 Redis 延迟。
- 查看是否有 fallback/retry 激增。
- 如同时出现 rate limit 或 budget block，确认是否是异常流量。

## LLMGatewayRateLimitSpike

结论：API Key RPM 限流激增。

排查：

- 确认是否有租户流量突增。
- 检查 API Key 配额是否设置过低。
- 必要时临时提高限流并记录审计。

## LLMGatewayBudgetBlockSpike

结论：预算策略阻断请求激增。

排查：

- 检查预算配置和用量汇总。
- 确认是否存在异常成本策略或模型路由变化。
- 与租户确认是否需要提高预算。

## LLMProviderErrorSpike

结论：Provider 错误率升高。

排查：

- 查看 Provider health check。
- 检查 base_url、API Key、网络出口和 Provider 状态页。
- 如 fallback 可用，确认路由策略是否已降级到备用 Provider。

## LLMProviderNoSuccessfulRequests

结论：长时间没有成功 Provider 请求。

排查：

- 如果业务低流量，可能是正常状态。
- 如果有业务请求，检查 Provider 配置、路由策略和 Gateway 日志。

## LLMUsageEventPublishFailures

结论：Kafka usage event 发送失败。

排查：

- 检查 Kafka brokers、topic 和网络。
- 主请求不应被阻断，但外部分析消费者可能延迟。

## LLMAnalyticsWriteFailures

结论：ClickHouse 或 analytics sink 写入失败。

排查：

- 检查 ClickHouse 健康状态、凭据和磁盘容量。
- PostgreSQL 仍是事实存储，必要时从 PostgreSQL 明细回补分析数据。
