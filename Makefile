test:
	go mod tidy
	go test -v ./... --race

lint:
	golangci-lint run ./...

build:
	go build -o bin/go-project-278 ./main.go

run:
	./bin/go-project-278