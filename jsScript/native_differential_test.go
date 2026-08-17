package jsScript

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/jwwsjlm/douyinLive/v2/internal/webcastsign"
)

func TestNativeWebcastSignerMatchesInstrumentedJavaScript(t *testing.T) {
	const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	instrumented := strings.Replace(
		jsScript,
		"crawler = _0x5c2014",
		"globalThis.__nativeWebcastCore = _0x1633f2; globalThis.__nativeWebcastState = _0x6caf; crawler = _0x5c2014",
		1,
	)
	if instrumented == jsScript {
		t.Fatal("webmssdk core instrumentation point not found")
	}

	runtime := goja.New()
	if _, err := runtime.RunString(browserEnvironmentScript(ua, "ttwid=native-differential-test")); err != nil {
		t.Fatal(err)
	}
	program, err := goja.Compile("webmssdk-native-differential.js", instrumented, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RunProgram(program); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		stub          string
		counter       byte
		randomFlag    bool
		payloadRandom byte
		keyRandom     byte
	}{
		{stub: testSignatureStub, counter: 1, randomFlag: true, payloadRandom: 63, keyRandom: 63},
		{stub: testSignatureStub, counter: 2, randomFlag: false, payloadRandom: 1, keyRandom: 2},
		{stub: "00000000000000000000000000000000", counter: 63, randomFlag: true, payloadRandom: 0, keyRandom: 254},
		{stub: "ffffffffffffffffffffffffffffffff", counter: 0, randomFlag: false, payloadRandom: 254, keyRandom: 0},
		{stub: "0123456789abcdef0123456789abcdef", counter: 17, randomFlag: true, payloadRandom: 127, keyRandom: 128},
		{stub: testSignatureStub, counter: 0, randomFlag: false, payloadRandom: 0, keyRandom: 6},
	}

	for _, test := range tests {
		name := fmt.Sprintf("counter_%d_payload_%d_key_%d", test.counter, test.payloadRandom, test.keyRandom)
		t.Run(name, func(t *testing.T) {
			flagRandom := 0.24
			if test.randomFlag {
				flagRandom = 0.25
			}
			randomValues := []float64{
				flagRandom,
				(float64(test.payloadRandom) + 0.1) / 255,
				(float64(test.keyRandom) + 0.1) / 255,
			}
			if err := runtime.Set("__nativeRandomValues", randomValues); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Set("__nativeStub", test.stub); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Set("__nativeCounter", int(test.counter)); err != nil {
				t.Fatal(err)
			}
			value, err := runtime.RunString(`
				(function () {
					var values = __nativeRandomValues.slice();
					Math.random = function () { return values.shift(); };
					__nativeWebcastState.bogusIndex = (__nativeCounter + 63) & 63;
					return __nativeWebcastCore(1, false, 0, null, __nativeStub);
				})()
			`)
			if err != nil {
				t.Fatal(err)
			}
			jsSignature := value.String()
			nativeSignature, err := webcastsign.SignWithValues(
				test.stub,
				test.counter,
				test.randomFlag,
				test.payloadRandom,
				test.keyRandom,
			)
			if err != nil {
				t.Fatal(err)
			}
			if nativeSignature != jsSignature {
				t.Fatalf("native signature = %q, JavaScript signature = %q", nativeSignature, jsSignature)
			}
		})
	}
}
