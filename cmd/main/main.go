package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// 加载配置
	cfg, err := NewConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "加载配置失败:", err)
		fmt.Fprintln(os.Stderr, "解决方法: 在同目录下创建 config.yaml，或使用命令行参数 douyinLive --port 1088")
		os.Exit(1)
	}
	logger := newAppLogger(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel(cfg.Log.Level),
	})))

	// 创建应用实例
	logger.Info("版本信息", "version", VersionString())

	app, err := NewApp(context.Background(), cfg, logger)
	if err != nil {
		logger.Error("创建应用实例失败", "err", err)
		os.Exit(1)
	}

	// 启动应用
	go func() {
		if err := app.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("服务运行失败", "err", err)
			os.Exit(1)
		}
	}()
	<-app.ready
	logger.Info("WebSocket 服务启动成功", "addr", "ws://127.0.0.1:"+app.runningPort)

	// 等待终止信号，实现优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("接收到终止信号，开始优雅关闭")

	if err := app.Shutdown(); err != nil {
		logger.Error("服务关闭失败", "err", err)
		os.Exit(1)
	}
	logger.Info("服务已成功关闭")
}
