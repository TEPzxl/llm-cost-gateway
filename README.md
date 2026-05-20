# LLM Cost Gateway

SaaS multi-tenant LLM gateway and CostOps platform.

Current implementation status: phase 1 task 1 skeleton.

## Requirements

- Go 1.22+
- Node.js 22+
- npm 11+

## Backend

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

Verify migration tenant and value constraints:

```bash
make verify-migration
```

Rollback all local migrations:

```bash
make migrate-down
```

Run tests:

```bash
make test
```

Start the gateway with the example config:

```bash
make run
```

Health check:

```bash
curl http://localhost:8080/healthz
```

Expected response:

```json
{"status":"ok"}
```

## Console

Install dependencies:

```bash
make console-install
```

Build the placeholder management console:

```bash
make console-build
```

Start the Next.js development server:

```bash
cd web/console && npm run dev
```

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

## Local Services

Docker Compose starts:

- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- Gateway: `http://localhost:8080`
- Console: `http://localhost:3000`
- Prometheus: `http://localhost:9090`
