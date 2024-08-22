BINARY_NAME := douyinlive
# 编译为 Windows 系统的二进制文件
build-windows:
	@echo "Building for Windows..."
	go build -o $(BINARY_NAME)-windows.exe cmd/main/main.go
install:
	@echo "Installing dependencies..."
	go mod tidy
all: build-windows