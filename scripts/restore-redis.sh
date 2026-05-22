#!/usr/bin/env sh
set -eu

CONFIRM_RESTORE="${CONFIRM_RESTORE:-}"
if [ "$CONFIRM_RESTORE" != "restore-redis" ]; then
  echo "refusing Redis restore placeholder; set CONFIRM_RESTORE=restore-redis" >&2
  exit 1
fi
echo "Redis restore is environment-specific. Use managed Redis snapshot restore for production."
