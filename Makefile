BINARY_NAME := douyinlive
CMD_PATH := cmd/main/main.go

# Go commands
GO_BUILD := go build
GO_CLEAN := go clean
GO_TIDY := go mod tidy
LDFLAGS := -ldflags="-s -w"

# Build binary for Windows
build-windows:
	@echo "Building application for Windows..."
	$(GO_BUILD) $(LDFLAGS) -o $(BINARY_NAME)-windows.exe $(CMD_PATH)

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
