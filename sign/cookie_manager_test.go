package sign

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
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

func TestCookieManagerSaveConfigRestrictsFilePermissions(t *testing.T) {
	for _, precreate := range []bool{false, true} {
		name := "new"
		if precreate {
			name = "overwrite"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cookie.yaml")
			if precreate {
				if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
					t.Fatalf("create existing config: %v", err)
				}
				if runtime.GOOS != "windows" {
					if err := os.Chmod(path, 0o644); err != nil {
						t.Fatalf("set existing config permissions: %v", err)
					}
				}
			}

			manager := NewCookieManager()
			manager.SetDouyinCookie("ttwid=sensitive-cookie")
			if err := manager.SaveConfig(path); err != nil {
				t.Fatalf("SaveConfig() error = %v", err)
			}
			if runtime.GOOS == "windows" {
				return
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat saved config: %v", err)
			}
			if got := info.Mode().Perm(); got != 0o600 {
				t.Fatalf("saved config permissions = %04o, want 0600", got)
			}
		})
	}
}

func TestCookieManagerSaveConfigLeavesNoTemporaryFiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "cookie.yaml")
	manager := NewCookieManager()
	manager.SetDouyinCookie("ttwid=sensitive-cookie")
	if err := manager.SaveConfig(path); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(directory, ".cookie.yaml.tmp-*"))
	if err != nil {
		t.Fatalf("glob temporary files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left after successful save: %v", matches)
	}
}

func TestAtomicCookieSaveFailurePreservesExistingFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "cookie.yaml")
	oldData := []byte("cookie:\n  douyin: old-cookie\n")
	if err := os.WriteFile(path, oldData, 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}
	renamingFailed := errors.New("injected rename failure")
	err := writeFileAtomicallyUsing(path, []byte("cookie:\n  douyin: new-cookie\n"), 0o600, func(_, _ string) error {
		return renamingFailed
	})
	if !errors.Is(err, renamingFailed) {
		t.Fatalf("writeFileAtomicallyUsing() error = %v, want injected failure", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read preserved config: %v", err)
	}
	if !reflect.DeepEqual(got, oldData) {
		t.Fatalf("existing config changed after failed replacement: %q", got)
	}
	matches, err := filepath.Glob(filepath.Join(directory, ".cookie.yaml.tmp-*"))
	if err != nil {
		t.Fatalf("glob temporary files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left after failed save: %v", matches)
	}
}

func TestCookieManagerSaveConfigRejectsSymbolicLink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.yaml")
	link := filepath.Join(directory, "cookie.yaml")
	oldData := []byte("cookie:\n  douyin: target-cookie\n")
	if err := os.WriteFile(target, oldData, 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}

	manager := NewCookieManager()
	manager.SetDouyinCookie("ttwid=new-cookie")
	if err := manager.SaveConfig(link); err == nil {
		t.Fatal("SaveConfig() unexpectedly followed or replaced a symbolic link")
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat symbolic link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symbolic link was replaced after rejected save")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read symlink target: %v", err)
	}
	if !reflect.DeepEqual(got, oldData) {
		t.Fatalf("symlink target changed after rejected save: %q", got)
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

func TestCookieManagerSupportsConcurrentAccess(t *testing.T) {
	manager := NewCookieManager()
	const workers = 8
	const iterations = 50
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				manager.SetDouyinCookie("ttwid=worker-cookie")
				_ = manager.GetDouyinCookie()
				if err := manager.SetCookies("https://live.douyin.com/", "ttwid=one"); err != nil {
					t.Errorf("worker %d SetCookies() error = %v", worker, err)
					return
				}
				_ = manager.GetCookies("https://live.douyin.com/room")
			}
		}(worker)
	}
	wg.Wait()
}
