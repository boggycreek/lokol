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

# Enforce XDG Base Directory specification and prevent GOROOT pollution
unexport GOROOT
XDG_DATA_HOME ?= $(HOME)/.local/share
XDG_CACHE_HOME ?= $(HOME)/.cache
export GOPATH = $(XDG_DATA_HOME)/go
export GOCACHE = $(XDG_CACHE_HOME)/go-build
export GOBIN = $(HOME)/.local/bin

all: test build

build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/lokol
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BIN_DIR)/lokol-mcp ./cmd/lokol-mcp

test:
	go test -v ./test/...

probe: build
	./$(BIN_DIR)/$(BINARY_NAME) probe

clean:
	rm -rf $(BIN_DIR) dist/

sbom:
	@mkdir -p dist
	@if command -v syft >/dev/null 2>&1; then \
		syft scan dir:. -o spdx-json > dist/lokol-sbom.spdx.json; \
	elif [ -x /tmp/syft-bin/syft ]; then \
		/tmp/syft-bin/syft scan dir:. -o spdx-json > dist/lokol-sbom.spdx.json; \
	elif command -v cyclonedx-gomod >/dev/null 2>&1; then \
		cyclonedx-gomod app -output dist/lokol-sbom.spdx.json -json; \
	else \
		echo "Installing temporary syft binary..."; \
		curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /tmp/syft-bin v1.19.0; \
		/tmp/syft-bin/syft scan dir:. -o spdx-json > dist/lokol-sbom.spdx.json; \
	fi
	@sha256sum dist/lokol-sbom.spdx.json > dist/lokol-sbom.spdx.json.sha256
	@echo "Generated dist/lokol-sbom.spdx.json and dist/lokol-sbom.spdx.json.sha256"

run: build
	./$(BIN_DIR)/$(BINARY_NAME)

