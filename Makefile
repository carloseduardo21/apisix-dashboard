# Lintel for Apache APISIX — developer tasks.
# Run `make help` for the list of targets.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE   ?= apisix-dashboard
LDFLAGS := -s -w -X main.version=$(VERSION)
GO_PKGS := ./...

# Pinned tool versions (keep in sync with CI).
GOLANGCI_LINT_VERSION ?= v2.15.0
GOVULNCHECK_VERSION   ?= v1.9.0
GITLEAKS_VERSION      ?= v8.30.1
GORELEASER_VERSION    ?= v2.19.0

# Local toolchain installed under ~/.local (see README / CLAUDE.md).
export PATH := $(HOME)/.local/go/bin:$(HOME)/go/bin:$(HOME)/.local/bin:$(PATH)

NPM := npm --prefix web

.PHONY: help tools fmt fmt-check lint lint-fast test test-race cover vuln secrets \
        check web build run dev docker-build docker release-check release-snapshot clean

help: ## Show this help
	@awk 'BEGIN{FS=":.*##"; printf "Targets:\n"} /^[a-zA-Z_-]+:.*##/{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools: ## Install pinned Go tools (govulncheck); check golangci-lint/gitleaks
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	@golangci-lint version 2>/dev/null | grep -q "$(GOLANGCI_LINT_VERSION:v%=%)" || \
	  echo "WARN: install golangci-lint $(GOLANGCI_LINT_VERSION) (https://golangci-lint.run/docs/welcome/install/)"
	@gitleaks version 2>/dev/null | grep -q "$(GITLEAKS_VERSION:v%=%)" || \
	  echo "WARN: install gitleaks $(GITLEAKS_VERSION) (https://github.com/gitleaks/gitleaks/releases)"
	@goreleaser --version 2>/dev/null | grep -q "$(GORELEASER_VERSION:v%=%)" || \
	  echo "WARN: install goreleaser $(GORELEASER_VERSION) for release targets (https://github.com/goreleaser/goreleaser/releases)"
	@test -d web/node_modules || $(NPM) ci

## ---- quality --------------------------------------------------------------

fmt: ## Format Go (gofmt+goimports) and frontend (prettier)
	golangci-lint fmt $(GO_PKGS)
	$(NPM) run format

fmt-check: ## Fail if anything is not formatted
	golangci-lint fmt --diff $(GO_PKGS)
	$(NPM) run format:check

lint: ## Full lint: golangci-lint (gosec, errcheck, govet, staticcheck, revive), oxlint type-aware, prettier, tsc
	golangci-lint run $(GO_PKGS)
	$(NPM) run typecheck
	$(NPM) run lint
	$(NPM) run format:check

lint-fast: ## Fast lint (used while editing)
	golangci-lint run --fast-only $(GO_PKGS)
	$(NPM) run lint:fast

test: ## Run Go tests
	go test $(GO_PKGS)

test-race: ## Run Go tests with the race detector (needs a C compiler)
	CGO_ENABLED=1 go test -race $(GO_PKGS)

cover: ## Go test coverage report (coverage.out)
	go test -covermode=atomic -coverprofile=coverage.out $(GO_PKGS)
	go tool cover -func=coverage.out | tail -1

vuln: ## Dependency vulnerability scan: govulncheck + npm audit (high+)
	govulncheck $(GO_PKGS)
	$(NPM) run audit

secrets: ## Secret scan: full git history + uncommitted/staged changes (redacted; .gitignore'd files are skipped)
	gitleaks git --log-opts="--all" --redact --no-banner .
	gitleaks git --pre-commit --redact --no-banner .
	gitleaks git --pre-commit --staged --redact --no-banner .

check: fmt-check lint test vuln secrets ## Everything CI runs (Definition of Done)

## ---- build & run ----------------------------------------------------------

web: ## Build the frontend into internal/web/dist
	$(NPM) ci
	$(NPM) run build

build: web ## Build the single binary into bin/
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/apisix-dashboard ./cmd/apisix-dashboard

# Run the binary with the variables from .env. Ex.: make run ARGS="user add -u admin -r admin"
run: ## Run bin/apisix-dashboard with .env (ARGS=... for CLI subcommands)
	@test -f .env || { echo ".env not found: cp .env.example .env"; exit 1; }
	set -a && . ./.env && set +a && ./bin/apisix-dashboard $(ARGS)

dev: ## Backend (go run, :8080) + Vite dev server (:5173, proxies /api)
	@test -f .env || { echo ".env not found: cp .env.example .env"; exit 1; }
	@test -d web/node_modules || $(NPM) ci
	@trap 'kill 0' INT TERM EXIT; \
	  (set -a && . ./.env && set +a && go run ./cmd/apisix-dashboard serve) & \
	  $(NPM) run dev & \
	  wait

docker-build: ## Build the container image ($(IMAGE):$(VERSION))
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

docker: docker-build ## Alias of docker-build

release-check: ## Validate the GoReleaser configuration
	goreleaser check

release-snapshot: web ## Local release dry run into dist/ (no publish, no signing/SBOM)
	goreleaser release --snapshot --clean --skip=sign,sbom

clean: ## Remove build output
	rm -rf bin dist coverage.out internal/web/dist/assets internal/web/dist/index.html
