package douyinLive_test

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	douyinLive "github.com/jwwsjlm/douyinLive/v2"
)

func TestPublicLibraryAPICompilesAndHonorsContext(t *testing.T) {
	var logger douyinLive.Logger = log.New(io.Discard, "", 0)
	live, err := douyinLive.NewDouyinLive("library-test", logger, "ttwid=test")
	if err != nil {
		t.Fatalf("NewDouyinLive() failed: %v", err)
	}
	defer live.Dispose()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err := live.CheckLiveStatus(ctx)
	if !errors.Is(err, douyinLive.ErrLiveStatusUnknown) {
		t.Fatalf("CheckLiveStatus() error = %v, want ErrLiveStatusUnknown", err)
	}
	if status.Code != douyinLive.LiveStatusUnknown {
		t.Fatalf("CheckLiveStatus() code = %q, want %q", status.Code, douyinLive.LiveStatusUnknown)
	}

	var _ = douyinLive.LiveStatusOnline
}
