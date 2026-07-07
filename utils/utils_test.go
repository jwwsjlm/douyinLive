package utils

import "testing"

func TestGenerateMsTokenMatchesBrowserShape(t *testing.T) {
	token := GenerateMsToken(172)
	if len(token) != 172 {
		t.Fatalf("GenerateMsToken() length = %d, want 172", len(token))
	}
	for _, r := range token {
		if (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '-' ||
			r == '_' {
			continue
		}
		t.Fatalf("GenerateMsToken() contains non-browser token char %q in %q", r, token)
	}
}

func TestGetxMSStubMatchesBrowserWebsocketKey(t *testing.T) {
	params := NewOrderedMap("7659772534023654196", "7659776308930922010")

	got := GetxMSStub(params)
	want := "94d8b625e851f0a1f70db875514e621c"
	if got != want {
		t.Fatalf("GetxMSStub() = %q, want %q", got, want)
	}
}
