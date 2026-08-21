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

// main 加载配置、启动 WebSocket 服务，并在收到退出信号时优雅关闭。
// main loads configuration, starts the WebSocket service, and gracefully shuts down on exit signals.
func main() {
	os.Exit(run())
}

// run executes the service lifecycle and returns the process exit code.
// run 执行完整服务生命周期并返回进程退出码，确保所有错误路径都先完成资源清理。
func run() int {
	// 加载配置。
	// Load configuration.
	cfg, err := NewConfig()
	if err != nil {
		if errors.Is(err, ErrVersionRequested) {
			fmt.Println(VersionString())
			return 0
		}
		fmt.Fprintln(os.Stderr, "加载配置失败:", err)
		fmt.Fprintln(os.Stderr, "解决方法: 在同目录下创建 config.yaml，或使用命令行参数 douyinLive --port 1088")
		return 1
	}
	logger := newAppLogger(slog.New(newPrettyTextHandler(os.Stdout, appLogHandlerOptions(cfg.Log.Level))))

	// 创建应用实例。
	// Create the application instance.
	logger.Info(
		"DouyinLive 启动",
		"stage", "startup",
		"step", "version",
		"tag", buildTag,
		"commit", buildCommit,
		"build_date", buildDate,
		"build_source", buildSource,
		"sign_provider", cfg.Sign.Provider,
	)

	runCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	app, err := NewApp(runCtx, cfg, logger)
	if err != nil {
		logger.Error("创建应用实例失败", "stage", "startup", "step", "create_app", "err", err)
		return 1
	}

	// 启动应用。
	// Start the application.
	runErrCh := make(chan error, 1)
	go func() { runErrCh <- app.Run() }()
	select {
	case <-app.ready:
	case err := <-runErrCh:
		logger.Error("服务启动失败", "stage", "startup", "step", "run_server", "err", err)
		if shutdownErr := app.Shutdown(); shutdownErr != nil {
			logger.Error("服务启动失败后的资源清理失败", "stage", "shutdown", "step", "close_server", "err", shutdownErr)
		}
		return 1
	}
	logger.Info("WebSocket 服务启动成功", "stage", "startup", "step", "listen", "addr", "ws://127.0.0.1:"+app.runningPort)

	select {
	case <-runCtx.Done():
		logger.Info("接收到终止信号，开始优雅关闭", "stage", "shutdown", "step", "signal")
	case err := <-runErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("服务运行失败，开始清理资源", "stage", "shutdown", "step", "run_server", "err", err)
			if shutdownErr := app.Shutdown(); shutdownErr != nil {
				logger.Error("服务异常退出后的资源清理失败", "stage", "shutdown", "step", "close_server", "err", shutdownErr)
			}
			return 1
		}
		return 0
	}

	if err := app.Shutdown(); err != nil {
		logger.Error("服务关闭失败", "stage", "shutdown", "step", "close_server", "err", err)
		return 1
	}
	logger.Info("服务已成功关闭", "stage", "shutdown", "step", "done")
	return 0
}
