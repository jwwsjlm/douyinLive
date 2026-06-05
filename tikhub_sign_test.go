package douyinLive

import (
	"errors"
	"testing"
)

func TestExtractTikHubSignature(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "data string",
			body: `{"code":200,"request_id":"req-id","data":"XB_SIGNATURE"}`,
			want: "XB_SIGNATURE",
		},
		{
			name: "data object xb",
			body: `{"code":200,"data":{"xb":"XB_OBJECT"}}`,
			want: "XB_OBJECT",
		},
		{
			name: "data object x bogus",
			body: `{"code":200,"data":{"X-Bogus":"XB_BOGUS"}}`,
			want: "XB_BOGUS",
		},
		{
			name: "ignore request id",
			body: `{"code":200,"request_id":"req-id","message":"Request successful."}`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractTikHubSignature([]byte(tt.body)); got != tt.want {
				t.Fatalf("extractTikHubSignature() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateTikHubXBSignatureRequiresToken(t *testing.T) {
	dl, err := NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatalf("NewDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()

	_, err = dl.generateTikHubXBSignature("room-id", "user-id")
	if !errors.Is(err, ErrTikHubTokenEmpty) {
		t.Fatalf("generateTikHubXBSignature() err = %v, want ErrTikHubTokenEmpty", err)
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
