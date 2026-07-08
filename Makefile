.PHONY: build test bench lint clean

APP=screener
API=api
BIN_DIR=bin

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

lint:
	golangci-lint run ./...

vet:
	go vet ./...

run:
	$(BIN_DIR)/$(APP) serve --port 8080

clean:
	rm -rf $(BIN_DIR)
	rm -f sanctions.db
