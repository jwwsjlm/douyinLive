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
	originalShowVersion := showVersion

	viper.Reset()
	pflag.CommandLine = pflag.NewFlagSet("douyinLive-test", pflag.ContinueOnError)
	os.Args = append([]string{"douyinLive-test"}, args...)
	showVersion = false

	t.Cleanup(func() {
		viper.Reset()
		pflag.CommandLine = originalFlags
		os.Args = originalArgs
		showVersion = originalShowVersion
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
	if cfg.Port != "1090" || cfg.Log.Level != "error" {
		t.Fatalf("flag priority result: port=%q log=%q", cfg.Port, cfg.Log.Level)
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
