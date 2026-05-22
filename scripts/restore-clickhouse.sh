#!/usr/bin/env sh
set -eu

CONFIRM_RESTORE="${CONFIRM_RESTORE:-}"
if [ "$CONFIRM_RESTORE" != "restore-clickhouse" ]; then
  echo "refusing ClickHouse restore placeholder; set CONFIRM_RESTORE=restore-clickhouse" >&2
  exit 1
fi
echo "ClickHouse restore is environment-specific. Restore into an isolated database and backfill from PostgreSQL facts."
