package main

import (
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPServerSetsDefensiveTimeouts(t *testing.T) {
	server := newHTTPServer(":0", http.NewServeMux())

	if server.ReadHeaderTimeout <= 0 {
		t.Fatalf("ReadHeaderTimeout = %s, want positive timeout", server.ReadHeaderTimeout)
	}
	if server.IdleTimeout <= 0 {
		t.Fatalf("IdleTimeout = %s, want positive timeout", server.IdleTimeout)
	}
	if server.MaxHeaderBytes < 8<<10 {
		t.Fatalf("MaxHeaderBytes = %d, want at least 8KiB", server.MaxHeaderBytes)
	}
	if server.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout = %s, want zero so upgraded WebSocket connections are not capped", server.ReadTimeout)
	}
	if server.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %s, want zero so upgraded WebSocket connections are not capped", server.WriteTimeout)
	}
	if server.IdleTimeout < time.Minute {
		t.Fatalf("IdleTimeout = %s, want at least 1m", server.IdleTimeout)
	}
}
