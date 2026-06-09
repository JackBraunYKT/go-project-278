.PHONY: test lint build run generate sqlc migrate-up migrate-down migrate-status migrate-create

POSTGRES_CONTAINER = go-project-278-postgres
TEST_POSTGRES_CONTAINER = go-project-278-postgres-test
POSTGRES_IMAGE = postgres:16
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/go-project-278?sslmode=disable

test:
	go mod tidy
	go test -v ./... --race

lint:
	go tool golangci-lint run ./...

build:
	go build -o bin/go-project-278 ./main.go

run:
	./bin/go-project-278

run-frontend:
	npx start-hexlet-url-shortener-frontend

generate:
	go tool sqlc generate

sqlc: generate

migrate-up:
	go tool goose -dir ./db/migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir ./db/migrations postgres "$(DATABASE_URL)" down

migrate-status:
	go tool goose -dir ./db/migrations postgres "$(DATABASE_URL)" status

migrate-create:
	go tool goose -dir ./db/migrations create $(name) sql
