VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE   ?= apisix-dashboard
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all web build run test vet audit docker clean

all: build

web:
	cd web && npm ci && npm run build

build: web
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/apisix-dashboard ./cmd/apisix-dashboard

# Roda o binário com as variáveis do .env. Ex.: make run ARGS="user add -u admin -r admin"
run:
	@test -f .env || { echo ".env não encontrado: cp .env.example .env"; exit 1; }
	set -a && . ./.env && set +a && ./bin/apisix-dashboard $(ARGS)

test:
	go test ./...

vet:
	go vet ./...

# Dependency vulnerability scan (PCI DSS 6.3).
audit:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	cd web && npm audit --omit=dev

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

clean:
	rm -rf bin internal/web/dist/assets internal/web/dist/index.html
