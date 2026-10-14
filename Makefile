.PHONY: build test vet

build:
	go build -o bin/codeplat ./cmd/cli
	go build -o bin/codeplat-rest ./cmd/rest
	go build -o bin/codeplat-mcp ./cmd/mcp
	go build -o bin/codeplat-web ./cmd/web

test:
	go test ./...

vet:
	go vet ./...
