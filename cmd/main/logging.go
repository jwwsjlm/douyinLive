package main

import (
	"fmt"
	"log/slog"
	"strings"
)

// appLogger 包装 slog.Logger，并兼容 douyinLive 库需要的日志接口。
// appLogger wraps slog.Logger while satisfying the logger interface required by the douyinLive package.
type appLogger struct {
	base *slog.Logger
}

// newAppLogger 创建应用日志器，未传入时使用默认 slog。
// newAppLogger creates an application logger and falls back to slog.Default when nil.
func newAppLogger(base *slog.Logger) *appLogger {
	if base == nil {
		base = slog.Default()
	}
	return &appLogger{base: base}
}

// Print 以 info 级别输出兼容旧接口的日志。
// Print writes legacy-compatible output at info level.
func (l *appLogger) Print(v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprint(v...), "\n"))
}

// Printf 以 info 级别输出格式化日志。
// Printf writes formatted legacy-compatible output at info level.
func (l *appLogger) Printf(format string, v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprintf(format, v...), "\n"))
}

// Println 以 info 级别输出行日志。
// Println writes line-oriented legacy-compatible output at info level.
func (l *appLogger) Println(v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprintln(v...), "\n"))
}

// Debug 输出调试级别日志。
// Debug writes a debug-level log message.
func (l *appLogger) Debug(msg string, args ...interface{}) {
	l.base.Debug(msg, args...)
}

// Info 输出信息级别日志。
// Info writes an info-level log message.
func (l *appLogger) Info(msg string, args ...interface{}) {
	l.base.Info(msg, args...)
}

// Warn 输出警告级别日志。
// Warn writes a warning-level log message.
func (l *appLogger) Warn(msg string, args ...interface{}) {
	l.base.Warn(msg, args...)
}

// Error 输出错误级别日志。
// Error writes an error-level log message.
func (l *appLogger) Error(msg string, args ...interface{}) {
	l.base.Error(msg, args...)
}

// slogLevel 将配置字符串转换为 slog.Level。
// slogLevel converts a configured level string into slog.Level.
func slogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
