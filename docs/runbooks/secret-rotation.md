# 密钥轮换 Runbook

## 适用范围

本文档覆盖 Provider API Key 的加密密钥轮换。当前实现使用 `provider_secrets.key_version` 标记密文使用的 AES-GCM key version。

Token hash secret 的双读单写轮换将在后续任务中补齐。

## 配置

单 key 配置：

```yaml
secret_encryption_key: 0123456789abcdef0123456789abcdef
secret_encryption_key_version: 1
secret_encryption_keyring: ""
```

轮换到 version 2：

```yaml
secret_encryption_key: fedcba98765432100123456789abcdef
secret_encryption_key_version: 2
secret_encryption_keyring: "1:0123456789abcdef0123456789abcdef,2:fedcba98765432100123456789abcdef"
```

生产环境必须保证 active version 存在于 keyring 中。旧 version 在所有 Provider secret 完成 re-encryption 之前不能删除。

## Dry Run

先执行 dry-run，确认会扫描和轮换的记录数：

```bash
go run ./cmd/secret-rotate -config configs/config.example.yaml -dry-run
```

限定单个组织：

```bash
go run ./cmd/secret-rotate \
  -config configs/config.example.yaml \
  -org-id <org_id> \
  -dry-run
```

CLI 只输出数量摘要，不输出 Provider API Key 明文。

## 实际轮换

确认配置和 dry-run 结果后执行：

```bash
go run ./cmd/secret-rotate -config configs/config.example.yaml
```

执行完成后验证：

```bash
make test
make smoke-test
```

如果使用真实 Provider，还应执行真实 Provider 联调脚本。

## 回滚

如果应用发布后需要回滚：

1. 保留旧 key 和新 key 都在 `secret_encryption_keyring` 中。
2. 将 `secret_encryption_key_version` 改回旧 active version。
3. 重启服务。
4. 对新 version 密文执行一次 re-encryption，改回旧 version。

不要在确认所有密文都不再使用某个 version 前删除该 key。

## 审计证据

每次轮换应保存：

- 使用的配置版本。
- dry-run 输出。
- 实际轮换输出。
- 轮换前后的 smoke test 结果。
- 真实 Provider 联调结果，若适用。
