#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEMO_ENV_FILE="${DEMO_ENV_FILE:-$ROOT_DIR/.demo.env}"
if [[ -f "$DEMO_ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$DEMO_ENV_FILE"
  set +a
fi

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
DEMO_API_KEY="${DEMO_API_KEY:-}"
LOAD_TEST_SCENARIO="${LOAD_TEST_SCENARIO:-baseline}"
LOAD_TEST_REQUESTS="${LOAD_TEST_REQUESTS:-20}"
LOAD_TEST_VUS="${LOAD_TEST_VUS:-2}"
LOAD_TEST_MODEL="${LOAD_TEST_MODEL:-fast-chat}"
LOAD_TEST_OUTPUT_DIR="${LOAD_TEST_OUTPUT_DIR:-$ROOT_DIR/.loadtest}"

if [[ -z "$DEMO_API_KEY" ]]; then
  echo "DEMO_API_KEY is required; run make seed-demo first or set DEMO_ENV_FILE" >&2
  exit 1
fi
if ! [[ "$LOAD_TEST_REQUESTS" =~ ^[0-9]+$ ]] || [[ "$LOAD_TEST_REQUESTS" -le 0 ]]; then
  echo "LOAD_TEST_REQUESTS must be a positive integer" >&2
  exit 1
fi
if ! [[ "$LOAD_TEST_VUS" =~ ^[0-9]+$ ]] || [[ "$LOAD_TEST_VUS" -le 0 ]]; then
  echo "LOAD_TEST_VUS must be a positive integer" >&2
  exit 1
fi

case "$LOAD_TEST_SCENARIO" in
  baseline | burst | streaming) ;;
  *)
    echo "LOAD_TEST_SCENARIO must be one of baseline, burst, streaming" >&2
    exit 1
    ;;
esac

mkdir -p "$LOAD_TEST_OUTPUT_DIR"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
RESULTS_FILE="$LOAD_TEST_OUTPUT_DIR/results-$RUN_ID.jsonl"
REPORT_FILE="$LOAD_TEST_OUTPUT_DIR/report-$RUN_ID.txt"
REQUEST_BODY_FILE="$LOAD_TEST_OUTPUT_DIR/body-$RUN_ID.json"

stream=false
if [[ "$LOAD_TEST_SCENARIO" == "streaming" ]]; then
  stream=true
fi

jq -n \
  --arg model "$LOAD_TEST_MODEL" \
  --argjson stream "$stream" \
  '{
    model: $model,
    messages: [{role: "user", content: "load test prompt"}],
    max_tokens: 16,
    stream: $stream
  }' > "$REQUEST_BODY_FILE"

requests_per_worker=$(( (LOAD_TEST_REQUESTS + LOAD_TEST_VUS - 1) / LOAD_TEST_VUS ))
started_epoch_ms="$(date +%s%3N)"

run_one() {
  local worker="$1"
  local index="$2"
  local started ended latency status body_file
  body_file="$LOAD_TEST_OUTPUT_DIR/response-$RUN_ID-$worker-$index.json"
  started="$(date +%s%3N)"
  status="$(
    curl -sS -o "$body_file" -w "%{http_code}" \
      "$GATEWAY_URL/v1/chat/completions" \
      -H "Authorization: Bearer $DEMO_API_KEY" \
      -H "Content-Type: application/json" \
      --data @"$REQUEST_BODY_FILE" || true
  )"
  ended="$(date +%s%3N)"
  latency=$((ended - started))
  jq -c -n \
    --arg worker "$worker" \
    --arg index "$index" \
    --arg status "$status" \
    --argjson latency_ms "$latency" \
    --arg body_file "$body_file" \
    '{worker: $worker, index: $index, status: $status, latency_ms: $latency_ms, body_file: $body_file}' >> "$RESULTS_FILE"
}

worker() {
  local worker_id="$1"
  local count="$2"
  local i
  for ((i = 1; i <= count; i++)); do
    run_one "$worker_id" "$i"
    if [[ "$LOAD_TEST_SCENARIO" == "baseline" ]]; then
      sleep 0.1
    fi
  done
}

for ((worker_id = 1; worker_id <= LOAD_TEST_VUS; worker_id++)); do
  remaining=$((LOAD_TEST_REQUESTS - (worker_id - 1) * requests_per_worker))
  if [[ "$remaining" -le 0 ]]; then
    break
  fi
  count="$requests_per_worker"
  if [[ "$remaining" -lt "$count" ]]; then
    count="$remaining"
  fi
  worker "$worker_id" "$count" &
done
wait

ended_epoch_ms="$(date +%s%3N)"
duration_ms=$((ended_epoch_ms - started_epoch_ms))
"$ROOT_DIR/scripts/load-test-report.sh" "$RESULTS_FILE" "$duration_ms" > "$REPORT_FILE"
cat "$REPORT_FILE"
