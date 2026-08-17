package main

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestPrettyTextHandlerFormatsReadableStructuredLine(t *testing.T) {
	var output bytes.Buffer
	handler := newPrettyTextHandler(&output, appLogHandlerOptions("info"))
	record := slog.NewRecord(
		time.Date(2026, 8, 17, 20, 37, 59, 428000000, time.FixedZone("CST", 8*60*60)),
		slog.LevelInfo,
		"DouyinLive 启动",
		0,
	)
	record.AddAttrs(
		slog.String("tag", "v2.0.26-beta.4"),
		slog.String("commit", "abcdef12"),
		slog.String("sign_provider", "local"),
	)

	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	want := "2026-08-17 20:37:59.428 +08:00  INFO   DouyinLive 启动  tag=v2.0.26-beta.4 commit=abcdef12 sign_provider=local\n"
	if got := output.String(); got != want {
		t.Fatalf("log output = %q, want %q", got, want)
	}
}

func TestPrettyTextHandlerRespectsLevelAndKeepsMessagesOnOneLine(t *testing.T) {
	var output bytes.Buffer
	handler := newPrettyTextHandler(&output, appLogHandlerOptions("warn"))
	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info level unexpectedly enabled")
	}
	if !handler.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("warn level unexpectedly disabled")
	}

	record := slog.NewRecord(
		time.Date(2026, 8, 17, 20, 37, 59, 0, time.FixedZone("CST", 8*60*60)),
		slog.LevelWarn,
		"第一行\n第二行",
		0,
	)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	want := "2026-08-17 20:37:59.000 +08:00  WARN   第一行\\n第二行\n"
	if got := output.String(); got != want {
		t.Fatalf("log output = %q, want %q", got, want)
	}
}

func TestPrettyTextHandlerPreservesWithAttrsAndWithGroupOrder(t *testing.T) {
	var output bytes.Buffer
	handler := newPrettyTextHandler(&output, appLogHandlerOptions("info"))
	handler = handler.WithAttrs([]slog.Attr{slog.String("service", "douyinlive")})
	handler = handler.WithGroup("room")
	handler = handler.WithAttrs([]slog.Attr{slog.String("id", "123456")})

	record := slog.NewRecord(
		time.Date(2026, 8, 17, 20, 37, 59, 0, time.FixedZone("CST", 8*60*60)),
		slog.LevelInfo,
		"开始监听",
		0,
	)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	want := "2026-08-17 20:37:59.000 +08:00  INFO   开始监听  service=douyinlive room.id=123456\n"
	if got := output.String(); got != want {
		t.Fatalf("log output = %q, want %q", got, want)
	}
}
