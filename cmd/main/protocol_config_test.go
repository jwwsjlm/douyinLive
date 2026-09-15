package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNormalizeProtocolMode 覆盖协议画像配置的归一化与校验。
func TestNormalizeProtocolMode(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "web", false},
		{"pc", "pc", false},
		{"PC", "pc", false},
		{"  pc  ", "pc", false},
		{"web", "web", false},
		{"WEB", "web", false},
		{" Web ", "web", false},
		{"bogus", "", true},
		{"client", "", true},
		{"pcweb", "", true},
	}
	for _, tc := range cases {
		got, err := normalizeProtocolMode(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeProtocolMode(%q) 未报错", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeProtocolMode(%q) 报错: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeProtocolMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestProtocolConfigPriority 校验 YAML、环境变量、命令行三级覆盖顺序。
func TestProtocolConfigPriority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("protocol:\n  mode: web\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// YAML 值生效。
	resetConfigGlobalsForTest(t, "--config", path)
	config, err := NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Protocol.Mode != "web" {
		t.Fatalf("YAML protocol.mode = %q, want web", config.Protocol.Mode)
	}

	// 环境变量覆盖 YAML。
	t.Setenv("APP_PROTOCOL", "pc")
	config, err = NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Protocol.Mode != "pc" {
		t.Fatalf("环境变量 protocol = %q, want pc", config.Protocol.Mode)
	}

	// 命令行覆盖环境变量。
	resetConfigGlobalsForTest(t, "--config", path, "--protocol", "web")
	config, err = NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Protocol.Mode != "web" {
		t.Fatalf("命令行 protocol = %q, want web", config.Protocol.Mode)
	}

	// 非法取值必须被拒绝，避免静默回退到默认画像。
	resetConfigGlobalsForTest(t, "--config", path, "--protocol", "bogus")
	if _, err := NewConfig(); err == nil {
		t.Fatal("非法 protocol 配置未被拒绝")
	}
}

// TestRoomInheritsConfiguredProtocolMode 校验协议画像经配置层传递到房间。
func TestRoomInheritsConfiguredProtocolMode(t *testing.T) {
	useStored := true
	for _, mode := range []string{"pc", "web"} {
		rm := NewRoomManagerWithOptions(RoomManagerOptions{
			SignProvider: signProviderLocal, ProtocolMode: mode,
			PollInterval: time.Second, NotifyInterval: time.Second, UseStoredCookie: &useStored,
		})
		room := rm.GetOrCreateRoom("1001", "")
		if room == nil {
			t.Fatalf("protocol=%s: GetOrCreateRoom 返回 nil", mode)
		}
		if room.protocolMode != mode {
			t.Errorf("protocol=%s: room.protocolMode = %q", mode, room.protocolMode)
		}
		rm.Close()
	}

	// 空值必须在管理器入口归一化为默认画像，而不是把空串透传到房间。
	rm := NewRoomManagerWithOptions(RoomManagerOptions{
		SignProvider: signProviderLocal,
		PollInterval: time.Second, NotifyInterval: time.Second, UseStoredCookie: &useStored,
	})
	room := rm.GetOrCreateRoom("1001", "")
	if room == nil {
		t.Fatal("GetOrCreateRoom 返回 nil")
	}
	if room.protocolMode != defaultProtocolMode {
		t.Errorf("默认 room.protocolMode = %q, want %q", room.protocolMode, defaultProtocolMode)
	}
	rm.Close()
}

// TestRoomManagerFallsBackOnInvalidProtocolMode 校验管理器层的无效值回退。
func TestRoomManagerFallsBackOnInvalidProtocolMode(t *testing.T) {
	useStored := true
	rm := NewRoomManagerWithOptions(RoomManagerOptions{
		SignProvider: signProviderLocal, ProtocolMode: "  WEB  ",
		PollInterval: time.Second, NotifyInterval: time.Second, UseStoredCookie: &useStored,
	})
	defer rm.Close()
	if rm.protocolMode != "web" {
		t.Fatalf("大小写与空格未归一化: %q", rm.protocolMode)
	}
}

// TestProtocolConfigDefaultsToWeb 校验未配置时使用默认画像。
func TestProtocolConfigDefaultsToWeb(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("port: '1088'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resetConfigGlobalsForTest(t, "--config", path)
	config, err := NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Protocol.Mode != defaultProtocolMode {
		t.Fatalf("默认 protocol = %q, want %q", config.Protocol.Mode, defaultProtocolMode)
	}
	if defaultProtocolMode != "web" {
		t.Fatalf("默认协议画像应为 web，实际 %q", defaultProtocolMode)
	}
}
