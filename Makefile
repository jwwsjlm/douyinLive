BINARY_NAME := build/douyinLive
CMD_PATH := ./cmd/main
GO ?= go
GO_BUILD := $(GO) build
GO_CLEAN := $(GO) clean
GO_TIDY := $(GO) mod tidy
LDFLAGS := -s -w

.PHONY: all build build-linux build-windows build-darwin install clean proto help

all: build

build:
	@echo "Building application..."
	mkdir -p build
	$(GO_BUILD) -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) $(CMD_PATH)

build-linux:
	@echo "Building application for Linux..."
	mkdir -p build
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-linux-amd64 $(CMD_PATH)

build-windows:
	@echo "Building application for Windows..."
	mkdir -p build
	GOOS=windows GOARCH=amd64 $(GO_BUILD) -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-windows-amd64.exe $(CMD_PATH)

build-darwin:
	@echo "Building application for macOS..."
	mkdir -p build
	GOOS=darwin GOARCH=amd64 $(GO_BUILD) -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-darwin-amd64 $(CMD_PATH)

install:
	@echo "Tidying dependencies..."
	$(GO_TIDY)

proto:
	@echo "Generating Go code from .proto files..."
	protoc --proto_path=protobuf --go_out=. protobuf/douyin.proto

clean:
	@echo "Cleaning build artifacts..."
	$(GO_CLEAN)
	rm -rf build

help:
	@echo "Usage:"
	@echo "  make build         - Build application for current platform"
	@echo "  make build-linux   - Build application for Linux"
	@echo "  make build-windows - Build application for Windows"
	@echo "  make build-darwin  - Build application for macOS"
	@echo "  make install       - Tidy dependencies"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make proto         - Generate Go code from .proto files"
	@echo "  make help          - Display this help message"
