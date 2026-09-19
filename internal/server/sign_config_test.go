package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeSignProvider(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty uses default", in: "", want: signProviderLocal},
		{name: "local", in: "local", want: "local"},
		{name: "js alias", in: "js", want: "local"},
		{name: "tikhub", in: "tikhub", want: "tikhub"},
		{name: "case and space", in: " TikHub ", want: "tikhub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSignProvider(tt.in)
			if err != nil {
				t.Fatalf("normalizeSignProvider() returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeSignProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeSignProviderRejectsUnknown(t *testing.T) {
	if _, err := normalizeSignProvider("bad"); err == nil {
		t.Fatalf("normalizeSignProvider() returned nil error for unknown provider")
	}
}

func TestNormalizeSignProviderEmptyFollowsConfiguredDefault(t *testing.T) {
	got, err := normalizeSignProviderWithDefault("", signProviderTikHub)
	if err != nil {
		t.Fatalf("normalizeSignProvider() returned error: %v", err)
	}
	if got != signProviderTikHub {
		t.Fatalf("normalizeSignProvider(\"\") = %q, want %q", got, signProviderTikHub)
	}
}

func TestNewConfigUsesProvidedDefaultSignProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("port: '1088'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_TIKHUB_KEY", "test-key")
	resetConfigGlobalsForTest(t, "--config", path)

	config, err := newConfig(signProviderTikHub)
	if err != nil {
		t.Fatal(err)
	}
	if config.Sign.Provider != signProviderTikHub {
		t.Fatalf("Sign.Provider = %q, want %q", config.Sign.Provider, signProviderTikHub)
	}
}
