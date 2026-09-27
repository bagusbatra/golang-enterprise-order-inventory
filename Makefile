-include .env
export

MIGRATE_DSN=postgres://$(DATABASE_USER):$(DATABASE_PASSWORD)@localhost:$(DATABASE_PORT)/$(DATABASE_NAME)?sslmode=$(DATABASE_SSLMODE)

.PHONY: run build test test-race migrate-up migrate-down swagger docker-up docker-down lint

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...

test-race:
	go test -race ./...

migrate-up:
	migrate -path migrations -database "$(MIGRATE_DSN)" up

migrate-down:
	migrate -path migrations -database "$(MIGRATE_DSN)" down 1

swagger:
	swag init -g cmd/server/main.go -o docs/swagger

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

lint:
	golangci-lint run ./...
