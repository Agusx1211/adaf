.PHONY: build install test clean lint fmt

BINARY=adaf
BUILD_DIR=bin
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X github.com/agusx1211/adaf/internal/cli.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/adaf

install:
	go install $(LDFLAGS) ./cmd/adaf

test:
	go test ./...

clean:
	rm -rf $(BUILD_DIR)

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

all: tidy fmt build test
