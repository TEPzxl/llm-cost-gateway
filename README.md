# LLM Cost Gateway

SaaS multi-tenant LLM gateway and CostOps platform.

Current implementation status: phase 1 v0.1 SaaS MVP.

## Requirements

- Go 1.22+
- Node.js 22+
- npm
- Docker and Docker Compose
- `curl`
- `jq`

## Quick Demo

Start services, apply migrations, seed demo data, and run an end-to-end smoke test:

```bash
make up
make migrate-up
make seed-demo
make smoke-test
```

`make seed-demo` writes local demo credentials to `.demo.env`. Use `DEMO_ADMIN_TOKEN` from that file to log in to the console.

## Local Services

| Service | URL |
|---|---|
| Gateway | `http://localhost:8080` |
| Console | `http://localhost:3000` |
| Prometheus | `http://localhost:9090` |
| PostgreSQL | `localhost:5432` |
| Redis | `localhost:6379` |

Health check:

```bash
curl http://localhost:8080/healthz
```

Expected response:

```json
{"status":"ok"}
```

## Common Commands

Start the local stack:

```bash
make up
```

Stop the local stack:

```bash
make down
```

Run database migrations:

```bash
make migrate-up
```

Rollback local migrations:

```bash
make migrate-down
```

Verify migration tenant and value constraints:

```bash
make verify-migration
```

Seed demo resources:

```bash
make seed-demo
```

Run the smoke test:

```bash
make smoke-test
```

Run backend tests:

```bash
make test
```

Run full Go package tests:

```bash
go test ./...
```

Build the management console:

```bash
make console-build
```

## Console

The management console runs at:

```text
http://localhost:3000
```

Login with `DEMO_ADMIN_TOKEN` from `.demo.env`.

Implemented pages:

- Dashboard
- API Keys
- Providers
- Models
- Route Policies
- Budgets
- Request Logs
- Usage Summary

The console stores the Admin Token in `sessionStorage` for the current browser session and does not write it to `localStorage`.

## Gateway API

Use `DEMO_API_KEY` from `.demo.env`:

```bash
curl -sS http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $DEMO_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "fast-chat",
    "messages": [{"role": "user", "content": "hello"}],
    "stream": false
  }'
```

The response includes usage and cost. Cost values use integer micro USD, not floating-point money.

## Metrics

Prometheus UI:

```text
http://localhost:9090
```

Gateway metrics endpoint:

```bash
curl http://localhost:8080/metrics
```

## Documentation

- `docs/api.md`: implemented v0.1 API reference
- `docs/operations.md`: local operations guide
- `docs/demo.md`: phase 1 demo walkthrough
- `docs/00_MASTER_SDD.md`: master software design document
- `docs/01_PHASE_1_V0_1_SAAS_MVP.md`: phase 1 task plan
- `docs/02_PHASE_2_V0_2_PRODUCTION_READY.md`: phase 2 task plan

## Configuration

Example configuration lives at `configs/config.example.yaml`.

Environment variables override config file values:

- `APP_ENV`
- `SERVER_PORT`
- `DATABASE_URL`
- `REDIS_URL`
- `PLATFORM_BOOTSTRAP_TOKEN`
- `TOKEN_HASH_SECRET`
- `SECRET_ENCRYPTION_KEY`
- `LOG_LEVEL`

`SECRET_ENCRYPTION_KEY` must be 32 bytes for AES-256.
