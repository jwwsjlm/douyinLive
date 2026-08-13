package main

import (
	"errors"
	"testing"

	douyinLive "github.com/jwwsjlm/douyinLive/v2"
)

type fakeLiveStatusChecker struct {
	knownOffline bool
	knownLive    bool
	isLive       bool
	err          error
	checks       int
}

func (f *fakeLiveStatusChecker) IsKnownOfflineStatus() bool { return f.knownOffline }
func (f *fakeLiveStatusChecker) IsKnownLiveStatus() bool    { return f.knownLive }
func (f *fakeLiveStatusChecker) IsLive() (bool, error) {
	f.checks++
	return f.isLive, f.err
}

func TestConfirmLiveSessionStatusRejectsKnownOffline(t *testing.T) {
	checker := &fakeLiveStatusChecker{knownOffline: true}
	err := confirmLiveSessionStatus(checker)
	if !errors.Is(err, douyinLive.ErrLiveNotStarted) {
		t.Fatalf("confirmLiveSessionStatus() error = %v, want ErrLiveNotStarted", err)
	}
	if checker.checks != 0 {
		t.Fatalf("known offline state triggered %d refresh checks", checker.checks)
	}
}

func TestConfirmLiveSessionStatusAcceptsKnownLive(t *testing.T) {
	checker := &fakeLiveStatusChecker{knownLive: true}
	if err := confirmLiveSessionStatus(checker); err != nil {
		t.Fatalf("confirmLiveSessionStatus() failed: %v", err)
	}
	if checker.checks != 0 {
		t.Fatalf("known live state triggered %d refresh checks", checker.checks)
	}
}

func TestConfirmLiveSessionStatusDoesNotTreatUnknownAsLive(t *testing.T) {
	checker := &fakeLiveStatusChecker{}
	err := confirmLiveSessionStatus(checker)
	if !errors.Is(err, douyinLive.ErrLiveNotStarted) {
		t.Fatalf("confirmLiveSessionStatus() error = %v, want ErrLiveNotStarted", err)
	}
	if checker.checks != 1 {
		t.Fatalf("unknown state refresh checks = %d, want 1", checker.checks)
	}
}

func TestConfirmLiveSessionStatusAcceptsRefreshedLiveState(t *testing.T) {
	checker := &fakeLiveStatusChecker{isLive: true}
	if err := confirmLiveSessionStatus(checker); err != nil {
		t.Fatalf("confirmLiveSessionStatus() failed: %v", err)
	}
	if checker.checks != 1 {
		t.Fatalf("unknown state refresh checks = %d, want 1", checker.checks)
	}
}

func TestConfirmLiveSessionStatusPropagatesRefreshError(t *testing.T) {
	want := errors.New("status request failed")
	checker := &fakeLiveStatusChecker{err: want}
	err := confirmLiveSessionStatus(checker)
	if !errors.Is(err, want) {
		t.Fatalf("confirmLiveSessionStatus() error = %v, want wrapped %v", err, want)
	}
}
