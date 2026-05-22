#!/usr/bin/env sh
set -eu

COMPOSE_FILE="${COMPOSE_FILE:-deploy/docker-compose.yml}"
BACKUP_DIR="${BACKUP_DIR:-.backups}"
CLICKHOUSE_SERVICE="${CLICKHOUSE_SERVICE:-clickhouse}"
CLICKHOUSE_USER="${CLICKHOUSE_USER:-llmgw}"
CLICKHOUSE_PASSWORD="${CLICKHOUSE_PASSWORD:-llmgw}"
CLICKHOUSE_DATABASE="${CLICKHOUSE_DATABASE:-llmgw}"
BACKUP_NAME="${BACKUP_NAME:-clickhouse-$(date -u +%Y%m%dT%H%M%SZ).sql}"

mkdir -p "$BACKUP_DIR"
backup_path="$BACKUP_DIR/$BACKUP_NAME"

docker compose -f "$COMPOSE_FILE" --project-directory . --profile analytics exec -T "$CLICKHOUSE_SERVICE" \
  clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DATABASE" \
  --query "SHOW CREATE DATABASE $CLICKHOUSE_DATABASE" > "$backup_path"

echo "Wrote ClickHouse schema backup to $backup_path"
