package server

import (
	"os"
	"path/filepath"
	"testing"
)

func resetConfigGlobalsForTest(t *testing.T, args ...string) {
	t.Helper()
	originalArgs := os.Args
	os.Args = append([]string{"douyinLive-test"}, args...)

	t.Cleanup(func() {
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

func TestConfigAutoDiscoveryPreservesLegacyExtensions(t *testing.T) {
	directory := t.TempDir()
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDirectory) })

	if err := os.WriteFile(filepath.Join(directory, "config.yml"), []byte("api:\n  key: yml-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, loaded, err := findConfigFile("")
	if err != nil {
		t.Fatalf("findConfigFile() failed: %v", err)
	}
	if !loaded || filepath.Base(path) != "config.yml" {
		t.Fatalf("discovered path = %q loaded=%v, want config.yml", path, loaded)
	}
	schema, err := loadConfigFileSchemaWithProvider(path, signProviderLocal)
	if err != nil {
		t.Fatalf("loadConfigFileSchema() error = %v", err)
	}
	if schema.API.Key != "yml-key" {
		t.Fatalf("API key = %q, want yml-key", schema.API.Key)
	}
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

func TestConfigParsesListEnvironmentVariables(t *testing.T) {
	configPath := writeConfigFixture(t)
	t.Setenv("APP_API_ALLOWED_DOMAINS", "live.douyin.com, www.douyin.com")
	t.Setenv("APP_WEBSOCKET_ALLOWED_ORIGINS", `["https://one.example.com","https://two.example.com"]`)
	resetConfigGlobalsForTest(t, "--config", configPath)

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if got := cfg.API.AllowedDomains; len(got) != 2 || got[0] != "live.douyin.com" || got[1] != "www.douyin.com" {
		t.Fatalf("AllowedDomains = %#v", got)
	}
	if got := cfg.WebSocket.AllowedOrigins; len(got) != 2 || got[0] != "https://one.example.com" || got[1] != "https://two.example.com" {
		t.Fatalf("AllowedOrigins = %#v", got)
	}
}

func TestConfigPreservesCaseSensitiveRoomCookieKeys(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("port: '1088'\ncookie:\n  rooms:\n    AbC123: 'room-cookie'\n")
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	resetConfigGlobalsForTest(t, "--config", configPath)

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if got := cfg.Cookie.Rooms["AbC123"]; got != "room-cookie" {
		t.Fatalf("case-sensitive room cookie = %q, map=%#v", got, cfg.Cookie.Rooms)
	}
	if _, exists := cfg.Cookie.Rooms["abc123"]; exists {
		t.Fatalf("room cookie key was unexpectedly lower-cased: %#v", cfg.Cookie.Rooms)
	}
}

func TestConfigRoomCookieEnvironmentOverridesFileAndPreservesCase(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("port: '1088'\ncookie:\n  rooms:\n    AbC123: 'file-cookie'\n")
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	t.Setenv("APP_COOKIE_ROOMS", `{"AbC123":"env-cookie","XYZ789":"second-cookie"}`)
	resetConfigGlobalsForTest(t, "--config", configPath)

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if got := cfg.Cookie.Rooms["AbC123"]; got != "env-cookie" {
		t.Fatalf("environment room cookie = %q, map=%#v", got, cfg.Cookie.Rooms)
	}
	if got := cfg.Cookie.Rooms["XYZ789"]; got != "second-cookie" {
		t.Fatalf("second environment room cookie = %q, map=%#v", got, cfg.Cookie.Rooms)
	}
	if len(cfg.Cookie.Rooms) != 2 {
		t.Fatalf("file room cookies were not replaced by environment map: %#v", cfg.Cookie.Rooms)
	}
}

func TestConfigRejectsInvalidRoomCookieEnvironmentJSON(t *testing.T) {
	configPath := writeConfigFixture(t)
	t.Setenv("APP_COOKIE_ROOMS", "not-json")
	resetConfigGlobalsForTest(t, "--config", configPath)
	if _, err := NewConfig(); err == nil {
		t.Fatal("invalid APP_COOKIE_ROOMS unexpectedly accepted")
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

func TestConfigRejectsCommaInsideAllowedDomainItem(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("port: '1088'\napi:\n  allowed_domains:\n    - live.douyin.com,www.douyin.com\n")
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	resetConfigGlobalsForTest(t, "--config", configPath)
	if _, err := NewConfig(); err == nil {
		t.Fatal("comma-separated domains inside one YAML item were unexpectedly accepted")
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

func TestConfigStrictSchemaAcceptsLegacyV21Config(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("port: '1088'\nunknown: false\nlog:\n  level: info\nsign:\n  provider: local\ntikhub:\n  key: ''\nmonitor:\n  poll_interval: 15s\n  notify_interval: 30s\ncookie:\n  douyin: ''\n  rooms:\n    AbC123: 'legacy-room-cookie'\n")
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	resetConfigGlobalsForTest(t, "--config", configPath)
	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() rejected complete v2.1 config: %v", err)
	}
	if !cfg.Cookie.UseStored || cfg.WebSocket.Path != "/ws" || cfg.API.Key != "" || cfg.Cookie.Rooms["AbC123"] != "legacy-room-cookie" {
		t.Fatalf("legacy defaults/config changed: %+v", cfg)
	}
}
