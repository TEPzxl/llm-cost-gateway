#!/usr/bin/env sh
set -eu

BACKUP_DIR="${BACKUP_DIR:-.backups}"
RESTORE_DRILL_PROJECT="${RESTORE_DRILL_PROJECT:-llmgw-restore-drill}"
COMPOSE_FILE="${COMPOSE_FILE:-deploy/docker-compose.yml}"
backup_name="restore-drill-$(date -u +%Y%m%dT%H%M%SZ).sql"

mkdir -p "$BACKUP_DIR"
BACKUP_DIR="$BACKUP_DIR" BACKUP_NAME="$backup_name" COMPOSE_FILE="$COMPOSE_FILE" ./scripts/backup-postgres.sh
backup_file="$BACKUP_DIR/$backup_name"
override_file="$(mktemp)"
cat > "$override_file" <<'YAML'
services:
  postgres:
    ports: !reset []
  redis:
    ports: !reset []
  gateway:
    ports: !reset []
  console:
    ports: !reset []
  prometheus:
    ports: !reset []
YAML

cleanup() {
  docker compose -p "$RESTORE_DRILL_PROJECT" -f "$COMPOSE_FILE" -f "$override_file" --project-directory . down -v >/dev/null 2>&1 || true
  rm -f "$override_file"
}
trap cleanup EXIT

docker compose -p "$RESTORE_DRILL_PROJECT" -f "$COMPOSE_FILE" -f "$override_file" --project-directory . up -d postgres

for _ in $(seq 1 30); do
  if docker compose -p "$RESTORE_DRILL_PROJECT" -f "$COMPOSE_FILE" -f "$override_file" --project-directory . exec -T postgres pg_isready -U llmgw -d llmgw >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

cat "$backup_file" | docker compose -p "$RESTORE_DRILL_PROJECT" -f "$COMPOSE_FILE" -f "$override_file" --project-directory . exec -T postgres \
  psql -U llmgw -d llmgw -v ON_ERROR_STOP=1 >/dev/null

counts="$(
  docker compose -p "$RESTORE_DRILL_PROJECT" -f "$COMPOSE_FILE" -f "$override_file" --project-directory . exec -T postgres \
    psql -U llmgw -d llmgw -tAc "SELECT (SELECT count(*) FROM organizations), (SELECT count(*) FROM api_keys), (SELECT count(*) FROM providers), (SELECT count(*) FROM request_logs);"
)"
echo "Restore drill passed. counts=$counts backup_file=$backup_file"
