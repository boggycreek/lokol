.PHONY: all build test clean lint

all: test build

build:
	go build ./...

test:
	go test -v ./...

clean:
	rm -rf data/ tmp/ *.out *.test

lint:
	go vet ./...
