SHELL := /bin/bash
GO ?= go
BIN ?= content-grid-d
PKG ?= ./cmd/content-grid-d
GOCACHE ?= $(PWD)/.gocache

.PHONY: build test lint format proto proto-format

build:
	GOCACHE=$(GOCACHE) $(GO) build -o $(BIN) $(PKG)

test:
	GOCACHE=$(GOCACHE) $(GO) test ./...


lint:
	GOCACHE=$(GOCACHE) $(GO) vet ./...
	@files=$$(gofmt -s -l .); \
	 if [ -n "$$files" ]; then \
	   echo "gofmt needed for:" $$files; \
	   echo "Run 'make format' to apply fixes"; \
	   exit 1; \
	 fi

format:
	gofmt -s -w .
	$(MAKE) proto-format

# Proto helpers
# NOTE: Use buf for protobuf files; gofmt only works for Go.
proto-format:
	cd proto && $(GO) run github.com/bufbuild/buf/cmd/buf@v1.34.0 format -w

proto:
	cd proto && $(GO) run github.com/bufbuild/buf/cmd/buf@v1.34.0 generate
