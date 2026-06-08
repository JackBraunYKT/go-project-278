.PHONY: test lint build run generate sqlc migrate-up migrate-down migrate-status migrate-create

test:
	go mod tidy
	go test -v ./... --race

lint:
	golangci-lint run ./...

build:
	go build -o bin/go-project-278 ./main.go

run:
	./bin/go-project-278

generate:
	go tool sqlc generate

sqlc: generate

migrate-up:
	go tool goose -dir ./db/migrations postgres "$$DATABASE_URL" up

migrate-down:
	go tool goose -dir ./db/migrations postgres "$$DATABASE_URL" down

migrate-status:
	go tool goose -dir ./db/migrations postgres "$$DATABASE_URL" status

migrate-create:
	go tool goose -dir ./db/migrations create $(name) sql
