.PHONY: all build test clean probe run

BINARY_NAME=lokol
BIN_DIR=bin

all: test build

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/lokol

test:
	go test -v ./test/...

probe: build
	./$(BIN_DIR)/$(BINARY_NAME) probe

clean:
	rm -rf $(BIN_DIR)

run: build
	./$(BIN_DIR)/$(BINARY_NAME)
