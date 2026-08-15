package douyinLive

import (
	"testing"
)

func TestReadyClosesOnceAfterUpstreamHandshake(t *testing.T) {
	dl := &DouyinLive{readyCh: make(chan struct{})}
	ready := dl.Ready()

	select {
	case <-ready:
		t.Fatal("Ready channel is closed before the handshake")
	default:
	}

	dl.markReady()
	select {
	case <-ready:
	default:
		t.Fatal("Ready channel was not closed after the handshake")
	}

	dl.markReady()
	select {
	case <-ready:
	default:
		t.Fatal("Ready channel changed after a duplicate mark")
	}
}
