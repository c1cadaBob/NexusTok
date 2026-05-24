// Package buildinfo 暴露编译时的元数据，在服务器各模块间共享。
// 这些变量通过 ldflags 在发布构建时覆盖，本地开发使用默认值。
//
// Package buildinfo exposes compile-time metadata shared across the server.
package buildinfo

// 以下变量通过 ldflags 在发布构建时覆盖，默认值适用于本地开发构建。
// The following variables are overridden via ldflags during release builds.
var (
	// Version 是二进制文件的语义版本号或 git describe 输出。
	// Version is the semantic version or git describe output of the binary.
	Version = "dev"

	// Commit 是嵌入二进制文件的 git commit SHA。
	// Commit is the git commit SHA baked into the binary.
	Commit = "none"

	// BuildDate 记录二进制文件的构建时间（UTC 格式）。
	// BuildDate records when the binary was built in UTC.
	BuildDate = "unknown"
)
