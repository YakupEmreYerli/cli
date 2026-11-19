BIN      := bin/dna
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w -X main.version=$(VERSION:v%=%) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build install test lint docs check spec clean

build: ## Build bin/dna
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/dna

install: ## Install dna into GOBIN
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/dna

test: ## Run all tests
	go test ./...

lint: ## go vet + gofmt check
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)

docs: ## Regenerate docs/reference, docs/enums.md and skills/dna/SKILL.md
	go run ./cmd/dna gen-docs --dir .

check: docs lint test ## Everything CI runs; fails if generated files are stale
	@git diff --exit-code -- docs skills || (echo "generated files are out of date: commit the changes above"; exit 1)

spec: ## Download the latest OpenAPI document from production and OTE
	sh scripts/update-spec.sh

clean:
	rm -rf bin dist
