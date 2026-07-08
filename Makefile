.PHONY: build test bench lint clean

APP=screener
API=api
BIN_DIR=bin

build:
	CGO_ENABLED=1 go build -o $(BIN_DIR)/$(APP) ./cmd/$(APP)
	CGO_ENABLED=1 go build -o $(BIN_DIR)/$(API) ./cmd/$(API)

test:
	go test ./...

bench:
	go test -bench=. ./pkg/screening

lint:
	golangci-lint run ./...

run:
	$(BIN_DIR)/$(APP) serve --port 8080

clean:
	rm -rf $(BIN_DIR)
	rm -f sanctions.db
