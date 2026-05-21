# Observability

本文档记录当前网关的可观测性能力：Prometheus metrics 与可选 OpenTelemetry tracing。

## Metrics

`/metrics` 暴露 Prometheus 指标，Docker Compose 默认启动 Prometheus 并读取 `deploy/prometheus.yml`。

## OpenTelemetry Tracing

Tracing 默认关闭。启用后，服务会通过 OTLP HTTP exporter 发送 trace。

### 配置项

YAML：

```yaml
tracing_enabled: false
tracing_otlp_endpoint: localhost:4318
tracing_insecure: true
tracing_service_name: llm-cost-gateway
```

环境变量：

```fish
set -x TRACING_ENABLED true
set -x TRACING_OTLP_ENDPOINT localhost:4318
set -x TRACING_INSECURE true
set -x TRACING_SERVICE_NAME llm-cost-gateway
```

`tracing_otlp_endpoint` 可以使用 `host:port`，也可以使用包含 scheme 的 URL。`tracing_insecure=true` 时使用 HTTP。

### Span

当前会创建以下 span：

- `gateway.chat`
- `gateway.stream_chat`
- `gateway.budget_check`
- `gateway.api_key_quota_check`
- `gateway.routing`
- `gateway.provider_attempt`
- `provider.openai.chat`
- `provider.openai.stream_chat`

### 敏感信息边界

Span attribute 只允许记录请求标识、租户标识、provider/model 标识、状态和错误码。

不得记录：

- Gateway API Key
- Provider API Key
- Authorization header
- Prompt 原文
- Response 原文
- PII 命中文本
- cache key 原文

测试中使用 span recorder 验证 provider trace header 注入，并断言 span attribute 不包含 prompt/API key 明文。
