#!/usr/bin/env sh
set -eu

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

api_get() {
  url="$1"
  token="$2"
  tmp_body="$(mktemp)"
  status="$(
    curl -sS -o "$tmp_body" -w "%{http_code}" \
      -H "Authorization: Bearer $token" \
      "$url"
  )"
  if [ "$status" -lt 200 ] || [ "$status" -ge 300 ]; then
    echo "HTTP $status from $url" >&2
    cat "$tmp_body" >&2
    echo >&2
    rm -f "$tmp_body"
    exit 1
  fi
  cat "$tmp_body"
  rm -f "$tmp_body"
}

api_post_json() {
  url="$1"
  token="$2"
  body="$3"
  tmp_body="$(mktemp)"
  status="$(
    curl -sS -o "$tmp_body" -w "%{http_code}" \
      -X POST \
      -H "Authorization: Bearer $token" \
      -H "Content-Type: application/json" \
      --data "$body" \
      "$url"
  )"
  if [ "$status" -lt 200 ] || [ "$status" -ge 300 ]; then
    echo "HTTP $status from $url" >&2
    cat "$tmp_body" >&2
    echo >&2
    rm -f "$tmp_body"
    exit 1
  fi
  cat "$tmp_body"
  rm -f "$tmp_body"
}

require_value() {
  name="$1"
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    echo "missing $name; run make seed-demo first" >&2
    exit 1
  fi
}

require_cmd curl
require_cmd jq

DEMO_ENV_FILE="${DEMO_ENV_FILE:-.demo.env}"
if [ ! -f "$DEMO_ENV_FILE" ]; then
  echo "missing $DEMO_ENV_FILE; run make seed-demo first" >&2
  exit 1
fi

case "$DEMO_ENV_FILE" in
  */*) demo_env_path="$DEMO_ENV_FILE" ;;
  *) demo_env_path="./$DEMO_ENV_FILE" ;;
esac

. "$demo_env_path"

require_value GATEWAY_URL
require_value DEMO_ADMIN_TOKEN
require_value DEMO_API_KEY
require_value DEMO_MODEL_ID
require_value DEMO_MATCH_MODEL

chat_payload="$(
  jq -n \
    --arg model "$DEMO_MATCH_MODEL" \
    '{
      model:$model,
      messages:[{role:"user", content:"smoke test request"}],
      temperature:0.2,
      max_tokens:64,
      stream:false
    }'
)"
chat_response="$(api_post_json "$GATEWAY_URL/v1/chat/completions" "$DEMO_API_KEY" "$chat_payload")"
total_tokens="$(printf "%s" "$chat_response" | jq -r '.usage.total_tokens // -1')"
total_cost_micro="$(printf "%s" "$chat_response" | jq -r '.cost.total_cost_micro // -1')"
request_id="$(printf "%s" "$chat_response" | jq -r '.request_id // empty')"

if [ "$total_tokens" -lt 0 ]; then
  echo "chat response has invalid total_tokens: $total_tokens" >&2
  exit 1
fi
if [ "$total_cost_micro" -lt 0 ]; then
  echo "chat response has invalid total_cost_micro: $total_cost_micro" >&2
  exit 1
fi

logs_response="$(api_get "$GATEWAY_URL/api/v1/admin/request-logs?limit=10&status=success" "$DEMO_ADMIN_TOKEN")"
matching_logs="$(
  printf "%s" "$logs_response" |
    jq --arg model "$DEMO_MATCH_MODEL" '[.items[] | select(.request_model == $model and .status == "success")] | length'
)"
if [ "$matching_logs" -lt 1 ]; then
  echo "request logs did not include a successful $DEMO_MATCH_MODEL request" >&2
  exit 1
fi

summary_response="$(api_get "$GATEWAY_URL/api/v1/admin/usage/summary?group_by=model" "$DEMO_ADMIN_TOKEN")"
summary_cost="$(
  printf "%s" "$summary_response" |
    jq --arg model_id "$DEMO_MODEL_ID" '[.items[] | select(.model_id == $model_id) | .total_cost_micro_usd] | add // -1'
)"
summary_tokens="$(
  printf "%s" "$summary_response" |
    jq --arg model_id "$DEMO_MODEL_ID" '[.items[] | select(.model_id == $model_id) | .total_tokens] | add // -1'
)"
if [ "$summary_cost" -lt 0 ]; then
  echo "usage summary has invalid total_cost_micro_usd: $summary_cost" >&2
  exit 1
fi
if [ "$summary_tokens" -lt 0 ]; then
  echo "usage summary has invalid total_tokens: $summary_tokens" >&2
  exit 1
fi

echo "Smoke test passed."
echo "request_id=$request_id total_tokens=$total_tokens total_cost_micro=$total_cost_micro"
