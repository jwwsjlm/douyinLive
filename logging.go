package douyinLive

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/url"
	"strings"
)

// logger 定义兼容标准库 log.Logger 的最小日志接口。
// logger defines the minimal logging interface compatible with log.Logger.
type logger interface {
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	Println(v ...interface{})
}

// logSink 扩展基础日志接口，支持结构化日志级别。
// logSink extends the base logging interface with structured log levels.
type logSink interface {
	logger
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// printLogger 将传统 logger 适配为内部结构化日志接收器。
// printLogger adapts a legacy logger into the internal structured log sink.
type printLogger struct {
	base logger
}

// normalizeLogger 返回可用的日志接收器，必要时使用默认 logger。
// normalizeLogger returns a usable log sink and falls back to the default logger when needed.
// 参数/Parameters:
//   - base: 外部传入的旧版日志器。 Legacy logger provided by the caller.
func normalizeLogger(base logger) logSink {
	if base == nil {
		base = log.Default()
	}
	if sink, ok := base.(logSink); ok {
		return sink
	}
	return printLogger{base: base}
}

// Print 输出一条兼容旧接口的日志。
// Print writes a log line through the legacy-compatible interface.
// 参数/Parameters:
//   - v: 要输出的日志片段。 Log fragments to write.
func (l printLogger) Print(v ...interface{}) {
	l.base.Print(v...)
}

// Printf 按格式输出一条兼容旧接口的日志。
// Printf writes a formatted log line through the legacy-compatible interface.
// 参数/Parameters:
//   - format: 格式化模板。 Format string.
//   - v: 模板参数。 Format arguments.
func (l printLogger) Printf(format string, v ...interface{}) {
	l.base.Printf(format, v...)
}

// Println 输出一条带换行语义的兼容旧接口日志。
// Println writes a line-oriented log through the legacy-compatible interface.
// 参数/Parameters:
//   - v: 要输出的日志片段。 Log fragments to write.
func (l printLogger) Println(v ...interface{}) {
	l.base.Println(v...)
}

// Debug 输出调试级别日志。
// Debug writes a debug-level log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l printLogger) Debug(msg string, args ...interface{}) {
	l.base.Printf("[DEBUG] %s", formatLogMessage(msg, args...))
}

// Info 输出信息级别日志。
// Info writes an info-level log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l printLogger) Info(msg string, args ...interface{}) {
	l.base.Print(formatLogMessage(msg, args...))
}

// Warn 输出警告级别日志。
// Warn writes a warning-level log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l printLogger) Warn(msg string, args ...interface{}) {
	l.base.Printf("[WARN] %s", formatLogMessage(msg, args...))
}

// Error 输出错误级别日志。
// Error writes an error-level log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l printLogger) Error(msg string, args ...interface{}) {
	l.base.Printf("[ERROR] %s", formatLogMessage(msg, args...))
}

// formatLogMessage 将结构化键值参数拼接为传统日志文本。
// formatLogMessage flattens structured key-value arguments into legacy log text.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
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

func logFlowArgs(stage, step string, args ...interface{}) []interface{} {
	flowArgs := make([]interface{}, 0, 4+len(args))
	flowArgs = append(flowArgs, "stage", stage, "step", step)
	flowArgs = append(flowArgs, args...)
	return flowArgs
}

func websocketHostForLog(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return "<unknown>"
	}
	return parsed.Host
}

func classifyReadError(err error) string {
	if err == nil {
		return "none"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "i/o timeout"):
		return "timeout"
	case strings.Contains(text, "use of closed network connection"):
		return "closed_network_connection"
	default:
		return "network_or_unknown"
	}
}

// SlogLogger 将 slog.Logger 适配到 NewDouyinLive 接受的旧 logger 接口。
// SlogLogger adapts slog.Logger to the legacy logger interface accepted by NewDouyinLive while preserving structured levels.
type SlogLogger struct {
	base *slog.Logger
}

