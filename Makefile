GOLANGCI_LINT ?= golangci-lint

.PHONY: test lint fmt tidy

test:
	go test ./...

lint:
	$(GOLANGCI_LINT) run ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy
