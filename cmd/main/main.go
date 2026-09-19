package main

import (
	"os"

	"github.com/jwwsjlm/douyinLive/v2/internal/server"
)

var (
	// Release builds inject these values with -X main.<name>.
	buildTag            = "dev"
	buildCommit         = "unknown"
	buildDate           = "unknown"
	buildSource         = "local"
	defaultSignProvider = "local"
)

func main() {
	os.Exit(server.Run(buildInfo()))
}

func buildInfo() server.BuildInfo {
	return server.BuildInfo{
		Tag:                 buildTag,
		Commit:              buildCommit,
		Date:                buildDate,
		Source:              buildSource,
		DefaultSignProvider: defaultSignProvider,
	}
}
