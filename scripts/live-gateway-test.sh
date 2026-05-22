#!/usr/bin/env sh
set -eu

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
LIVE_PROVIDER_ENV_FILE="${LIVE_PROVIDER_ENV_FILE:-$ROOT_DIR/.live-provider.env}"
DEMO_ENV_FILE="${DEMO_ENV_FILE:-$ROOT_DIR/.demo.env}"

if [ -f "$LIVE_PROVIDER_ENV_FILE" ]; then
  # shellcheck disable=SC1090
  . "$LIVE_PROVIDER_ENV_FILE"
fi
if [ -f "$DEMO_ENV_FILE" ]; then
  # shellcheck disable=SC1090
  . "$DEMO_ENV_FILE"
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_value() {
  name="$1"
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    echo "missing $name; set it in LIVE_PROVIDER_ENV_FILE, DEMO_ENV_FILE, or the environment" >&2
    exit 1
  fi
}

api_json() {
  method="$1"
  url="$2"
  token="$3"
  body="$4"
  tmp_body="$(mktemp)"
  status="$(
    curl -sS -o "$tmp_body" -w "%{http_code}" \
      -X "$method" \
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

require_cmd curl
require_cmd jq

if [ "${LIVE_PROVIDER_TESTS_ENABLED:-}" != "true" ]; then
  echo "LIVE_PROVIDER_TESTS_ENABLED must be true to run real provider integration tests" >&2
  echo "Create .live-provider.env from configs/live-provider.example.env and set LIVE_PROVIDER_TESTS_ENABLED=true." >&2
  exit 1
fi

require_value GATEWAY_URL
require_value DEMO_ADMIN_TOKEN
require_value DEMO_API_KEY
require_value LIVE_PROVIDER_BASE_URL
require_value LIVE_PROVIDER_API_KEY
require_value LIVE_PROVIDER_MODEL

LIVE_PROVIDER_TIMEOUT_MS="${LIVE_PROVIDER_TIMEOUT_MS:-30000}"
LIVE_PROVIDER_MAX_TOKENS="${LIVE_PROVIDER_MAX_TOKENS:-16}"
LIVE_PROVIDER_INPUT_PRICE_MICRO_USD_PER_1K_TOKENS="${LIVE_PROVIDER_INPUT_PRICE_MICRO_USD_PER_1K_TOKENS:-1000}"
LIVE_PROVIDER_OUTPUT_PRICE_MICRO_USD_PER_1K_TOKENS="${LIVE_PROVIDER_OUTPUT_PRICE_MICRO_USD_PER_1K_TOKENS:-3000}"

run_id="$(date -u +%Y%m%dT%H%M%SZ)"
route_model="${LIVE_PROVIDER_ROUTE_MODEL:-live-provider-$run_id}"

provider_payload="$(
  jq -n \
    --arg name "live-provider-$run_id" \
    --arg base_url "$LIVE_PROVIDER_BASE_URL" \
    --arg api_key "$LIVE_PROVIDER_API_KEY" \
    --argjson timeout_ms "$LIVE_PROVIDER_TIMEOUT_MS" \
    '{
      name:$name,
      type:"openai_compatible",
      base_url:$base_url,
      api_key:$api_key,
      timeout_ms:$timeout_ms
    }'
)"
provider_response="$(api_json POST "$GATEWAY_URL/api/v1/admin/providers" "$DEMO_ADMIN_TOKEN" "$provider_payload")"
provider_id="$(printf "%s" "$provider_response" | jq -r '.id')"

model_payload="$(
  jq -n \
    --arg provider_id "$provider_id" \
    --arg provider_model_name "$LIVE_PROVIDER_MODEL" \
    --arg display_name "Live Provider $run_id" \
    --argjson input_price "$LIVE_PROVIDER_INPUT_PRICE_MICRO_USD_PER_1K_TOKENS" \
    --argjson output_price "$LIVE_PROVIDER_OUTPUT_PRICE_MICRO_USD_PER_1K_TOKENS" \
    '{
      provider_id:$provider_id,
      provider_model_name:$provider_model_name,
      display_name:$display_name,
      input_price_micro_usd_per_1k_tokens:$input_price,
      output_price_micro_usd_per_1k_tokens:$output_price,
      context_window:8192
    }'
)"
model_response="$(api_json POST "$GATEWAY_URL/api/v1/admin/models" "$DEMO_ADMIN_TOKEN" "$model_payload")"
model_id="$(printf "%s" "$model_response" | jq -r '.id')"

