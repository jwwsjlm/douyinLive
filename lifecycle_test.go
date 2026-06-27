package douyinLive

import (
	"context"
	"errors"
	"runtime"
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

func TestRequestContextDoesNotStartPerRequestGoroutine(t *testing.T) {
	dl := &DouyinLive{}
	const requests = 50

	before := runtime.NumGoroutine()
	cancels := make([]context.CancelFunc, 0, requests)
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
		dl.Close()
	}()

	for i := 0; i < requests; i++ {
		_, cancel := dl.requestContext()
		cancels = append(cancels, cancel)
	}
	time.Sleep(50 * time.Millisecond)

	after := runtime.NumGoroutine()
	if delta := after - before; delta > 10 {
		t.Fatalf("requestContext started too many goroutines: before=%d after=%d delta=%d", before, after, delta)
	}
}

func TestRequestContextAfterCloseIsCanceled(t *testing.T) {
	var dl DouyinLive
	dl.Close()

	ctx, cancel := dl.requestContext()
	defer cancel()

	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("ctx.Err() = %v, want context.Canceled", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatalf("context was not canceled after Close")
	}
}

func TestStartAfterCloseDoesNotResetCloseSignal(t *testing.T) {
	var dl DouyinLive
	dl.Close()

	err := dl.Start()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() after Close err = %v, want context.Canceled", err)
	}

	ctx, cancel := dl.requestContext()
	defer cancel()
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("ctx.Err() = %v, want context.Canceled", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatalf("request context was reopened after Start on a closed instance")
	}
}
