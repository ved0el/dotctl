BINARY  := dotctl
VERSION ?= dev
# Target main.version (not a module-qualified path) so renaming the repo needs
# no change here.
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: help build test cover lint fmt clean

# Default target: list available commands (parsed from `## ` comments below).
.DEFAULT_GOAL := help

help: ## Show this help
	@echo "Usage: make <target>"
	@echo
	@grep -E '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*## "} {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Build the dotctl binary
	go build $(LDFLAGS) -o $(BINARY) ./cmd/dotctl

test: ## Run unit tests
	go test ./...

cover: ## Run tests with coverage
	go test -cover ./...

lint: ## Run linters (Go + shell)
	golangci-lint run
	shellcheck install.sh

fmt: ## Format Go sources
	gofmt -w .

clean: ## Remove build artifacts
	rm -f $(BINARY)
