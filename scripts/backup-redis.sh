#!/usr/bin/env sh
set -eu

BACKUP_DIR="${BACKUP_DIR:-.backups}"
mkdir -p "$BACKUP_DIR"
note="$BACKUP_DIR/redis-backup-note-$(date -u +%Y%m%dT%H%M%SZ).txt"
cat > "$note" <<'NOTE'
Redis stores rate-limit counters, cache entries, and local sessions in the demo stack.
Rate-limit counters and cache entries are disposable. For production, use managed Redis
persistence or provider-native snapshots if sessions must survive failover.
NOTE
echo "Wrote Redis backup note to $note"
