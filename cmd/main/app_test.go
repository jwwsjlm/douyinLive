package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type testListener struct {
	net.Listener
}

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

func TestParseConfiguredPort(t *testing.T) {
	tests := []struct {
		value string
		want  int
		ok    bool
	}{
		{value: "1088", want: 1088, ok: true},
		{value: " 65535 ", want: 65535, ok: true},
		{value: "0"},
		{value: "65536"},
		{value: "not-a-port"},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := parseConfiguredPort(tt.value)
			if tt.ok {
				if err != nil || got != tt.want {
					t.Fatalf("parseConfiguredPort(%q) = %d, %v; want %d, nil", tt.value, got, err, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("parseConfiguredPort(%q) unexpectedly succeeded", tt.value)
			}
		})
	}
}

func TestListenOnAvailablePortOnlyRetriesAddressInUse(t *testing.T) {
	var attempts []int
	listener, selected, err := listenOnAvailablePort(1088, func(_ string, address string) (net.Listener, error) {
		port, parseErr := strconv.Atoi(address[1:])
		if parseErr != nil {
			t.Fatalf("parse address %q: %v", address, parseErr)
		}
		attempts = append(attempts, port)
		if port == 1088 {
			return nil, syscall.EADDRINUSE
		}
		return &testListener{}, nil
	})
	if err != nil {
		t.Fatalf("listenOnAvailablePort() error = %v", err)
	}
	if listener == nil || selected != 1089 {
		t.Fatalf("listenOnAvailablePort() = (%v, %d), want non-nil listener and 1089", listener, selected)
	}
	if len(attempts) != 2 {
		t.Fatalf("attempts = %v, want [1088 1089]", attempts)
	}
}

func TestListenOnAvailablePortReturnsNonAddressErrorImmediately(t *testing.T) {
	wantErr := errors.New("permission denied")
	attempts := 0
	_, _, err := listenOnAvailablePort(1088, func(_, _ string) (net.Listener, error) {
		attempts++
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("listenOnAvailablePort() error = %v, want wrapped %v", err, wantErr)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestListenOnAvailablePortStopsAtMaximumPort(t *testing.T) {
	attempts := 0
	_, _, err := listenOnAvailablePort(65535, func(_, _ string) (net.Listener, error) {
		attempts++
		return nil, syscall.EADDRINUSE
	})
	if err == nil {
		t.Fatal("listenOnAvailablePort() unexpectedly succeeded")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestHealthEndpoint(t *testing.T) {
	app, err := NewApp(context.Background(), &Config{}, nil)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	recorder := httptest.NewRecorder()
	app.handleHealth(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if body := recorder.Body.String(); body == "" || !containsAll(body, `"status":"ok"`, `"version":`) {
		t.Fatalf("health response = %q", body)
	}
}

func TestNewAppRejectsNilConfig(t *testing.T) {
	if _, err := NewApp(context.Background(), nil, nil); err == nil {
		t.Fatal("NewApp() unexpectedly accepted nil config")
	}
}

func TestAppRunAndShutdownGracefully(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve test port: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	if err := probe.Close(); err != nil {
		t.Fatalf("release test port: %v", err)
	}

	app, err := NewApp(context.Background(), &Config{
		Port: strconv.Itoa(port),
		Monitor: MonitorConfig{
			PollInterval:   time.Second,
			NotifyInterval: time.Second,
		},
		Sign: SignConfig{Provider: signProviderLocal},
	}, nil)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- app.Run()
	}()

	select {
	case <-app.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("App.Run() did not become ready")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + app.runningPort + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if err := app.Shutdown(); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	select {
	case err := <-runErrCh:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Run() error = %v, want http.ErrServerClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("App.Run() did not exit after Shutdown()")
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
