.PHONY: build install test race clean lint fmt web web-install web-watch e2e-install e2e e2e-clean record-vibe-fixture

BINARY=adaf
BUILD_DIR=bin
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short=12 HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/agusx1211/adaf/internal/buildinfo.Version=$(VERSION) -X github.com/agusx1211/adaf/internal/buildinfo.CommitHash=$(COMMIT) -X github.com/agusx1211/adaf/internal/buildinfo.BuildDate=$(BUILD_DATE)"

web-install:
	cd web && npm install

web: web-install
	cd web && node esbuild.mjs --prod

web-watch:
	cd web && node esbuild.mjs --watch

build: web
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/adaf

install: web
	go install $(LDFLAGS) ./cmd/adaf

e2e-install:
	cd e2e && npm install

e2e: e2e-install
	make web
	cd e2e && npm run install:browsers && npm test

e2e-clean:
	cd e2e && rm -rf node_modules test-results playwright-report .state

test:
	go test ./...

race:
	go test -race ./...

clean:
	rm -rf $(BUILD_DIR)

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

all: tidy fmt build test race

record-vibe-fixture:
	ADAF_RECORD_VIBE_FIXTURE=1 go test -tags=integration ./internal/agent -run TestRecordVibeFixture -v
