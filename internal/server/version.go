package server

import "fmt"

// BuildInfo is the immutable build metadata supplied by cmd/main.
type BuildInfo struct {
	Tag                 string
	Commit              string
	Date                string
	Source              string
	DefaultSignProvider string
}

// VersionString returns the build version information used by --version and health.
func (info BuildInfo) VersionString() string {
	return fmt.Sprintf("tag=%s commit=%s buildDate=%s source=%s signProvider=%s", info.Tag, info.Commit, info.Date, info.Source, info.DefaultSignProvider)
}

func defaultBuildInfo() BuildInfo {
	return BuildInfo{Tag: "dev", Commit: "unknown", Date: "unknown", Source: "local", DefaultSignProvider: signProviderLocal}
}
