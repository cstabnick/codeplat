.PHONY: test vet run smoke

test:
	go test ./...
	node --test test/*.test.js

vet:
	go vet ./...

run:
	go run ./cmd/farm

smoke:
	node test/smoke.cjs
