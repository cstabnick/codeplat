.PHONY: build test vet start stop

build:
	go build -o bin/codeplat ./cmd/cli
	go build -o bin/codeplat-rest ./cmd/rest
	go build -o bin/codeplat-mcp ./cmd/mcp
	go build -o bin/codeplat-web ./cmd/web
	go build -o bin/codeplat-runner ./cmd/runner
	go build -o bin/codeplat-nav ./cmd/nav

test:
	go test ./...

vet:
	go vet ./...

start:
	./start

stop:
	./stop
