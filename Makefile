.PHONY: build test lint fmt tools

GOLANGCI_LINT := bin/golangci-lint

build:
	go build -o bin/ghtui ./cmd/ghtui

test:
	go test ./...

# golangci-lint must be built with the same Go toolchain as the code.
$(GOLANGCI_LINT):
	GOBIN=$(CURDIR)/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

tools: $(GOLANGCI_LINT)

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

fmt:
	gofmt -w .
