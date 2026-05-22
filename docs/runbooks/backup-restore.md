# 备份恢复 Runbook

## 范围

PostgreSQL 是事实存储，必须备份和演练恢复。Redis 在当前项目中主要保存限流计数、缓存和本地会话；限流计数与缓存可丢失，生产会话是否可丢失取决于部署策略。ClickHouse 是分析存储，可从 PostgreSQL facts 回补。

## PostgreSQL 备份

```bash
make backup-demo
```

备份文件写入 `.backups/`，该目录已被 git ignore。

单独执行：

```bash
BACKUP_DIR=.backups ./scripts/backup-postgres.sh
```

## PostgreSQL 恢复

恢复是破坏性操作，必须显式确认：

```bash
BACKUP_FILE=.backups/postgres-xxx.sql \
CONFIRM_RESTORE=restore-postgres \
./scripts/restore-postgres.sh
```

## 隔离恢复演练

```bash
make restore-drill
```

演练会：

1. 从当前 PostgreSQL 生成备份。
2. 启动独立 Docker Compose project。
3. 把备份恢复到隔离 PostgreSQL。
4. 验证 organizations、api_keys、providers、request_logs 计数可查询。
5. 自动删除隔离环境。

## Redis

当前脚本只生成备份策略说明：

```bash
./scripts/backup-redis.sh
```

生产环境建议使用托管 Redis 的快照/备份能力。限流计数和 cache 可丢失；如果 user session 放在 Redis 中，则需要按 RTO/RPO 要求启用持久化。

## ClickHouse

当前脚本提供 schema 备份示例：

```bash
./scripts/backup-clickhouse.sh
```

生产恢复建议恢复到隔离 database，再从 PostgreSQL facts 或 usage event pipeline 回补。

## 验收

```bash
make backup-demo
make restore-drill
make smoke-test
```
