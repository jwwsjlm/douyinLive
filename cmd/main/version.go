package main

import "fmt"

var (
	buildTag    = "dev"
	buildCommit = "unknown"
	buildDate   = "unknown"
	buildSource = "local"

	defaultSignProvider = "local"
)

func VersionString() string {
	return fmt.Sprintf("tag=%s commit=%s buildDate=%s source=%s signProvider=%s", buildTag, buildCommit, buildDate, buildSource, defaultSignProvider)
}
