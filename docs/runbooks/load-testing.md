# 部署环境压测 Runbook

## 目标

使用本仓库自带的 `scripts/load-test.sh` 对 `/v1/chat/completions` 做可重复的低成本压测，观察吞吐、延迟、HTTP 错误率和 smoke test 是否仍可通过。

## 前置条件

```bash
make up
make migrate-up
make seed-demo
```

脚本默认读取 `.demo.env` 中的 `DEMO_API_KEY`。

## Baseline 场景

```bash
LOAD_TEST_SCENARIO=baseline \
LOAD_TEST_REQUESTS=20 \
LOAD_TEST_VUS=2 \
make load-test
```

## Burst 场景

```bash
LOAD_TEST_SCENARIO=burst \
LOAD_TEST_REQUESTS=100 \
LOAD_TEST_VUS=10 \
make load-test
```

## Streaming 场景

```bash
LOAD_TEST_SCENARIO=streaming \
LOAD_TEST_REQUESTS=20 \
LOAD_TEST_VUS=2 \
make load-test
```

## 输出

报告写入 `.loadtest/report-<timestamp>.txt`，包含：

- request count
- success count
- HTTP 4xx / 5xx
- RPS
- p50 / p95 / p99 latency

`.loadtest/` 已被 git ignore。

## 验收

压测前后都应运行：

```bash
make smoke-test
```

压测请求使用固定非敏感 Prompt：`load test prompt`。系统仍不得把 Prompt / Response 原文写入数据库或日志。
