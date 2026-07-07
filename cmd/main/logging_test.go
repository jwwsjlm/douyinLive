package main

import (
	"log/slog"
	"testing"
	"time"
)

func TestAppLogHandlerOptionsFormatsTimeForHumans(t *testing.T) {
	options := appLogHandlerOptions("info")
	if options.ReplaceAttr == nil {
		t.Fatal("ReplaceAttr is nil")
	}

	timestamp := time.Date(2026, 7, 8, 1, 8, 6, 821000000, time.Local)
	attr := options.ReplaceAttr(nil, slog.Time(slog.TimeKey, timestamp))

	if attr.Key != slog.TimeKey {
		t.Fatalf("attr key = %q, want %q", attr.Key, slog.TimeKey)
	}
	if attr.Value.Kind() != slog.KindString {
		t.Fatalf("attr kind = %v, want %v", attr.Value.Kind(), slog.KindString)
	}
	if got, want := attr.Value.String(), "2026-07-08 01:08:06.821"; got != want {
		t.Fatalf("formatted time = %q, want %q", got, want)
	}
}
