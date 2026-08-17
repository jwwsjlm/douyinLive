package sign

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCookieManagerConfigurationLifecycle(t *testing.T) {
	manager := NewCookieManager()
	if got := manager.GetDouyinCookie(); got != "" {
		t.Fatalf("initial cookie = %q, want empty", got)
	}

	manager.SetDouyinCookie("ttwid=one; sessionid=two")
	if got := manager.GetDouyinCookie(); got != "ttwid=one; sessionid=two" {
		t.Fatalf("configured cookie = %q", got)
	}
	if !manager.ValidateCookie(manager.GetDouyinCookie()) {
		t.Fatal("ValidateCookie() rejected a ttwid cookie")
	}
	if manager.ValidateCookie("plain=value") {
		t.Fatal("ValidateCookie() accepted an unrelated cookie")
	}
	if got := manager.GetCookieNames(manager.GetDouyinCookie()); !reflect.DeepEqual(got, []string{"ttwid", "sessionid"}) {
		t.Fatalf("cookie names = %v", got)
	}

	manager.UpdateCookie("douyin", "odin_tt=updated")
	if got := manager.GetDouyinCookie(); got != "odin_tt=updated" {
		t.Fatalf("updated cookie = %q", got)
	}
}

func TestCookieManagerJarRoundTrip(t *testing.T) {
	manager := NewCookieManager()
	if err := manager.SetCookies("https://live.douyin.com/", "ttwid=one; sessionid=two; invalid"); err != nil {
		t.Fatalf("SetCookies() error = %v", err)
	}
	cookies := manager.GetCookies("https://live.douyin.com/room")
	if len(cookies) != 2 {
		t.Fatalf("GetCookies() returned %d cookies, want 2", len(cookies))
	}
	if got := manager.GetCookies("://bad-url"); got != nil {
		t.Fatalf("GetCookies() for invalid URL = %v, want nil", got)
	}
	if err := manager.SetCookies("://bad-url", "ttwid=one"); err == nil {
		t.Fatal("SetCookies() unexpectedly accepted invalid URL")
	}
}

func TestCookieManagerLoadsAndSavesYAML(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.yaml")
	output := filepath.Join(dir, "output.yaml")
	if err := os.WriteFile(input, []byte("cookie:\n  douyin: 'ttwid=from-file'\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	manager := NewCookieManager()
	if err := manager.LoadConfig(input); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := manager.GetDouyinCookie(); got != "ttwid=from-file" {
		t.Fatalf("loaded cookie = %q", got)
	}
	if err := manager.SaveConfig(output); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	reloaded := NewCookieManager()
	if err := reloaded.LoadConfig(output); err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if got := reloaded.GetDouyinCookie(); got != "ttwid=from-file" {
		t.Fatalf("reloaded cookie = %q", got)
	}
}

func TestCookieManagerLoadsEnvironment(t *testing.T) {
	t.Setenv("DOUYIN_COOKIE", "passport_csrf_token=from-env")
	manager := NewCookieManager()
	manager.LoadFromEnv()
	if got := manager.GetDouyinCookie(); got != "passport_csrf_token=from-env" {
		t.Fatalf("environment cookie = %q", got)
	}
}