route_payload="$(
  jq -n \
    --arg name "live-provider-policy-$run_id" \
    --arg match_model "$route_model" \
    --arg provider_id "$provider_id" \
    --arg model_id "$model_id" \
    '{
      name:$name,
      match_model:$match_model,
      strategy:"single",
      targets:[{
        provider_id:$provider_id,
        model_id:$model_id,
        priority:1,
        weight:100
      }]
    }'
)"
route_response="$(api_json POST "$GATEWAY_URL/api/v1/admin/route-policies" "$DEMO_ADMIN_TOKEN" "$route_payload")"
route_policy_id="$(printf "%s" "$route_response" | jq -r '.id')"

chat_payload="$(
  jq -n \
    --arg model "$route_model" \
    --argjson max_tokens "$LIVE_PROVIDER_MAX_TOKENS" \
    '{
      model:$model,
      messages:[{role:"user", content:"Reply with exactly: live-provider-ok"}],
      max_tokens:$max_tokens,
      stream:false
    }'
)"
chat_response="$(api_json POST "$GATEWAY_URL/v1/chat/completions" "$DEMO_API_KEY" "$chat_payload")"
request_id="$(printf "%s" "$chat_response" | jq -r '.request_id // empty')"
total_tokens="$(printf "%s" "$chat_response" | jq -r '.usage.total_tokens // -1')"
total_cost_micro="$(printf "%s" "$chat_response" | jq -r '.cost.total_cost_micro // -1')"
if [ "$total_tokens" -le 0 ]; then
  echo "live gateway response has invalid total_tokens: $total_tokens" >&2
  exit 1
fi
if [ "$total_cost_micro" -lt 0 ]; then
  echo "live gateway response has invalid total_cost_micro: $total_cost_micro" >&2
  exit 1
fi

stream_payload="$(
  jq -n \
    --arg model "$route_model" \
    --argjson max_tokens "$LIVE_PROVIDER_MAX_TOKENS" \
    '{
      model:$model,
      messages:[{role:"user", content:"Reply with exactly: live-provider-ok"}],
      max_tokens:$max_tokens,
      stream:true
    }'
)"
stream_body="$(mktemp)"
stream_status="$(
  curl -sS -o "$stream_body" -w "%{http_code}" \
    -H "Authorization: Bearer $DEMO_API_KEY" \
    -H "Content-Type: application/json" \
    --data "$stream_payload" \
    "$GATEWAY_URL/v1/chat/completions"
)"
if [ "$stream_status" -lt 200 ] || [ "$stream_status" -ge 300 ]; then
  echo "HTTP $stream_status from streaming gateway request" >&2
  cat "$stream_body" >&2
  echo >&2
  rm -f "$stream_body"
  exit 1
fi
if ! grep -q '^data:' "$stream_body"; then
  echo "streaming gateway response did not contain SSE data events" >&2
  rm -f "$stream_body"
  exit 1
fi
if ! grep -q '\[DONE\]' "$stream_body"; then
  echo "streaming gateway response did not contain [DONE]" >&2
  rm -f "$stream_body"
  exit 1
fi
rm -f "$stream_body"

logs_response="$(
  curl -sS \
    -H "Authorization: Bearer $DEMO_ADMIN_TOKEN" \
    "$GATEWAY_URL/api/v1/admin/request-logs?status=success&request_model=$route_model&provider_id=$provider_id&model_id=$model_id&limit=10"
)"
matching_logs="$(printf "%s" "$logs_response" | jq '[.items[] | select(.provider_id == "'"$provider_id"'" and .model_id == "'"$model_id"'")] | length')"
if [ "$matching_logs" -lt 1 ]; then
  echo "request logs did not include successful live provider request" >&2
  exit 1
fi

summary_response="$(curl -sS -H "Authorization: Bearer $DEMO_ADMIN_TOKEN" "$GATEWAY_URL/api/v1/admin/usage/summary?group_by=model")"
summary_tokens="$(printf "%s" "$summary_response" | jq --arg model_id "$model_id" '[.items[] | select(.model_id == $model_id) | .total_tokens] | add // -1')"
summary_cost="$(printf "%s" "$summary_response" | jq --arg model_id "$model_id" '[.items[] | select(.model_id == $model_id) | .total_cost_micro_usd] | add // -1')"
if [ "$summary_tokens" -le 0 ]; then
  echo "usage summary has invalid live total_tokens: $summary_tokens" >&2
  exit 1
fi
if [ "$summary_cost" -lt 0 ]; then
  echo "usage summary has invalid live total_cost_micro_usd: $summary_cost" >&2
  exit 1
fi

echo "Live gateway provider test passed."
echo "provider_id=$provider_id model_id=$model_id route_policy_id=$route_policy_id route_model=$route_model"
echo "request_id=$request_id total_tokens=$total_tokens total_cost_micro=$total_cost_micro summary_tokens=$summary_tokens summary_cost_micro_usd=$summary_cost"
