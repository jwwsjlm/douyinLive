package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// 初始化日志
	logger := log.New(os.Stdout, "DOUYIN-LIVE-WS :: ", log.LstdFlags)

	// 加载配置
	cfg, err := NewConfig()
	if err != nil {
		logger.Fatalf("加载配置失败: %v", err)
	}

	// 创建应用实例
	app, err := NewApp(context.Background(), cfg, logger)
	if err != nil {
		logger.Fatalf("创建应用实例失败: %v", err)
	}

	// 启动应用
	go func() {
		if err := app.Run(); err != nil {
			logger.Fatalf("服务运行失败: %v", err)
		}
	}()

	logger.Printf("WebSocket 服务启动成功, 地址为: ws://127.0.0.1:%s", app.runningPort)

	// 等待终止信号，实现优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Println("接收到终止信号，开始优雅关闭...")

	if err := app.Shutdown(); err != nil {
		logger.Fatalf("服务关闭失败: %v", err)
	}
	logger.Println("服务已成功关闭")
}
