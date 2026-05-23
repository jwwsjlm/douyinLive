package main

import "fmt"

var (
	buildTag    = "dev"
	buildCommit = "unknown"
	buildDate   = "unknown"
	buildSource = "local"
)

func VersionString() string {
	return fmt.Sprintf("tag=%s commit=%s buildDate=%s source=%s", buildTag, buildCommit, buildDate, buildSource)
}
