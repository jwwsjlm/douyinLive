package main

import "fmt"

var (
	buildTag    = "dev"
	buildCommit = "unknown"
	buildDate   = "unknown"
)

func VersionString() string {
	return fmt.Sprintf("tag=%s commit=%s buildDate=%s", buildTag, buildCommit, buildDate)
}
