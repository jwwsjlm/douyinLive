package sign

import (
	"encoding/hex"
	"strings"
	"sync"
	"testing"
)

const benchmarkUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"

func TestSM3OfficialVectors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "empty",
			data: "",
			want: "1ab21d8355cfa17f8e61194831e81a8f22bec8c728fefb747ed035eb5082aa2b",
		},
		{
			name: "abc",
			data: "abc",
			want: "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hex.EncodeToString(NewSM3().Sum([]byte(tt.data)))
			if got != tt.want {
				t.Fatalf("SM3(%q) = %s, want %s", tt.data, got, tt.want)
			}
		})
	}
}

func TestSM3SupportsStreamingAndResetsAfterSum(t *testing.T) {
	hasher := NewSM3()
	hasher.Write([]byte("a"))
	hasher.Write([]byte("bc"))
	first := hex.EncodeToString(hasher.Sum(nil))
	second := hex.EncodeToString(hasher.Sum([]byte("abc")))

	const want = "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0"
	if first != want || second != want {
		t.Fatalf("streaming/reset results = %s, %s; want %s", first, second, want)
	}
}

func TestAbSignProducesExpectedShape(t *testing.T) {
	signature := AbSign("aid=6383&app_name=douyin_web&room_id=123456789", benchmarkUserAgent)
	if signature == "" {
		t.Fatal("AbSign() returned an empty signature")
	}
	if !strings.HasSuffix(signature, "=") {
		t.Fatalf("AbSign() = %q, want padded signature", signature)
	}
	if strings.ContainsAny(signature, "\r\n\t ") {
		t.Fatalf("AbSign() contains whitespace: %q", signature)
	}
	if len(signature) < 80 {
		t.Fatalf("AbSign() length = %d, want at least 80", len(signature))
	}
}

func TestAbSignSupportsConcurrentCalls(t *testing.T) {
	const workers = 32
	const iterations = 50

	var wg sync.WaitGroup
	errorsCh := make(chan string, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				signature := AbSign("aid=6383&room_id=123456789", benchmarkUserAgent)
				if signature == "" || !strings.HasSuffix(signature, "=") {
					errorsCh <- signature
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errorsCh)
	for signature := range errorsCh {
		t.Fatalf("invalid concurrent signature: %q", signature)
	}
}

func TestEncodingHelpers(t *testing.T) {
	if got := resultEncrypt("abc", "s0"); got != "YWJj" {
		t.Fatalf("resultEncrypt(abc) = %q, want YWJj", got)
	}
	if got := rc4Encrypt("payload", ""); got != "" {
		t.Fatalf("rc4Encrypt() with empty key = %q, want empty", got)
	}
	if got := getLongInt(0, "abc"); got != 0x616263 {
		t.Fatalf("getLongInt() = %#x, want %#x", got, uint32(0x616263))
	}
	if got := splitToBytes(0x01020304); string(got) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("splitToBytes() = %v", got)
	}
	if got := generRandom(0x1234, []int{0x56, 0x78}); len(got) != 4 {
		t.Fatalf("generRandom() length = %d, want 4", len(got))
	}
	if got := generateRandomStr(); len(got) != 12 {
		t.Fatalf("generateRandomStr() length = %d, want 12", len(got))
	}
}

func BenchmarkAbSign(b *testing.B) {
	params := "aid=6383&app_name=douyin_web&room_id=123456789"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = AbSign(params, benchmarkUserAgent)
	}
}

func BenchmarkSM3(b *testing.B) {
	payload := []byte("aid=6383&app_name=douyin_web&room_id=123456789")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewSM3().Sum(payload)
	}
}
