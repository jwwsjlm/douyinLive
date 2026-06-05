package douyinLive_test

import (
	"testing"

	douyinLive "github.com/jwwsjlm/douyinLive/v2"
)

func TestNewDouyinLive(t *testing.T) {
	d, err := douyinLive.NewDouyinLive("740934774657", nil, "")
	if err != nil {
		t.Fatalf("NewDouyinLive() failed: %v", err)
	}
	d.Dispose()
}
