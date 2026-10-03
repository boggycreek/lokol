.PHONY: all build test integration-test clean lint sbom probe run test-probe test-core-driver

SUBPROJECTS = liblokol cmd/lk cmd/lokol cmd/lokol-mcp cmd/lokol-memory
BIN_DIR = bin

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
	@for p in $(SUBPROJECTS); do \
		echo "==> Building $$p"; \
		$(MAKE) -C $$p build BIN_DIR=$(CURDIR)/$(BIN_DIR) || exit 1; \
	done

test:
	@for p in $(SUBPROJECTS); do \
		echo "==> Testing $$p"; \
		$(MAKE) -C $$p test || exit 1; \
	done

test-probe:
	uv run --python .venv python -m unittest tools/probe/test_harness.py -v

test-core-driver:
	$(MAKE) -C tools/core_driver test

integration-test:
	@for p in $(SUBPROJECTS); do \
		echo "==> Integration Testing $$p"; \
		$(MAKE) -C $$p integration-test || exit 1; \
	done

clean:
	@for p in $(SUBPROJECTS); do \
		$(MAKE) -C $$p clean BIN_DIR=$(CURDIR)/$(BIN_DIR) || true; \
	done
	rm -rf $(BIN_DIR) dist/

lint:
	@for p in $(SUBPROJECTS); do \
		echo "==> Linting $$p"; \
		$(MAKE) -C $$p lint || exit 1; \
	done

probe: build
	./$(BIN_DIR)/lokol probe

run: build
	./$(BIN_DIR)/lk

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
