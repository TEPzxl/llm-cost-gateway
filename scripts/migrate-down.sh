#!/usr/bin/env sh
set -eu

COMPOSE_FILE="${COMPOSE_FILE:-deploy/docker-compose.yml}"
MIGRATE_DATABASE_URL="${MIGRATE_DATABASE_URL:-postgres://llmgw:llmgw@postgres:5432/llmgw?sslmode=disable}"

docker compose -f "$COMPOSE_FILE" --project-directory . run --rm migrate \
  -path=/migrations \
  -database "$MIGRATE_DATABASE_URL" \
  down -all
