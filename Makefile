.PHONY: build test bench bench-full lint clean install-hooks

APP=screener
API=api
BIN_DIR=bin
FULL_DATASET=eu_sanctions.jsonl
FULL_DATASET_URL=https://data.opensanctions.org/datasets/latest/eu_fsf/entities.ftm.json

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS = -s -w \
	-X 'main.Version=$(VERSION)' \
	-X 'main.Commit=$(COMMIT)' \
	-X 'main.BuildDate=$(DATE)'

build:
	CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(APP) ./cmd/$(APP)
	CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(API) ./cmd/$(API)

test:
	go test -race ./...

bench:
	go test -bench=. ./pkg/screening

bench-full: build
	@echo "=== Downloading full EU dataset (5,885 entries) ==="
	@if [ ! -f $(FULL_DATASET) ]; then \
		curl -sS -o $(FULL_DATASET) '$(FULL_DATASET_URL)'; \
		echo "Downloaded $$(wc -l < $(FULL_DATASET)) lines"; \
	else \
		echo "Already downloaded ($$(wc -l < $(FULL_DATASET)) lines)"; \
	fi
	@echo ""
	@echo "=== Ingesting into temp DB ==="
	@go run ./cmd/screener --db /tmp/bench_full.db ingest --source jsonl --data $(FULL_DATASET)
	@echo ""
	@echo "=== Go benchmarks ==="
	@echo "--- Screening engine ---"
	@go test -bench=BenchmarkScreen -benchmem -benchtime=15x ./pkg/screening/
	@echo ""
	@echo "--- Sanctions parser ---"
	@go test -bench=. -benchmem -benchtime=10x ./pkg/sanctions/
	@echo ""
	@echo "--- Ingest & cache ---"
	@go test -bench=. -benchmem -benchtime=10x ./pkg/ingest/
	@echo ""
	@echo "=== Python comparison ==="
	@python3 scripts/py_screen.py $(FULL_DATASET) --jsonl
	@echo ""
	@echo "=== CLI warm-run timing (Go) ==="
	@for name in "Irina Kostenko" "Vitaly Kulikov" "Vladimir Putin" "Sberbank"; do \
		printf "$$name: "; \
		time ($(BIN_DIR)/$(APP) --db /tmp/bench_full.db screen --name "$$name" 2>/dev/null) 2>&1 | grep real || true; \
	done
	@rm -f /tmp/bench_full.db

lint:
	golangci-lint run ./...

vet:
	go vet ./...

run:
	$(BIN_DIR)/$(APP) serve --port 8080

clean:
	rm -rf $(BIN_DIR)
	rm -f sanctions.db /tmp/bench_full.db

install-hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/*
	@echo "Git hooks installed (core.hooksPath=.githooks)."
