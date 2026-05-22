#!/usr/bin/env sh
set -eu

COMPOSE_FILE="${COMPOSE_FILE:-deploy/docker-compose.yml}"
POSTGRES_SERVICE="${POSTGRES_SERVICE:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-llmgw}"
POSTGRES_DB="${POSTGRES_DB:-llmgw}"
BACKUP_FILE="${BACKUP_FILE:-}"
CONFIRM_RESTORE="${CONFIRM_RESTORE:-}"

if [ -z "$BACKUP_FILE" ] || [ ! -f "$BACKUP_FILE" ]; then
  echo "BACKUP_FILE must point to an existing PostgreSQL backup" >&2
  exit 1
fi
if [ "$CONFIRM_RESTORE" != "restore-postgres" ]; then
  echo "refusing destructive restore; set CONFIRM_RESTORE=restore-postgres" >&2
  exit 1
fi

cat "$BACKUP_FILE" | docker compose -f "$COMPOSE_FILE" --project-directory . exec -T "$POSTGRES_SERVICE" \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1

echo "Restored PostgreSQL backup from $BACKUP_FILE"
