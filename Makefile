.PHONY: up down migrate-up migrate-down verify-migration test run console-install console-build

CONFIG_FILE ?= configs/config.example.yaml
COMPOSE ?= docker compose -f deploy/docker-compose.yml --project-directory .

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

test:
	go test ./cmd/... ./internal/...

run:
	CONFIG_FILE=$(CONFIG_FILE) go run ./cmd/gateway

console-install:
	cd web/console && npm install

console-build:
	cd web/console && npm run build
