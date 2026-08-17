package jsScript

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

const testSignatureStub = "704f436b2558b0d7a1c7e758527dd8f1"

func TestLoadGojaProvidesBrowserFingerprintShell(t *testing.T) {
	const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"

	if err := LoadGoja(ua); err != nil {
		t.Fatalf("LoadGoja() failed: %v", err)
	}

	value, err := vm.RunString(`JSON.stringify({
		userAgent: navigator.userAgent,
		platform: navigator.platform,
		language: navigator.language,
		languages: navigator.languages,
		cookieEnabled: navigator.cookieEnabled,
		deviceMemory: navigator.deviceMemory,
		hardwareConcurrency: navigator.hardwareConcurrency,
		maxTouchPoints: navigator.maxTouchPoints,
		webdriver: navigator.webdriver,
		vendor: navigator.vendor,
		productSub: navigator.productSub,
		pluginsLength: navigator.plugins.length,
		screenWidth: screen.width,
		screenHeight: screen.height,
		availHeight: screen.availHeight,
		colorDepth: screen.colorDepth,
		devicePixelRatio: window.devicePixelRatio,
		timezoneOffset: new Date().getTimezoneOffset(),
		localStorageLength: localStorage.length,
		indexedDB: !!indexedDB,
		canvasPrefix: document.createElement("canvas").toDataURL().slice(0, 22),
		webglVendor: document.createElement("canvas").getContext("webgl")
			.getParameter(document.createElement("canvas").getContext("webgl").getExtension("WEBGL_debug_renderer_info").UNMASKED_VENDOR_WEBGL),
		rtc: !!window.RTCPeerConnection,
		touchEvent: !!window.TouchEvent,
		battery: typeof navigator.getBattery === "function",
		sendBeacon: typeof navigator.sendBeacon === "function",
		visibilityState: document.visibilityState
	})`)
	if err != nil {
		t.Fatalf("fingerprint probe failed: %v", err)
	}

	got := value.String()
	for _, want := range []string{
		`"userAgent":"` + ua + `"`,
		`"platform":"Win32"`,
		`"language":"zh-CN"`,
		`"languages":["zh-CN","zh"]`,
		`"cookieEnabled":true`,
		`"deviceMemory":32`,
		`"hardwareConcurrency":20`,
		`"maxTouchPoints":0`,
		`"webdriver":false`,
		`"vendor":"Google Inc."`,
		`"productSub":"20030107"`,
		`"pluginsLength":5`,
		`"screenWidth":1920`,
		`"screenHeight":1080`,
		`"availHeight":1032`,
		`"colorDepth":24`,
		`"devicePixelRatio":1`,
		`"timezoneOffset":-480`,
		`"localStorageLength":5`,
		`"indexedDB":true`,
		`"canvasPrefix":"data:image/png;base64,"`,
		`"webglVendor":"Google Inc. (NVIDIA)"`,
		`"rtc":true`,
		`"touchEvent":true`,
		`"battery":true`,
		`"sendBeacon":true`,
		`"visibilityState":"visible"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("fingerprint shell missing %s in %s", want, got)
		}
	}
}

func TestLoadGojaWithCookieExposesSessionCookie(t *testing.T) {
	const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	const cookie = "ttwid=user-ttwid; passport_csrf_token=csrf-token; s_v_web_id=verify-web-id"

	if err := LoadGojaWithCookie(ua, cookie); err != nil {
		t.Fatalf("LoadGojaWithCookie() failed: %v", err)
	}

	value, err := vm.RunString(`document.cookie`)
	if err != nil {
		t.Fatalf("read document.cookie failed: %v", err)
	}
	if got := value.String(); got != cookie {
		t.Fatalf("document.cookie = %q, want %q", got, cookie)
	}
}

func TestSignerInstancesKeepProfilesIsolated(t *testing.T) {
	const (
		uaA     = "Mozilla/5.0 profile-a Chrome/150.0.0.0 Safari/537.36"
		uaB     = "Mozilla/5.0 profile-b Chrome/150.0.0.0 Safari/537.36"
		cookieA = "ttwid=room-a; msToken=token-a"
		cookieB = "ttwid=room-b; msToken=token-b"
	)

	signerA, err := NewSigner(uaA, cookieA)
	if err != nil {
		t.Fatalf("NewSigner(A) failed: %v", err)
	}
	defer signerA.Close()
	signerB, err := NewSigner(uaB, cookieB)
	if err != nil {
		t.Fatalf("NewSigner(B) failed: %v", err)
	}
	defer signerB.Close()

	readProfile := func(signer *Signer) string {
		signer.mu.Lock()
		defer signer.mu.Unlock()
		value, runErr := signer.vm.RunString(`navigator.userAgent + "|" + document.cookie`)
		if runErr != nil {
			t.Fatalf("profile probe failed: %v", runErr)
		}
		return value.String()
	}

	if got := readProfile(signerA); got != uaA+"|"+cookieA {
		t.Fatalf("signer A profile = %q", got)
	}
	if got := readProfile(signerB); got != uaB+"|"+cookieB {
		t.Fatalf("signer B profile = %q", got)
	}
	if got := readProfile(signerA); got != uaA+"|"+cookieA {
		t.Fatalf("signer A profile changed after signer B initialization: %q", got)
	}
}

func TestSignerSupportsConcurrentRepeatedCalls(t *testing.T) {
	signer, err := NewSigner(
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/150.0.0.0 Safari/537.36",
		"ttwid=repeated-test",
	)
	if err != nil {
		t.Fatalf("NewSigner() failed: %v", err)
	}
	defer signer.Close()

	const goroutines = 8
	const callsPerGoroutine = 25
	var waitGroup sync.WaitGroup
	errorsCh := make(chan error, goroutines*callsPerGoroutine)
	for range goroutines {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for range callsPerGoroutine {
				value, signErr := signer.Sign(testSignatureStub)
				if signErr != nil {
					errorsCh <- signErr
					continue
				}
				if value == "" {
					errorsCh <- errEmptySignature
				}
			}
		}()
	}
	waitGroup.Wait()
	close(errorsCh)
	for signErr := range errorsCh {
		t.Fatalf("concurrent Sign() failed: %v", signErr)
	}
}

func TestSignerCloseRejectsFurtherCalls(t *testing.T) {
	signer, err := NewSigner("Mozilla/5.0", "ttwid=close-test")
	if err != nil {
		t.Fatalf("NewSigner() failed: %v", err)
	}
	signer.Close()
	signer.Close()
	if _, err := signer.Sign(testSignatureStub); err == nil {
		t.Fatal("Sign() succeeded after Close()")
	}
}

var errEmptySignature = errors.New("empty signature")
