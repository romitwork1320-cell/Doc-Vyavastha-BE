.DEFAULT_GOAL := help
DATABASE_URL ?= postgres://edc:edc@localhost:5432/edconsultancy?sslmode=disable
PUBLIC_MIGRATIONS := migrations/public

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: tidy
tidy: ## go mod tidy
	go mod tidy

.PHONY: sqlc
sqlc: ## Regenerate sqlc query code
	sqlc generate

.PHONY: build
build: ## Build the API binary
	CGO_ENABLED=0 go build -o bin/api ./cmd/api

.PHONY: run
run: ## Run the API (expects DATABASE_URL + .env)
	go run ./cmd/api

.PHONY: test
test: ## Run tests
	go test ./...

.PHONY: vet
vet: ## go vet
	go vet ./...

.PHONY: migrate-up
migrate-up: ## Apply public-schema migrations via the migrate CLI
	migrate -path $(PUBLIC_MIGRATIONS) -database "pgx5://$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Roll back one public-schema migration
	migrate -path $(PUBLIC_MIGRATIONS) -database "pgx5://$(DATABASE_URL)" down 1


.PHONY: tools
tools: ## Install dev tools (sqlc, migrate)
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
