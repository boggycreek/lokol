.PHONY: all build test clean probe run release-snapshot

BINARY_NAME=lokol
BIN_DIR=bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.1.0-alpha")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS=-ldflags "-s -w \
	-X github.com/boggycreek/lokol/pkg/version.Version=$(VERSION) \
	-X github.com/boggycreek/lokol/pkg/version.GitCommit=$(COMMIT) \
	-X github.com/boggycreek/lokol/pkg/version.BuildDate=$(DATE)"

all: test build

build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/lokol

test:
	go test -v ./test/...

probe: build
	./$(BIN_DIR)/$(BINARY_NAME) probe

clean:
	rm -rf $(BIN_DIR) dist/

run: build
	./$(BIN_DIR)/$(BINARY_NAME)

