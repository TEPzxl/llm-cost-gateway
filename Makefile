.PHONY: up down migrate-up migrate-down verify-migration seed-demo smoke-test load-test sqlc-generate test run secret-rotate console-install console-build

CONFIG_FILE ?= configs/config.example.yaml
COMPOSE ?= docker compose -f deploy/docker-compose.yml --project-directory .
SQLC_VERSION ?= v1.31.1

up:
	$(COMPOSE) up -d --build

down:
	$(COMPOSE) down

migrate-up:
	./scripts/migrate-up.sh

migrate-down:
	./scripts/migrate-down.sh

verify-migration:
	./scripts/verify-migration-constraints.sh

seed-demo:
	./scripts/seed-demo.sh

smoke-test:
	./scripts/smoke-test.sh

load-test:
	./scripts/load-test.sh

sqlc-generate:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

test:
	go test ./cmd/... ./internal/...

run:
	CONFIG_FILE=$(CONFIG_FILE) go run ./cmd/gateway

secret-rotate:
	go run ./cmd/secret-rotate -config $(CONFIG_FILE) -dry-run

console-install:
	cd web/console && npm install

console-build:
	cd web/console && npm run build
