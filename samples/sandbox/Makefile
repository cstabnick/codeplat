.PHONY: test vet run live

test:
	go test ./...

vet:
	go vet ./...

run:
	go run ./cmd/sandbox-server

live:
	SANDBOX_LIVE=1 go test -count=1 -v -timeout 10m ./internal/live/
