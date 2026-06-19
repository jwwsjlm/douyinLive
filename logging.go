package douyinLive

import (
	"fmt"
	"log"
	"log/slog"
	"strings"
)

type logger interface {
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	Println(v ...interface{})
}

type logSink interface {
	logger
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

type printLogger struct {
	base logger
}

func normalizeLogger(base logger) logSink {
	if base == nil {
		base = log.Default()
	}
	if sink, ok := base.(logSink); ok {
		return sink
	}
	return printLogger{base: base}
}

func (l printLogger) Print(v ...interface{}) {
	l.base.Print(v...)
}

func (l printLogger) Printf(format string, v ...interface{}) {
	l.base.Printf(format, v...)
}

func (l printLogger) Println(v ...interface{}) {
	l.base.Println(v...)
}

func (l printLogger) Debug(msg string, args ...interface{}) {
	l.base.Printf("[DEBUG] %s", formatLogMessage(msg, args...))
}

func (l printLogger) Info(msg string, args ...interface{}) {
	l.base.Print(formatLogMessage(msg, args...))
}

func (l printLogger) Warn(msg string, args ...interface{}) {
	l.base.Printf("[WARN] %s", formatLogMessage(msg, args...))
}

func (l printLogger) Error(msg string, args ...interface{}) {
	l.base.Printf("[ERROR] %s", formatLogMessage(msg, args...))
}

func formatLogMessage(msg string, args ...interface{}) string {
	if len(args) == 0 {
		return msg
	}

	parts := make([]string, 0, 1+len(args)/2)
	parts = append(parts, msg)
	for i := 0; i < len(args); i += 2 {
		key := fmt.Sprint(args[i])
		value := "<missing>"
		if i+1 < len(args) {
			value = fmt.Sprint(args[i+1])
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, " ")
}

// SlogLogger adapts slog.Logger to the legacy logger interface accepted by
// NewDouyinLive while preserving structured levels for new code paths.
type SlogLogger struct {
	base *slog.Logger
}

// NewSlogLogger wraps a slog.Logger for use with NewDouyinLive.
func NewSlogLogger(base *slog.Logger) *SlogLogger {
	if base == nil {
		base = slog.Default()
	}
	return &SlogLogger{base: base}
}

// NewDouyinLiveWithSlog creates a DouyinLive instance backed by slog.
func NewDouyinLiveWithSlog(liveID string, logger *slog.Logger, cookie string) (*DouyinLive, error) {
	return newDouyinLive(liveID, NewSlogLogger(logger), cookie, newLocalWebsocketSigner())
}

// NewDouyinLiveWithSlogAndTikHub creates a DouyinLive instance backed by slog
// and uses TikHub's online API to generate the WebSocket signature.
func NewDouyinLiveWithSlogAndTikHub(liveID string, logger *slog.Logger, cookie string, tikHubToken string) (*DouyinLive, error) {
	return newDouyinLive(liveID, NewSlogLogger(logger), cookie, newTikHubWebsocketSigner(tikHubToken, ""))
}

func (l *SlogLogger) Print(v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprint(v...), "\n"))
}

func (l *SlogLogger) Printf(format string, v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprintf(format, v...), "\n"))
}

func (l *SlogLogger) Println(v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprintln(v...), "\n"))
}

func (l *SlogLogger) Debug(msg string, args ...interface{}) {
	l.base.Debug(msg, args...)
}

func (l *SlogLogger) Info(msg string, args ...interface{}) {
	l.base.Info(msg, args...)
}

func (l *SlogLogger) Warn(msg string, args ...interface{}) {
	l.base.Warn(msg, args...)
}

func (l *SlogLogger) Error(msg string, args ...interface{}) {
	l.base.Error(msg, args...)
}
