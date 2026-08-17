package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// prettyTextHandler 以人类可读的前缀输出日志，同时保留 key=value 结构化字段。
// prettyTextHandler renders a human-readable prefix while preserving key=value structured attributes.
type prettyTextHandler struct {
	writer  io.Writer
	options slog.HandlerOptions
	steps   []prettyHandlerStep
	mu      *sync.Mutex
}

// prettyHandlerStep 按调用顺序保存 WithAttrs 和 WithGroup，确保分组语义与 slog 一致。
// prettyHandlerStep preserves WithAttrs and WithGroup call order so grouping matches slog semantics.
type prettyHandlerStep struct {
	attrs []slog.Attr
	group string
}

// newPrettyTextHandler 创建适用于终端、Docker 和日志文件的单行文本处理器。
// newPrettyTextHandler creates a single-line text handler for terminals, Docker, and log files.
func newPrettyTextHandler(writer io.Writer, options *slog.HandlerOptions) slog.Handler {
	resolved := slog.HandlerOptions{}
	if options != nil {
		resolved = *options
	}
	return &prettyTextHandler{
		writer:  writer,
		options: resolved,
		mu:      &sync.Mutex{},
	}
}

// Enabled 判断指定日志级别是否需要输出。
// Enabled reports whether a record at the supplied level should be emitted.
func (h *prettyTextHandler) Enabled(_ context.Context, level slog.Level) bool {
	minimum := slog.LevelInfo
	if h.options.Level != nil {
		minimum = h.options.Level.Level()
	}
	return level >= minimum
}

// Handle 将一条 slog 记录格式化为单行可读日志。
// Handle formats one slog record as a human-readable single-line log entry.
func (h *prettyTextHandler) Handle(ctx context.Context, record slog.Record) error {
	var line bytes.Buffer
	timestamp := record.Time
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	_, _ = fmt.Fprintf(
		&line,
		"%s  %-5s  %s",
		formatLogTime(timestamp),
		strings.ToUpper(record.Level.String()),
		singleLineLogMessage(record.Message),
	)

	if attributes := h.formatAttributes(ctx, record); attributes != "" {
		line.WriteString("  ")
		line.WriteString(attributes)
	}
	line.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	written, err := h.writer.Write(line.Bytes())
	if err == nil && written != line.Len() {
		return io.ErrShortWrite
	}
	return err
}

// WithAttrs 返回包含固定结构化字段的新处理器。
// WithAttrs returns a new handler carrying the supplied fixed attributes.
func (h *prettyTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	clone.steps = append(clone.steps, prettyHandlerStep{attrs: append([]slog.Attr(nil), attrs...)})
	return clone
}

// WithGroup 返回将后续字段放入指定分组的新处理器。
// WithGroup returns a new handler that places subsequent attributes in the named group.
func (h *prettyTextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := h.clone()
	clone.steps = append(clone.steps, prettyHandlerStep{group: name})
	return clone
}

// clone 复制处理器状态，并共享写锁以防并发日志交错。
// clone copies handler state while sharing the write lock to prevent interleaved records.
func (h *prettyTextHandler) clone() *prettyTextHandler {
	clone := *h
	clone.steps = append([]prettyHandlerStep(nil), h.steps...)
	return &clone
}

// formatAttributes 复用标准 TextHandler 编码结构化字段，避免自定义转义规则产生歧义。
// formatAttributes reuses TextHandler attribute encoding to avoid ambiguous custom escaping rules.
func (h *prettyTextHandler) formatAttributes(ctx context.Context, record slog.Record) string {
	var output bytes.Buffer
	options := h.options
	configuredReplace := options.ReplaceAttr
	options.AddSource = false
	options.ReplaceAttr = func(groups []string, attr slog.Attr) slog.Attr {
		switch attr.Key {
		case slog.TimeKey, slog.LevelKey, slog.MessageKey, slog.SourceKey:
			return slog.Attr{}
		}
		if configuredReplace != nil {
			return configuredReplace(groups, attr)
		}
		return attr
	}

	var handler slog.Handler = slog.NewTextHandler(&output, &options)
	for _, step := range h.steps {
		if step.group != "" {
			handler = handler.WithGroup(step.group)
			continue
		}
		if len(step.attrs) > 0 {
			handler = handler.WithAttrs(step.attrs)
		}
	}
	_ = handler.Handle(ctx, record)
	return strings.TrimSpace(output.String())
}

// singleLineLogMessage 转义换行和制表符，保证每条日志只占一行。
// singleLineLogMessage escapes line breaks and tabs so every record stays on one line.
func singleLineLogMessage(message string) string {
	replacer := strings.NewReplacer("\r", `\r`, "\n", `\n`, "\t", `\t`)
	return replacer.Replace(message)
}
