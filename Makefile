BINARY  ?= bin/haste
GO      ?= go
NPM     ?= npm
LDFLAGS ?= -s -w

.PHONY: help build web server test test-go test-web fmt vet run dev dev-web clean docker

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: web server ## Build the frontend, then the server binary that embeds it

web: ## Build the frontend into internal/webui/dist
	$(NPM) --prefix web ci
	$(NPM) --prefix web run build

server: ## Build the server binary (cgo required: zstd is a C library)
	CGO_ENABLED=1 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/haste

test: test-go test-web ## Run all tests

test-go: ## Run the Go test suite
	$(GO) test ./...

test-web: ## Typecheck the frontend and run its unit tests
	$(NPM) --prefix web run typecheck
	$(NPM) --prefix web run test

fmt: ## Format Go sources
	$(GO) fmt ./...

vet: ## Vet Go sources
	$(GO) vet ./...

run: build ## Build everything and start the server
	./$(BINARY)

dev: ## Run the API on :8080 with live reload disabled (pair with `make dev-web`)
	$(GO) run ./cmd/haste

dev-web: ## Run the Vite dev server, proxying /api and /raw to :8080
	$(NPM) --prefix web run dev

docker: ## Build the container image
	docker build -t haste-server:latest .

clean: ## Remove build output
	rm -rf $(BINARY) internal/webui/dist/assets internal/webui/dist/index.html \
		internal/webui/dist/favicon.svg
