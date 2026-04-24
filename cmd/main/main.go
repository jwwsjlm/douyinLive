package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	_ "net/http/pprof"
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
		logger.Printf("❌ 加载配置失败：%v", err)
		logger.Println("\n💡 解决方法:")
		logger.Println("1. 在同目录下创建 config.yaml 文件")
		logger.Println("   示例内容:")
		logger.Println("   port: 1088")
		logger.Println("   unknown: false")
		logger.Println("\n2. 或使用命令行参数:")
		logger.Println("   douyinLive.exe --port 1088")
		os.Exit(1)
	}

	if cfg.Pprof.Enabled {
		go func() {
			addr := ":" + cfg.Pprof.Port
			logger.Printf("pprof 调试服务启动成功，监听端口: %s", cfg.Pprof.Port)
			if err := http.ListenAndServe(addr, nil); err != nil {
				logger.Printf("pprof 服务异常退出: %v", err)
			}
		}()
	}

	// 创建应用实例
	app, err := NewApp(context.Background(), cfg, logger)
	if err != nil {
		logger.Fatalf("创建应用实例失败：%v", err)
	}

	// 启动应用
	go func() {
		if err := app.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("服务运行失败：%v", err)
		}
	}()
	<-app.ready
	logger.Printf("WebSocket 服务启动成功，地址为：ws://127.0.0.1:%v", app.runningPort)
	if cfg.Pprof.Enabled {
		logger.Printf("pprof 地址为：http://127.0.0.1:%s/debug/pprof/", cfg.Pprof.Port)
	}

	// 等待终止信号，实现优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Println("接收到终止信号，开始优雅关闭...")

	if err := app.Shutdown(); err != nil {
		logger.Fatalf("服务关闭失败：%v", err)
	}
	logger.Println("服务已成功关闭")
}
