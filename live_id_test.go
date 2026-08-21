package douyinLive

import (
	"errors"
	"testing"
)

func TestValidateLiveID(t *testing.T) {
	for _, value := range []string{"123456", "room-name", "room_name", " AbC123 "} {
		got, err := ValidateLiveID(value)
		if err != nil || got == "" {
			t.Fatalf("ValidateLiveID(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{"", "../123", "123/456", "直播间", string(make([]byte, 129))} {
		if _, err := ValidateLiveID(value); !errors.Is(err, ErrInvalidLiveID) {
			t.Fatalf("ValidateLiveID(%q) error = %v, want ErrInvalidLiveID", value, err)
		}
	}
}
