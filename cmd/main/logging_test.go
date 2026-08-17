package main

import (
	"testing"
	"time"
)

func TestFormatLogTimeIncludesMillisecondsAndOffset(t *testing.T) {
	timestamp := time.Date(2026, 7, 8, 1, 8, 6, 821000000, time.FixedZone("CST", 8*60*60))
	if got, want := formatLogTime(timestamp), "2026-07-08 01:08:06.821 +08:00"; got != want {
		t.Fatalf("formatted time = %q, want %q", got, want)
	}
}
