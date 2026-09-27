.PHONY: all build test integration-test clean lint

all: test build

build:
	go build ./...

test:
	go test -v ./...

integration-test:
	go test -v -tags integration ./...

clean:
	rm -rf data/ tmp/ *.out *.test

lint:
	go vet ./...
