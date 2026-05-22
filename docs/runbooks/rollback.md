# 生产回滚 Runbook

本文档用于生产发布失败、试运行异常或安全风险暴露时执行回滚。回滚目标是先恢复服务可用性和数据一致性，再处理根因。

## 回滚触发条件

任一条件满足即可进入回滚评估：

- Gateway 5xx rate 或 p95 latency 持续触发 P1 告警。
- 数据库迁移后核心 API 无法读写。
- magic link 和 Admin Token 登录均不可用。
- Provider 调用大面积失败，且 route fallback 无法缓解。
- request_logs、usage_records 或 cost_records 缺失或异常。
- 真实 Provider 成本失控。
- 密钥轮换导致 token 或 Provider secret 无法校验。
- 备份任务失败且无法确认可恢复性。

## 职责边界

| 角色 | 职责 | 联系方式 |
|---|---|---|
| Incident Commander | 决定回滚、协调沟通、记录时间线 |  |
| Backend Owner | 应用版本、配置、迁移和 Provider route 回滚 |  |
| SRE Owner | 部署系统、数据库、Redis、监控和备份恢复 |  |
| Security Owner | 密钥、token、访问边界和审计确认 |  |

## 回滚前确认

| 项目 | 要求 | 负责人 | 证据 |
|---|---|---|---|
| 当前故障范围 | 受影响租户、API、时间窗口明确 |  |  |
| 最近成功版本 | commit SHA、镜像 tag、配置版本明确 |  |  |
| 数据库迁移状态 | 已应用 migration 列表明确 |  |  |
| 备份状态 | 最近 PostgreSQL 备份文件可访问 |  |  |
| 密钥版本 | active 与 previous key version 明确 |  |  |
| Provider route | 当前 route policy 和 fallback 目标明确 |  |  |

## 应用版本回滚

1. 暂停扩大流量或切换入口限流。
2. 部署上一稳定镜像或上一稳定 commit。
3. 保持数据库不变，先验证旧版本是否兼容当前 schema。
4. 验证：

```bash
curl -f http://<gateway>/healthz
```

5. 执行受控 smoke test：

```bash
make smoke-test
```

如果旧版本不兼容当前 schema，进入数据库迁移回滚评估。

## 配置回滚

配置回滚优先级：

1. 恢复上一版本 `APP_ENV`、数据库、Redis、Kafka、ClickHouse、tracing 配置。
2. 恢复上一版本 `AUTH_*` 和 SMTP 配置。
3. 恢复上一版本 `TOKEN_HASH_SECRET_VERSION` 和 `TOKEN_HASH_SECRET_KEYRING`。
4. 恢复上一版本 `SECRET_ENCRYPTION_KEY_VERSION` 和 `SECRET_ENCRYPTION_KEYRING`。
5. 重启应用并验证 health、登录和 Provider health。

注意：

- 不要删除旧 token hash secret，直到确认所有旧 token 已过期或重新签发。
- 不要删除旧 Provider encryption key，直到确认所有 Provider secret 已重新加密到目标版本。

## 数据库迁移回滚限制

默认策略是前向修复或应用版本兼容当前 schema。只有满足以下条件才执行 migration down：

- 已确认迁移是可逆的。
- 已确认 down migration 不会丢失生产事实数据。
- 已完成当前数据库备份。
- Release Owner、Backend Owner、SRE Owner 共同签核。

执行前备份：

```bash
./scripts/backup-postgres.sh
```

执行回滚：

```bash
make migrate-down
```

回滚后验证：

```bash
make verify-migration
make smoke-test
```

若 down migration 涉及数据丢失，不允许直接执行；改为禁用新功能、恢复旧应用或应用前向修复。

## Provider Route 回滚

适用于真实 Provider 错误、成本异常或新 route policy 导致请求失败。

1. 使用 Admin API 或 Console 将目标 route policy 切回上一稳定 provider/model。
2. 如有 fallback，降低异常 provider 的 priority 或临时移除。
3. 验证 provider health。
4. 执行低 token smoke 请求。
5. 观察 request_logs、usage summary 和成本记录。

记录：

| RoutePolicy | 回滚前目标 | 回滚后目标 | 操作人 | 时间 |
|---|---|---|---|---|
|  |  |  |  |  |

## 密钥轮换回滚

Provider secret encryption key：

- 如果 re-encryption 已完成，但旧应用无法解密新版本密文，优先回滚应用到支持 keyring 的版本。
- 如果必须回到旧 active key，确保 `SECRET_ENCRYPTION_KEYRING` 同时包含新旧 key，再执行 `cmd/secret-rotate` 将 secret 重新加密到旧版本。
- 不允许在未确认密文版本前删除任何 keyring 条目。

Token hash secret：

- 将 `TOKEN_HASH_SECRET_KEYRING` 恢复为包含旧 active secret 和新 secret 的双读配置。
- 新 token 可以切回旧 active version。
- 已签发 token 不需要批量改库，因为校验采用候选 hash 双读。
- 观察旧 token 使用情况，确认稳定后再规划下一次轮换。

验证：

```bash
make secret-rotate
make smoke-test
```

## 数据恢复

仅在事实数据损坏或误删时使用恢复流程。恢复必须进入隔离环境先验证：

```bash
make restore-drill
```

生产恢复前必须确认：

- 恢复目标和当前生产库不是同一实例，除非已完成停机和签核。
- 备份时间点符合 RPO。
- 恢复后 migration version、organizations、api_keys、providers、models、route_policies、request_logs、usage_records、cost_records 数量符合预期。

详见 `docs/runbooks/backup-restore.md`。

## 回滚后验证

必须执行：

```bash
make production-checks
make smoke-test
make verify-alert-rules
```

如涉及 Provider：

```bash
make live-provider-test
make live-gateway-test
```

如涉及备份恢复：

```bash
make restore-drill
```

## 沟通与记录

| 时间 | 事件 | 操作人 | 证据 |
|---|---|---|---|
|  |  |  |  |

回滚完成后 24 小时内补充：

- 故障时间线。
- 根因假设与证据。
- 数据影响范围。
- 成本影响范围。
- 后续修复 owner 和截止时间。

## 最终签核

| 角色 | 结论 | 备注 | 签名 | 时间 |
|---|---|---|---|---|
| Incident Commander |  |  |  |  |
| Backend Owner |  |  |  |  |
| SRE Owner |  |  |  |  |
| Security Owner |  |  |  |  |
