BINARY_NAME := build/douyinlive
CMD_PATH := cmd/main/main.go

# Go commands
GO_BUILD := go build
GO_CLEAN := go clean
GO_TIDY := go mod tidy
LDFLAGS := -s -w
# OS-specific settings
GOOS_WINDOWS := windows
GOARCH_WINDOWS := amd64
GOOS_LINUX := linux
GOARCH_LINUX := amd64
#TAGS := jsoniter
# Build binary for Windows
build-windows:
	@echo "Building application for Windows..."
	$(GO_BUILD) -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-windows.exe $(CMD_PATH)
# Build binary for Linux
build-linux:
	@echo Building application for Linux...
	set GOOS=$(GOOS_LINUX)& set GOARCH=$(GOARCH_LINUX)& $(GO_BUILD) -tags=$(TAGS) -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-linux $(CMD_PATH)

# Install dependencies
install:
	@echo "Installing dependencies..."
	$(GO_TIDY)

# Generate Go code from .proto files
proto:
	@echo "Generating Go code from .proto files..."
	protoc --proto_path=protobuf --go_out=. protobuf/douyin.proto

# Clean generated files
clean:
	@echo "Cleaning build artifacts..."
	$(GO_CLEAN)
	del /F /Q $(BINARY_NAME)-windows.exe

# Default target
all: build-windows

# Help information
help:
	@echo "Usage:"
	@echo "  make build-windows - Build application for Windows"
	@echo "  make install       - Install dependencies"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make proto         - Generate Go code from .proto files"
	@echo "  make help          - Display this help message"

.PHONY: build-windows install clean proto help all