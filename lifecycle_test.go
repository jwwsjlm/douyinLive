package douyinLive

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitForReconnectDelayStopsWhenClosed(t *testing.T) {
	dl := &DouyinLive{closeCh: make(chan struct{})}
	dl.signalClose()

	start := time.Now()
	if dl.waitForReconnectDelay(time.Hour) {
		t.Fatalf("waitForReconnectDelay returned true after close signal")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("waitForReconnectDelay took %s after close signal", elapsed)
	}
}

func TestCloseAllowsZeroValueDouyinLive(t *testing.T) {
	var dl DouyinLive

	dl.Close()
	dl.Close()
}

func TestContextWithCloseSignalCancels(t *testing.T) {
	closeCh := make(chan struct{})
	ctx, cancel := contextWithCloseSignal(closeCh)
	defer cancel()

	close(closeCh)

	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("ctx.Err() = %v, want context.Canceled", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatalf("context was not canceled")
	}
}
