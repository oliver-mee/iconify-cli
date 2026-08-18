.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/iconify-pp-cli$(BIN_EXT) ./cmd/iconify-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/iconify-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/iconify-pp-mcp$(BIN_EXT) ./cmd/iconify-pp-mcp

install-mcp:
	go install ./cmd/iconify-pp-mcp

build-all: build build-mcp
