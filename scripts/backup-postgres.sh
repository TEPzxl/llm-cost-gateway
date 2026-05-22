#!/usr/bin/env sh
set -eu

COMPOSE_FILE="${COMPOSE_FILE:-deploy/docker-compose.yml}"
BACKUP_DIR="${BACKUP_DIR:-.backups}"
BACKUP_NAME="${BACKUP_NAME:-postgres-$(date -u +%Y%m%dT%H%M%SZ).sql}"
POSTGRES_SERVICE="${POSTGRES_SERVICE:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-llmgw}"
POSTGRES_DB="${POSTGRES_DB:-llmgw}"

mkdir -p "$BACKUP_DIR"
backup_path="$BACKUP_DIR/$BACKUP_NAME"

docker compose -f "$COMPOSE_FILE" --project-directory . exec -T "$POSTGRES_SERVICE" \
  pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-privileges > "$backup_path"

echo "Wrote PostgreSQL backup to $backup_path"
