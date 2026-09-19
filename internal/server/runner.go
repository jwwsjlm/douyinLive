package server

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

// Run executes the service lifecycle and returns the process exit code.
func Run(buildInfo BuildInfo) int {
	cfg, err := newConfig(buildInfo.DefaultSignProvider)
	if err != nil {
		if errors.Is(err, ErrVersionRequested) {
			fmt.Println(buildInfo.VersionString())
			return 0
		}
		fmt.Fprintln(os.Stderr, "加载配置失败:", err)
		fmt.Fprintln(os.Stderr, "解决方法: 在同目录下创建 config.yaml，或使用命令行参数 douyinLive --port 1088")
		return 1
	}
	logger := newAppLogger(slog.New(slog.NewTextHandler(os.Stdout, appLogHandlerOptions(cfg.Log.Level))))
	logger.Info(
		"DouyinLive 启动",
		"stage", "startup",
		"step", "version",
		"tag", buildInfo.Tag,
		"commit", buildInfo.Commit,
		"build_date", buildInfo.Date,
		"build_source", buildInfo.Source,
		"sign_provider", cfg.Sign.Provider,
	)

	runCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	app, err := newApp(runCtx, cfg, logger, buildInfo)
	if err != nil {
		logger.Error("创建应用实例失败", "stage", "startup", "step", "create_app", "err", err)
		return 1
	}

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
