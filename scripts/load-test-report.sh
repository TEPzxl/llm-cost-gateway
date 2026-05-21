#!/usr/bin/env bash
set -euo pipefail

RESULTS_FILE="${1:-}"
DURATION_MS="${2:-0}"
if [[ -z "$RESULTS_FILE" || ! -f "$RESULTS_FILE" ]]; then
  echo "usage: scripts/load-test-report.sh <results.jsonl> <duration_ms>" >&2
  exit 1
fi

total="$(wc -l < "$RESULTS_FILE" | tr -d ' ')"
success="$(jq -r 'select(.status | startswith("2")) | .status' "$RESULTS_FILE" | wc -l | tr -d ' ')"
client_errors="$(jq -r 'select(.status | startswith("4")) | .status' "$RESULTS_FILE" | wc -l | tr -d ' ')"
server_errors="$(jq -r 'select(.status | startswith("5")) | .status' "$RESULTS_FILE" | wc -l | tr -d ' ')"
latencies="$(mktemp)"
trap 'rm -f "$latencies"' EXIT
jq -r '.latency_ms' "$RESULTS_FILE" | sort -n > "$latencies"

percentile() {
  local pct="$1"
  awk -v pct="$pct" '
    { values[NR] = $1 }
    END {
      if (NR == 0) { print 0; exit }
      idx = int((pct / 100) * NR)
      if (idx < 1) idx = 1
      if (idx > NR) idx = NR
      print values[idx]
    }
  ' "$latencies"
}

if [[ "$DURATION_MS" -le 0 ]]; then
  rps="0"
else
  rps="$(awk -v total="$total" -v ms="$DURATION_MS" 'BEGIN { printf "%.2f", total / (ms / 1000) }')"
fi

echo "Load test report"
echo "results_file=$RESULTS_FILE"
echo "duration_ms=$DURATION_MS"
echo "requests=$total"
echo "success=$success"
echo "http_4xx=$client_errors"
echo "http_5xx=$server_errors"
echo "rps=$rps"
echo "latency_p50_ms=$(percentile 50)"
echo "latency_p95_ms=$(percentile 95)"
echo "latency_p99_ms=$(percentile 99)"
