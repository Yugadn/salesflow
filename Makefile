COMPOSE := docker compose -f deploy/docker-compose.yml
SERVICES := master order warehouse billing

.PHONY: up down logs seed web-dev test test-integration cover e2e e2e-chaos lint sqlc

up: ## build and start the whole stack, waiting until every container is healthy
	$(COMPOSE) up -d --build --wait

down: ## stop the stack and delete its data
	$(COMPOSE) down -v

logs:
	$(COMPOSE) logs -f

seed: ## fill a fresh stack with demo data (needs make up)
	go run ./tools/seed

web-dev: ## frontend with hot reload on :5173 (backend from make up)
	cd web && npm install && npm run dev

test: ## unit tests
	go test -race ./...

test-integration: ## unit + integration tests (needs Docker for testcontainers)
	go test -race -tags integration ./...

cover:
	go test -tags integration -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

e2e: ## end-to-end tests against a running stack (make up first)
	go test -tags e2e -count=1 -v ./e2e

e2e-chaos: ## e2e plus tests that stop containers and replay events
	E2E_CHAOS=1 go test -tags e2e -count=1 -v ./e2e

lint:
	golangci-lint run ./...

sqlc: ## regenerate type-safe query code
	for s in $(SERVICES); do (cd services/$$s && sqlc generate) || exit 1; done
