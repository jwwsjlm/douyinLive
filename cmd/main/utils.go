package main

import (
	"fmt"
	"net"
)

// isPortAvailable 检查指定 TCP 端口是否可用
func isPortAvailable(port int) bool {
	// 尝试监听该端口
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		// 如果监听失败，说明端口被占用
		return false
	}
	// 成功监听后立即关闭，释放端口
	_ = listener.Close()
	return true
}
