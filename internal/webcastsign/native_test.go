package webcastsign

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
)

const testXMSStub = "704f436b2558b0d7a1c7e758527dd8f1"

func TestSignWithValuesMatchesInstrumentedJavaScript(t *testing.T) {
	got, err := SignWithValues(testXMSStub, 1, true, 63, 63)
	if err != nil {
		t.Fatalf("SignWithValues() error = %v", err)
	}
	if want := "6pt0VC0aRZDP7n07"; got != want {
		t.Fatalf("SignWithValues() = %q, want JavaScript vector %q", got, want)
	}
}

func TestSignWithValuesUsesJavaScriptPlusAlphabetCharacter(t *testing.T) {
	got, err := SignWithValues(testXMSStub, 0, false, 0, 6)
	if err != nil {
		t.Fatalf("SignWithValues() error = %v", err)
	}
	if want := "fD+evFuzasjOC8Uu"; got != want {
		t.Fatalf("SignWithValues() = %q, want %q", got, want)
	}
}

func TestGeneratorUsesSessionCounterAndRandomBytes(t *testing.T) {
	generator := NewGeneratorWithReader(bytes.NewReader([]byte{
		1, 63, 63,
		0, 1, 2,
	}))
	first, err := generator.Sign(testXMSStub)
	if err != nil {
		t.Fatalf("first Sign() error = %v", err)
	}
	if first != "6pt0VC0aRZDP7n07" {
		t.Fatalf("first Sign() = %q", first)
	}
	second, err := generator.Sign(testXMSStub)
	if err != nil {
		t.Fatalf("second Sign() error = %v", err)
	}
	wantSecond, err := SignWithValues(testXMSStub, 2, false, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if second != wantSecond || second == first {
		t.Fatalf("second Sign() = %q, want %q and different from first", second, wantSecond)
	}
}

func TestGeneratorRejectsInvalidStub(t *testing.T) {
	_, err := NewGeneratorWithReader(bytes.NewReader([]byte{0, 1, 2})).Sign("not-md5")
	if !errors.Is(err, ErrInvalidXMSStub) {
		t.Fatalf("Sign() error = %v, want %v", err, ErrInvalidXMSStub)
	}
}

func TestGeneratorSupportsConcurrentCalls(t *testing.T) {
	generator := NewGenerator()
	const workers = 8
	const callsPerWorker = 50
	var waitGroup sync.WaitGroup
	errorsCh := make(chan error, workers*callsPerWorker)
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for range callsPerWorker {
				signature, err := generator.Sign(testXMSStub)
				if err != nil {
					errorsCh <- err
					continue
				}
				if len(signature) != 16 || strings.ContainsAny(signature, "\r\n\t ") {
					errorsCh <- errors.New("invalid signature shape")
				}
			}
		}()
	}
	waitGroup.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("concurrent Sign() failed: %v", err)
	}
}

func TestReadRandomByteSkipsFF(t *testing.T) {
	got, err := readRandomByte(bytes.NewReader([]byte{0xff, 0xff, 0x2a}), true)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0x2a {
		t.Fatalf("readRandomByte() = %d, want 42", got)
	}
}

func BenchmarkGeneratorSign(b *testing.B) {
	generator := NewGenerator()
	b.ReportAllocs()
	for range b.N {
		if _, err := generator.Sign(testXMSStub); err != nil {
			b.Fatal(err)
		}
	}
}
