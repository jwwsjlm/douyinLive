package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func resetConfigGlobalsForTest(t *testing.T, args ...string) {
	t.Helper()
	originalArgs := os.Args
	originalFlags := pflag.CommandLine

	viper.Reset()
	pflag.CommandLine = pflag.NewFlagSet("douyinLive-test", pflag.ContinueOnError)
	os.Args = append([]string{"douyinLive-test"}, args...)

	t.Cleanup(func() {
		viper.Reset()
		pflag.CommandLine = originalFlags
		os.Args = originalArgs
	})
}

func writeConfigFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("port: '1088'\nlog:\n  level: debug\nsign:\n  provider: local\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	return path
}

func TestConfigPriorityFlagOverEnvironmentOverFile(t *testing.T) {
	configPath := writeConfigFixture(t)
	t.Setenv("APP_PORT", "1089")
	t.Setenv("APP_LOG_LEVEL", "warn")
	resetConfigGlobalsForTest(t,
		"--config", configPath,
		"--port", "1090",
		"--log-level", "error",
	)

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if cfg.Port != "1090" || cfg.Log.Level != "error" || !cfg.Cookie.UseStored {
		t.Fatalf("flag priority result: port=%q log=%q use_stored=%v", cfg.Port, cfg.Log.Level, cfg.Cookie.UseStored)
	}
}

func TestConfigCanDisableStoredCookies(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("port: '1088'\ncookie:\n  use_stored: false\n  douyin: 'global-cookie'\n")
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	resetConfigGlobalsForTest(t, "--config", configPath)
	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if cfg.Cookie.UseStored {
		t.Fatal("cookie.use_stored=false was not preserved")
	}
}

func TestConfigSupportsCustomWebSocketPath(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: '1088'\nwebsocket:\n  path: /live-stream\n"), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	resetConfigGlobalsForTest(t, "--config", configPath)
	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if cfg.WebSocket.Path != "/live-stream" {
		t.Fatalf("WebSocket.Path = %q, want /live-stream", cfg.WebSocket.Path)
	}
}

func TestConfigNormalizesWebSocketOrigins(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: '1088'\nwebsocket:\n  allowed_origins:\n    - HTTPS://Client.Example.com\n    - https://client.example.com\n"), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	resetConfigGlobalsForTest(t, "--config", configPath)
	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if len(cfg.WebSocket.AllowedOrigins) != 1 || cfg.WebSocket.AllowedOrigins[0] != "https://client.example.com" {
		t.Fatalf("AllowedOrigins = %#v", cfg.WebSocket.AllowedOrigins)
	}
}

func TestConfigRejectsInvalidWebSocketOrigin(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: '1088'\nwebsocket:\n  allowed_origins:\n    - not-an-origin\n"), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	resetConfigGlobalsForTest(t, "--config", configPath)
	if _, err := NewConfig(); err == nil {
		t.Fatal("invalid Origin unexpectedly accepted")
	}
}

func TestConfigPriorityEnvironmentOverFile(t *testing.T) {
	configPath := writeConfigFixture(t)
	t.Setenv("APP_PORT", "1089")
	t.Setenv("APP_LOG_LEVEL", "warn")
	resetConfigGlobalsForTest(t, "--config", configPath)

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if cfg.Port != "1089" || cfg.Log.Level != "warn" {
		t.Fatalf("environment priority result: port=%q log=%q", cfg.Port, cfg.Log.Level)
	}
}

func TestConfigRejectsTikHubWithoutKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: '1088'\nsign:\n  provider: tikhub\n"), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	resetConfigGlobalsForTest(t, "--config", configPath)

	if _, err := NewConfig(); err == nil {
		t.Fatal("NewConfig() unexpectedly accepted TikHub without an API key")
	}
}

func TestConfigRejectsUnknownYAMLFields(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("port: '1088'\napi:\n  kye: should-not-be-ignored\n")
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	resetConfigGlobalsForTest(t, "--config", configPath)
	if _, err := NewConfig(); err == nil {
		t.Fatal("NewConfig() unexpectedly accepted an unknown security-related field")
	}
}

func TestConfigStrictSchemaAcceptsLegacyMinimalConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 1088\n"), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	resetConfigGlobalsForTest(t, "--config", configPath)
	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() rejected legacy minimal config: %v", err)
	}
	if cfg.Port != "1088" {
		t.Fatalf("Port = %q, want 1088", cfg.Port)
	}
}
