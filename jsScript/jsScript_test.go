package jsScript

import (
	"strings"
	"testing"
)

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
