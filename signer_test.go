package douyinLive

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type failingWebsocketSigner struct {
	err error
}

func (s failingWebsocketSigner) Name() string { return "failing" }

func (s failingWebsocketSigner) Sign(context.Context, string, string, string) (string, error) {
	return "", s.err
}

func (s failingWebsocketSigner) UpdateUserAgent(string) {}

func TestNewDouyinLiveDefaultsLocalSigner(t *testing.T) {
	dl, err := NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatalf("NewDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()

	if got := dl.signer.Name(); got != SignProviderLocal {
		t.Fatalf("signer = %q, want %q", got, SignProviderLocal)
	}
}

func TestNewDouyinLiveWithTikHubUsesTikHubSigner(t *testing.T) {
	dl, err := NewDouyinLiveWithTikHub("live-id", nil, "", "api-key")
	if err != nil {
		t.Fatalf("NewDouyinLiveWithTikHub() failed: %v", err)
	}
	defer dl.Dispose()

	if got := dl.signer.Name(); got != SignProviderTikHub {
		t.Fatalf("signer = %q, want %q", got, SignProviderTikHub)
	}
}

func TestTikHubSignerRequiresToken(t *testing.T) {
	signer := newTikHubWebsocketSigner("", "Mozilla/5.0")

	_, err := signer.Sign(context.Background(), "room-id", "user-id", "Mozilla/5.0")
	if !errors.Is(err, ErrTikHubTokenEmpty) {
		t.Fatalf("Sign() err = %v, want ErrTikHubTokenEmpty", err)
	}
}

func TestBuildWebsocketURLReturnsSignerError(t *testing.T) {
	wantErr := errors.New("sign failed")
	dl, err := newDouyinLive("live-id", nil, "", failingWebsocketSigner{err: wantErr})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.updateRoomInfo("room-id", "user-id", "live-name", "title", "avatar")

	_, err = dl.buildWebsocketURL()
	if !errors.Is(err, wantErr) {
		t.Fatalf("buildWebsocketURL() err = %v, want %v", err, wantErr)
	}
}

func TestBuildWebsocketURLEscapesSignerOutput(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "", staticWebsocketSigner{signature: "ab+c/d e"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.updateRoomInfo("room-id", "user-id", "live-name", "title", "avatar")

	url, err := dl.buildWebsocketURL()
	if err != nil {
		t.Fatalf("buildWebsocketURL() failed: %v", err)
	}
	if !strings.Contains(url, "signature=ab%2Bc%2Fd%20e") {
		t.Fatalf("buildWebsocketURL() did not escape signature correctly: %s", url)
	}
}

type staticWebsocketSigner struct {
	signature string
}

func (s staticWebsocketSigner) Name() string { return "static" }

func (s staticWebsocketSigner) Sign(context.Context, string, string, string) (string, error) {
	return s.signature, nil
}

func (s staticWebsocketSigner) UpdateUserAgent(string) {}

func TestExtractTikHubSignature(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "data string", body: `{"code":200,"request_id":"req-id","data":"XB_SIGNATURE"}`, want: "XB_SIGNATURE"},
		{name: "data object xb", body: `{"code":200,"data":{"xb":"XB_OBJECT"}}`, want: "XB_OBJECT"},
		{name: "data object x bogus", body: `{"code":200,"data":{"X-Bogus":"XB_BOGUS"}}`, want: "XB_BOGUS"},
		{name: "ignore request id", body: `{"code":200,"request_id":"req-id","message":"Request successful."}`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractTikHubSignature([]byte(tt.body)); got != tt.want {
				t.Fatalf("extractTikHubSignature() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeTikHubSignature(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "raw", in: "XB_RAW", want: "XB_RAW"},
		{name: "query signature", in: "signature=XB_QUERY", want: "XB_QUERY"},
		{name: "query x bogus", in: "X-Bogus=XB_BOGUS", want: "XB_BOGUS"},
		{name: "url", in: "https://example.com/path?signature=XB_URL", want: "XB_URL"},
		{name: "raw plus", in: "XB+RAW/VALUE", want: "XB+RAW/VALUE"},
		{name: "escaped raw", in: "XB%2BVALUE", want: "XB+VALUE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeTikHubSignature(tt.in); got != tt.want {
				t.Fatalf("normalizeTikHubSignature() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTikHubAPIResponseSuccessCode(t *testing.T) {
	tests := []struct {
		name string
		code int
		want bool
	}{
		{name: "zero", code: 0, want: true},
		{name: "http ok", code: 200, want: true},
		{name: "unauthorized", code: 401, want: false},
		{name: "business error", code: 10001, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTikHubSuccessCode(tt.code); got != tt.want {
				t.Fatalf("isTikHubSuccessCode(%d) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

func TestLiveStatusGuardRequiresConsecutiveOfflineConfirmations(t *testing.T) {
	guard := liveStatusGuard{}

	if guard.Record(true) {
		t.Fatalf("Record(true) closed connection")
	}
	if guard.Record(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	if !guard.Record(false) {
		t.Fatalf("second consecutive offline confirmation did not close connection")
	}
}

func TestLiveStatusGuardResetsAfterOnlineConfirmation(t *testing.T) {
	guard := liveStatusGuard{}

	if guard.Record(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	if guard.Record(true) {
		t.Fatalf("online confirmation closed connection")
	}
	if guard.Record(false) {
		t.Fatalf("offline confirmation after reset closed connection")
	}
}

func TestLiveStatusGuardCanResetForNewConnection(t *testing.T) {
	guard := liveStatusGuard{}

	if guard.Record(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	guard.Reset()
	if guard.Record(false) {
		t.Fatalf("offline confirmation after connection reset closed connection")
	}
}

func TestStatusCheckKeepsLiveStateUntilSecondOfflineConfirmation(t *testing.T) {
	dl := &DouyinLive{}
	dl.setLiveStatus(true)

	if dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	if !dl.isLiveStatus() {
		t.Fatalf("first offline confirmation changed live state")
	}
	if !dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("second offline confirmation did not close connection")
	}
	if dl.isLiveStatus() {
		t.Fatalf("second offline confirmation did not change live state")
	}
}

func TestStatusCheckOnlineConfirmationResetsOfflineGuard(t *testing.T) {
	dl := &DouyinLive{}
	dl.setLiveStatus(true)

	if dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	if dl.shouldCloseAfterStatusCheck(true) {
		t.Fatalf("online confirmation closed connection")
	}
	if !dl.isLiveStatus() {
		t.Fatalf("online confirmation changed live state")
	}
	if dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("offline confirmation after online reset closed connection")
	}
	if !dl.isLiveStatus() {
		t.Fatalf("offline confirmation after online reset changed live state")
	}
}

func TestSetLiveStatusOnlineResetsOfflineGuard(t *testing.T) {
	dl := &DouyinLive{}
	dl.setLiveStatus(true)

	if dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	dl.setLiveStatus(true)
	if dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("offline confirmation after setLiveStatus(true) closed connection")
	}
	if !dl.isLiveStatus() {
		t.Fatalf("offline confirmation after setLiveStatus(true) changed live state")
	}
}
