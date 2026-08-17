package douyinLive

import (
	"bytes"
	"strings"
	"testing"
)

func TestBrowserFingerprintIsCoherentAndMappedToJS(t *testing.T) {
	profile := newBrowserFingerprintWithReader(bytes.NewReader(bytes.Repeat([]byte{0x2a}, 128)))
	if profile.Preset == "" || profile.ID == "" || profile.ScreenWidth <= 0 || profile.ScreenHeight <= 0 {
		t.Fatalf("invalid profile: %#v", profile)
	}
	if profile.InnerWidth >= profile.OuterWidth || profile.InnerHeight >= profile.OuterHeight {
		t.Fatalf("inner/outer dimensions are inconsistent: %#v", profile)
	}
	if profile.OuterWidth > profile.ScreenWidth || profile.OuterHeight > profile.AvailHeight {
		t.Fatalf("outer/screen dimensions are inconsistent: %#v", profile)
	}
	if profile.DeviceMemory <= 0 || profile.HardwareConcurrency <= 0 || profile.WebGLRenderer == "" {
		t.Fatalf("hardware profile is incomplete: %#v", profile)
	}

	jsProfile := profile.jsProfile()
	if jsProfile.ID != profile.ID || jsProfile.ScreenWidth != profile.ScreenWidth || jsProfile.WebGLRenderer != profile.WebGLRenderer {
		t.Fatalf("JS profile mismatch: root=%#v js=%#v", profile, jsProfile)
	}
}

func TestHTTPUserAgentsMatchTransportImpersonationVersion(t *testing.T) {
	for _, userAgent := range impersonatedUserAgents {
		if major := chromeMajorVersionFromUserAgent(userAgent); major != httpImpersonationChromeMajor {
			t.Fatalf("UA major = %q, transport profile major = %q, UA = %q", major, httpImpersonationChromeMajor, userAgent)
		}
	}
}

func TestSessionProfilesRotateFingerprint(t *testing.T) {
	first := newSessionProfile("ua-a", staticWebsocketSigner{signature: "sig"}, "")
	defer first.close()
	second := newSessionProfile("ua-b", staticWebsocketSigner{signature: "sig"}, "")
	defer second.close()
	if first.fingerprint.ID == "" || second.fingerprint.ID == "" {
		t.Fatal("session fingerprint id is empty")
	}
	if first.fingerprint.ID == second.fingerprint.ID {
		t.Fatalf("two sessions reused fingerprint id %q", first.fingerprint.ID)
	}
}

func TestBrowserFingerprintFlowsIntoHTTPAndWebSocketParams(t *testing.T) {
	dl, err := newDouyinLive("room-short-id", nil, "", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Dispose()
	dl.fingerprint.ScreenWidth = 1536
	dl.fingerprint.ScreenHeight = 864
	dl.updateRoomInfo("room-id", "user-id", "name", "title", "")

	if params := dl.buildRoomEnterParams(); !strings.Contains(params, "screen_width=1536") || !strings.Contains(params, "screen_height=864") {
		t.Fatalf("room enter params did not use session fingerprint: %s", params)
	}
	if params := dl.buildInitialIMFetchParams(dl.roomInfoSnapshot(), "token"); !strings.Contains(params, "screen_width=1536") || !strings.Contains(params, "screen_height=864") {
		t.Fatalf("im/fetch params did not use session fingerprint: %s", params)
	}
	wsParams := newWebsocketURLParamsWithScreen(dl.roomInfoSnapshot(), dl.userAgent, "cursor", "ext", "sig", 1536, 864).QueryString()
	if !strings.Contains(wsParams, "screen_width=1536") || !strings.Contains(wsParams, "screen_height=864") {
		t.Fatalf("websocket params did not use session fingerprint: %s", wsParams)
	}
}
