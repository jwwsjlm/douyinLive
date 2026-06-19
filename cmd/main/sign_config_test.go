package main

import "testing"

func TestNormalizeSignProvider(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty uses default", in: "", want: defaultSignProvider},
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

func TestNormalizeSignProviderEmptyFollowsBuildDefault(t *testing.T) {
	original := defaultSignProvider
	defaultSignProvider = signProviderTikHub
	t.Cleanup(func() {
		defaultSignProvider = original
	})

	got, err := normalizeSignProvider("")
	if err != nil {
		t.Fatalf("normalizeSignProvider() returned error: %v", err)
	}
	if got != signProviderTikHub {
		t.Fatalf("normalizeSignProvider(\"\") = %q, want %q", got, signProviderTikHub)
	}
}
