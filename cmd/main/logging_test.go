package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestFormatLogTimeIncludesMillisecondsAndOffset(t *testing.T) {
	timestamp := time.Date(2026, 7, 8, 1, 8, 6, 821000000, time.FixedZone("CST", 8*60*60))
	if got, want := formatLogTime(timestamp), "2026-07-08 01:08:06.821 +08:00"; got != want {
		t.Fatalf("formatted time = %q, want %q", got, want)
	}
}

func TestTextLogHandlerKeepsTimeLevelAndStructuredFields(t *testing.T) {
	var output bytes.Buffer
	handler := slog.NewTextHandler(&output, appLogHandlerOptions("info"))
	record := slog.NewRecord(
		time.Date(2026, 8, 17, 20, 37, 59, 428000000, time.FixedZone("CST", 8*60*60)),
		slog.LevelInfo,
		"DouyinLive 启动",
		0,
	)
	record.AddAttrs(slog.String("tag", "v2.2.1"), slog.String("time", "upstream"))
	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	for _, want := range []string{
		`time="2026-08-17 20:37:59.428 +08:00"`,
		`level=INFO`,
		`msg="DouyinLive 启动"`,
		`tag=v2.2.1`,
		`time=upstream`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("log output = %q, missing %q", output.String(), want)
		}
	}
}
