package server

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	// logTimeFormat 使用带毫秒和时区偏移的 RFC 3339 格式，便于排序和跨时区排查。
	// logTimeFormat uses RFC 3339 with milliseconds and a timezone offset for sorting and cross-zone diagnostics.
	logTimeFormat = "2006-01-02 15:04:05.000 -07:00"
)

// appLogger adds the legacy Print methods required by the library logger interface.
type appLogger struct {
	*slog.Logger
	// base remains available to existing constructors that accept *slog.Logger.
	base *slog.Logger
}

func newAppLogger(base *slog.Logger) *appLogger {
	if base == nil {
		base = slog.Default()
	}
	return &appLogger{Logger: base, base: base}
}

func (l *appLogger) Print(v ...any) {
	l.Info(strings.TrimSuffix(fmt.Sprint(v...), "\n"))
}

func (l *appLogger) Printf(format string, v ...any) {
	l.Info(strings.TrimSuffix(fmt.Sprintf(format, v...), "\n"))
}

func (l *appLogger) Println(v ...any) {
	l.Info(strings.TrimSuffix(fmt.Sprintln(v...), "\n"))
}

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
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey && attr.Value.Kind() == slog.KindTime {
				return slog.String(slog.TimeKey, formatLogTime(attr.Value.Time()))
			}
			return attr
		},
	}
}

var logLocation = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return location
}()

func formatLogTime(t time.Time) string {
	return t.In(logLocation).Format(logTimeFormat)
}
