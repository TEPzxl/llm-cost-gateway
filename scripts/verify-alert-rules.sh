#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if command -v promtool >/dev/null 2>&1; then
  promtool check rules "$ROOT_DIR"/deploy/prometheus/rules/*.yml
  promtool check config "$ROOT_DIR"/deploy/prometheus.yml
  exit 0
fi

docker run --rm --entrypoint sh \
  -v "$ROOT_DIR/deploy/prometheus.yml:/etc/prometheus/prometheus.yml:ro" \
  -v "$ROOT_DIR/deploy/prometheus/rules:/etc/prometheus/rules:ro" \
  prom/prometheus:v2.55.1 \
  -c 'promtool check rules /etc/prometheus/rules/*.yml'

docker run --rm --entrypoint promtool \
  -v "$ROOT_DIR/deploy/prometheus.yml:/etc/prometheus/prometheus.yml:ro" \
  -v "$ROOT_DIR/deploy/prometheus/rules:/etc/prometheus/rules:ro" \
  prom/prometheus:v2.55.1 \
  check config /etc/prometheus/prometheus.yml
