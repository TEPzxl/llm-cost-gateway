#!/usr/bin/env sh
set -eu

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
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

write_export() {
  name="$1"
  value="$2"
  printf "export %s='%s'\n" "$name" "$value" >>"$env_tmp"
}

require_cmd curl
require_cmd jq

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
PLATFORM_BOOTSTRAP_TOKEN="${PLATFORM_BOOTSTRAP_TOKEN:-dev-bootstrap-token}"
DEMO_ENV_FILE="${DEMO_ENV_FILE:-.demo.env}"
DEMO_SUFFIX="${DEMO_SUFFIX:-$(date +%Y%m%d%H%M%S)}"
DEMO_ORG_NAME="${DEMO_ORG_NAME:-Demo Org}"
DEMO_SLUG="${DEMO_SLUG:-llmgw-demo-$DEMO_SUFFIX}"
DEMO_OWNER_EMAIL="${DEMO_OWNER_EMAIL:-owner@example.com}"
DEMO_OWNER_DISPLAY_NAME="${DEMO_OWNER_DISPLAY_NAME:-Demo Owner}"
DEMO_MATCH_MODEL="${DEMO_MATCH_MODEL:-fast-chat}"

org_payload="$(jq -n --arg name "$DEMO_ORG_NAME" --arg slug "$DEMO_SLUG" '{name:$name, slug:$slug}')"
org_response="$(api_json POST "$GATEWAY_URL/api/v1/platform/orgs" "$PLATFORM_BOOTSTRAP_TOKEN" "$org_payload")"
org_id="$(printf "%s" "$org_response" | jq -r '.id')"

admin_token_payload="$(jq -n --arg name "demo-admin-$DEMO_SUFFIX" '{name:$name, scopes:["admin:*"], expires_at:null}')"
admin_token_response="$(api_json POST "$GATEWAY_URL/api/v1/platform/orgs/$org_id/admin-tokens" "$PLATFORM_BOOTSTRAP_TOKEN" "$admin_token_payload")"
admin_token="$(printf "%s" "$admin_token_response" | jq -r '.token')"

member_payload="$(jq -n --arg email "$DEMO_OWNER_EMAIL" --arg display_name "$DEMO_OWNER_DISPLAY_NAME" '{email:$email, display_name:$display_name, role:"owner"}')"
member_response="$(api_json POST "$GATEWAY_URL/api/v1/admin/members" "$admin_token" "$member_payload")"
member_id="$(printf "%s" "$member_response" | jq -r '.membership_id')"

api_key_payload="$(jq -n --arg name "demo-gateway-key-$DEMO_SUFFIX" '{name:$name, scopes:["chat.completions"], rpm_limit:60, expires_at:null}')"
api_key_response="$(api_json POST "$GATEWAY_URL/api/v1/admin/api-keys" "$admin_token" "$api_key_payload")"
api_key_id="$(printf "%s" "$api_key_response" | jq -r '.id')"
api_key="$(printf "%s" "$api_key_response" | jq -r '.key')"

provider_payload="$(jq -n --arg name "demo-mock-provider-$DEMO_SUFFIX" '{name:$name, type:"mock", base_url:null, api_key:null, timeout_ms:30000}')"
provider_response="$(api_json POST "$GATEWAY_URL/api/v1/admin/providers" "$admin_token" "$provider_payload")"
provider_id="$(printf "%s" "$provider_response" | jq -r '.id')"

model_payload="$(
  jq -n \
    --arg provider_id "$provider_id" \
    --arg provider_model_name "mock-small" \
    --arg display_name "Mock Small" \
    '{
      provider_id:$provider_id,
      provider_model_name:$provider_model_name,
      display_name:$display_name,
      input_price_micro_usd_per_1k_tokens:100,
      output_price_micro_usd_per_1k_tokens:200,
      context_window:8192
    }'
)"
model_response="$(api_json POST "$GATEWAY_URL/api/v1/admin/models" "$admin_token" "$model_payload")"
model_id="$(printf "%s" "$model_response" | jq -r '.id')"

route_payload="$(
  jq -n \
    --arg name "demo-fast-chat-policy-$DEMO_SUFFIX" \
    --arg match_model "$DEMO_MATCH_MODEL" \
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
route_response="$(api_json POST "$GATEWAY_URL/api/v1/admin/route-policies" "$admin_token" "$route_payload")"
route_policy_id="$(printf "%s" "$route_response" | jq -r '.id')"

budget_payload="$(jq -n --arg name "demo-monthly-budget-$DEMO_SUFFIX" '{name:$name, scope_type:"org", scope_id:null, period:"monthly", limit_micro_usd:100000000, action:"block"}')"
budget_response="$(api_json POST "$GATEWAY_URL/api/v1/admin/budgets" "$admin_token" "$budget_payload")"
budget_id="$(printf "%s" "$budget_response" | jq -r '.id')"

env_tmp="$(mktemp)"
umask 077
write_export GATEWAY_URL "$GATEWAY_URL"
write_export DEMO_ORG_ID "$org_id"
write_export DEMO_ORG_SLUG "$DEMO_SLUG"
write_export DEMO_ADMIN_TOKEN "$admin_token"
write_export DEMO_OWNER_EMAIL "$DEMO_OWNER_EMAIL"
write_export DEMO_MEMBER_ID "$member_id"
write_export DEMO_API_KEY_ID "$api_key_id"
write_export DEMO_API_KEY "$api_key"
write_export DEMO_PROVIDER_ID "$provider_id"
write_export DEMO_MODEL_ID "$model_id"
write_export DEMO_ROUTE_POLICY_ID "$route_policy_id"
write_export DEMO_BUDGET_ID "$budget_id"
write_export DEMO_MATCH_MODEL "$DEMO_MATCH_MODEL"
mv "$env_tmp" "$DEMO_ENV_FILE"

echo "Seeded demo org $DEMO_SLUG."
echo "Wrote demo credentials to $DEMO_ENV_FILE."
