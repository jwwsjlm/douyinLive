package main

import (
	"fmt"
	"log/slog"
	"strings"
)

type appLogger struct {
	base *slog.Logger
}

func newAppLogger(base *slog.Logger) *appLogger {
	if base == nil {
		base = slog.Default()
	}
	return &appLogger{base: base}
}

func (l *appLogger) Print(v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprint(v...), "\n"))
}

func (l *appLogger) Printf(format string, v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprintf(format, v...), "\n"))
}

func (l *appLogger) Println(v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprintln(v...), "\n"))
}

func (l *appLogger) Debug(msg string, args ...interface{}) {
	l.base.Debug(msg, args...)
}

func (l *appLogger) Info(msg string, args ...interface{}) {
	l.base.Info(msg, args...)
}

func (l *appLogger) Warn(msg string, args ...interface{}) {
	l.base.Warn(msg, args...)
}

func (l *appLogger) Error(msg string, args ...interface{}) {
	l.base.Error(msg, args...)
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
