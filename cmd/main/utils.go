package main

import (
	"fmt"
	"net"
)

// checkPortAvailability 检查本地端口是否可用
func checkPortAvailability(port int) bool {
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return true // 如果连接失败，认为端口可用
	}
	conn.Close()
	return false // 如果连接成功，认为端口不可用
}
