.PHONY: test vet run live

test:
	go test ./...

vet:
	go vet ./...

run:
	go run ./cmd/llmchat

live:
	LLMCHAT_LIVE=1 go test -count=1 -v -timeout 20m ./internal/live/
