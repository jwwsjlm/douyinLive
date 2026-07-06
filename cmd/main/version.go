package main

import "fmt"

var (
	// buildTag 由发布流程注入，表示当前构建对应的 tag。
	// buildTag is injected by the release flow and identifies the build tag.
	buildTag = "dev"
	// buildCommit 由发布流程注入，表示当前构建对应的提交 hash。
	// buildCommit is injected by the release flow and identifies the commit hash.
	buildCommit = "unknown"
	// buildDate 由发布流程注入，表示当前构建时间。
	// buildDate is injected by the release flow and identifies the build time.
	buildDate = "unknown"
	// buildSource 由发布流程注入，表示构建来源。
	// buildSource is injected by the release flow and identifies the build source.
	buildSource = "local"

	// defaultSignProvider 由发布流程注入，表示默认签名实现。
	// defaultSignProvider is injected by the release flow and identifies the default signer.
	defaultSignProvider = "local"
)

// VersionString 返回当前构建版本信息。
// VersionString returns the current build version information.
func VersionString() string {
	return fmt.Sprintf("tag=%s commit=%s buildDate=%s source=%s signProvider=%s", buildTag, buildCommit, buildDate, buildSource, defaultSignProvider)
}
