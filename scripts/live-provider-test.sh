#!/usr/bin/env sh
set -eu

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
LIVE_PROVIDER_ENV_FILE="${LIVE_PROVIDER_ENV_FILE:-$ROOT_DIR/.live-provider.env}"

if [ -f "$LIVE_PROVIDER_ENV_FILE" ]; then
  # shellcheck disable=SC1090
  . "$LIVE_PROVIDER_ENV_FILE"
fi

require_value() {
  name="$1"
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    echo "missing $name; set it in LIVE_PROVIDER_ENV_FILE or the environment" >&2
    exit 1
  fi
}

if [ "${LIVE_PROVIDER_TESTS_ENABLED:-}" != "true" ]; then
  echo "LIVE_PROVIDER_TESTS_ENABLED must be true to run real provider integration tests" >&2
  echo "Create .live-provider.env from configs/live-provider.example.env and set LIVE_PROVIDER_TESTS_ENABLED=true." >&2
  exit 1
fi

require_value LIVE_PROVIDER_BASE_URL
require_value LIVE_PROVIDER_API_KEY
require_value LIVE_PROVIDER_MODEL

export LIVE_PROVIDER_TESTS_ENABLED
export LIVE_PROVIDER_BASE_URL
export LIVE_PROVIDER_API_KEY
export LIVE_PROVIDER_MODEL
export LIVE_PROVIDER_TIMEOUT_MS="${LIVE_PROVIDER_TIMEOUT_MS:-30000}"
export LIVE_PROVIDER_MAX_TOKENS="${LIVE_PROVIDER_MAX_TOKENS:-16}"

go test ./internal/provider/openai_compatible -run TestLiveOpenAICompatibleProvider -count=1 -v