// NewSlogLogger 包装 slog.Logger，供 NewDouyinLive 使用。
// NewSlogLogger wraps a slog.Logger for use with NewDouyinLive.
// 参数/Parameters:
//   - base: 外部 slog.Logger；为 nil 时使用默认日志器。 External slog.Logger; nil uses the default logger.
func NewSlogLogger(base *slog.Logger) *SlogLogger {
	if base == nil {
		base = slog.Default()
	}
	return &SlogLogger{base: base}
}

// NewDouyinLiveWithSlog 创建使用 slog 输出日志的 DouyinLive 实例。
// NewDouyinLiveWithSlog creates a DouyinLive instance backed by slog.
// 参数/Parameters:
//   - liveID: 直播间短号、web_rid 或房间标识。 Live room short ID, web_rid, or room identifier.
//   - logger: slog 日志器；为 nil 时使用默认日志器。 slog logger; nil uses the default logger.
//   - cookie: 可选抖音 Cookie，用于登录态请求。 Optional Douyin Cookie for authenticated requests.
func NewDouyinLiveWithSlog(liveID string, logger *slog.Logger, cookie string) (*DouyinLive, error) {
	return newDouyinLive(liveID, NewSlogLogger(logger), cookie, newLocalWebsocketSigner())
}

// NewDouyinLiveWithSlogAndTikHub 创建使用 slog 和 TikHub 在线签名的 DouyinLive 实例。
// NewDouyinLiveWithSlogAndTikHub creates a DouyinLive instance backed by slog and TikHub online signing.
// 参数/Parameters:
//   - liveID: 直播间短号、web_rid 或房间标识。 Live room short ID, web_rid, or room identifier.
//   - logger: slog 日志器；为 nil 时使用默认日志器。 slog logger; nil uses the default logger.
//   - cookie: 可选抖音 Cookie，用于登录态请求。 Optional Douyin Cookie for authenticated requests.
//   - tikHubToken: TikHub API Token，用于在线生成 WebSocket 签名。 TikHub API token for online WebSocket signing.
func NewDouyinLiveWithSlogAndTikHub(liveID string, logger *slog.Logger, cookie string, tikHubToken string) (*DouyinLive, error) {
	return newDouyinLive(liveID, NewSlogLogger(logger), cookie, newTikHubWebsocketSigner(tikHubToken, ""))
}

// Print 通过 Info 级别输出兼容旧接口的日志。
// Print writes legacy-compatible output at info level.
// 参数/Parameters:
//   - v: 要输出的日志片段。 Log fragments to write.
func (l *SlogLogger) Print(v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprint(v...), "\n"))
}

// Printf 通过 Info 级别输出格式化日志。
// Printf writes formatted legacy-compatible output at info level.
// 参数/Parameters:
//   - format: 格式化模板。 Format string.
//   - v: 模板参数。 Format arguments.
func (l *SlogLogger) Printf(format string, v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprintf(format, v...), "\n"))
}

// Println 通过 Info 级别输出行日志。
// Println writes line-oriented legacy-compatible output at info level.
// 参数/Parameters:
//   - v: 要输出的日志片段。 Log fragments to write.
func (l *SlogLogger) Println(v ...interface{}) {
	l.Info(strings.TrimSuffix(fmt.Sprintln(v...), "\n"))
}

// Debug 输出调试级别结构化日志。
// Debug writes a debug-level structured log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l *SlogLogger) Debug(msg string, args ...interface{}) {
	l.base.Debug(msg, args...)
}

// Info 输出信息级别结构化日志。
// Info writes an info-level structured log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l *SlogLogger) Info(msg string, args ...interface{}) {
	l.base.Info(msg, args...)
}

// Warn 输出警告级别结构化日志。
// Warn writes a warning-level structured log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l *SlogLogger) Warn(msg string, args ...interface{}) {
	l.base.Warn(msg, args...)
}

// Error 输出错误级别结构化日志。
// Error writes an error-level structured log message.
// 参数/Parameters:
//   - msg: 日志消息。 Log message.
//   - args: 结构化键值参数。 Structured key-value arguments.
func (l *SlogLogger) Error(msg string, args ...interface{}) {
	l.base.Error(msg, args...)
}
