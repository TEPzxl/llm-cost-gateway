# 密钥轮换 Runbook

## 适用范围

本文档覆盖 Provider API Key 与 Budget Alert webhook secret 的加密密钥轮换，以及 Admin Token、Gateway API Key、user session、magic link token 的 hash secret 双读单写轮换。

## 配置

单 key 配置：

```yaml
secret_encryption_key: 0123456789abcdef0123456789abcdef
secret_encryption_key_version: 1
secret_encryption_keyring: ""
token_hash_secret: change-me-token-hash-secret
token_hash_secret_version: 1
token_hash_secret_keyring: ""
```

轮换到 version 2：

```yaml
secret_encryption_key: fedcba98765432100123456789abcdef
secret_encryption_key_version: 2
secret_encryption_keyring: "1:0123456789abcdef0123456789abcdef,2:fedcba98765432100123456789abcdef"
token_hash_secret: new-token-hash-secret
token_hash_secret_version: 2
token_hash_secret_keyring: "1:old-token-hash-secret,2:new-token-hash-secret"
```

生产环境必须保证 active version 存在于 keyring 中。旧 version 在所有 Provider secret 和 webhook secret 完成 re-encryption 之前不能删除。

Token hash secret 轮换采用双读单写：

- 新创建的 Admin Token、Gateway API Key、session 和 magic link token 使用 active secret 写入 hash。
- 认证时会尝试 keyring 中所有 secret，因此 grace period 内旧 token 仍可用。
- grace period 结束后，删除旧 secret 前应确认旧 token 已过期、撤销或重新签发。

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

CLI 分别输出 Provider secret 和 webhook secret 的数量摘要，不输出任何 secret 明文。

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
