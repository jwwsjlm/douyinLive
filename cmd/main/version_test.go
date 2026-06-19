package main

import (
	"strings"
	"testing"
)

func TestVersionStringIncludesBuildMetadata(t *testing.T) {
	got := VersionString()
	for _, want := range []string{
		"tag=" + buildTag,
		"commit=" + buildCommit,
		"buildDate=" + buildDate,
		"source=" + buildSource,
		"signProvider=" + defaultSignProvider,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("VersionString() = %q, want it to include %q", got, want)
		}
	}
}
