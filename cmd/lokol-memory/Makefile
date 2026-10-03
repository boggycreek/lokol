.PHONY: all build test integration-test clean lint

BIN_DIR ?= ../../bin
BINARY ?= $(BIN_DIR)/lokol-memory

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.1.0-alpha")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS=-ldflags "-s -w \
	-X github.com/boggycreek/lokol/liblokol/version.Version=$(VERSION) \
	-X github.com/boggycreek/lokol/liblokol/version.GitCommit=$(COMMIT) \
	-X github.com/boggycreek/lokol/liblokol/version.BuildDate=$(DATE)"

all: test build

build:
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BINARY) .

test:
	go test -v ./...

integration-test:
	go test -v -tags integration ./...

clean:
	rm -rf $(BINARY) data/ tmp/ *.out *.test

lint:
	go vet ./...
