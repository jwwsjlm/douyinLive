package main

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// appLogger 包装 slog.Logger，并兼容 douyinLive 库需要的日志接口。
// appLogger wraps slog.Logger while satisfying the logger interface required by the douyinLive package.
type appLogger struct {
	base *slog.Logger
}

// newAppLogger 创建应用日志器，未传入时使用默认 slog。
// newAppLogger creates an application logger and falls back to slog.Default when nil.
// 参数/Parameters:
//   - base: 外部 slog.Logger；为 nil 时使用默认日志器。 External slog.Logger; nil uses the default logger.
func newAppLogger(base *slog.Logger) *appLogger {
	if base == nil {
		base = slog.Default()
	}
	return &appLogger{base: base}
}

// Print 以 info 级别输出兼容旧接口的日志。
// Print writes legacy-compatible output at info level.
// 参数/Parameters:
//   - v: 要输出的日志片段。 Log fragments to write.
func (l *appLogger) Print(v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprint(v...), "\n"))
}

// Printf 以 info 级别输出格式化日志。
// Printf writes formatted legacy-compatible output at info level.
// 参数/Parameters:
//   - format: 格式化模板。 Format string.
//   - v: 模板参数。 Format arguments.
func (l *appLogger) Printf(format string, v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprintf(format, v...), "\n"))
}

// Println 以 info 级别输出行日志。
// Println writes line-oriented legacy-compatible output at info level.
// 参数/Parameters:
//   - v: 要输出的日志片段。 Log fragments to write.
func (l *appLogger) Println(v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprintln(v...), "\n"))
}

// Debug 输出调试级别日志。
// Debug writes a debug-level log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l *appLogger) Debug(msg string, args ...interface{}) {
	l.base.Debug(msg, args...)
}

// Info 输出信息级别日志。
// Info writes an info-level log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l *appLogger) Info(msg string, args ...interface{}) {
	l.base.Info(msg, args...)
}

// Warn 输出警告级别日志。
// Warn writes a warning-level log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l *appLogger) Warn(msg string, args ...interface{}) {
	l.base.Warn(msg, args...)
}

// Error 输出错误级别日志。
// Error writes an error-level log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l *appLogger) Error(msg string, args ...interface{}) {
	l.base.Error(msg, args...)
}

// slogLevel 将配置字符串转换为 slog.Level。
// slogLevel converts a configured level string into slog.Level.
// 参数/Parameters:
//   - level: 日志级别字符串。 Log level string.
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

func appLogHandlerOptions(level string) *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level: slogLevel(level),
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey && attr.Value.Kind() == slog.KindTime {
				attr.Value = slog.StringValue(formatLogTime(attr.Value.Time()))
			}
			return attr
		},
	}
}

func formatLogTime(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04:05.000")
}
