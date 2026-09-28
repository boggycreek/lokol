.PHONY: all build test clean

BIN_DIR ?= ../../bin
TARGET = $(BIN_DIR)/core_driver

all: test build

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(TARGET) .

test:
	go test -v ./...

clean:
	rm -f $(TARGET)
