// synthesizer - context.go
// 本文件定义了认证合成所需的上下文结构。
// 包含配置、认证目录、时间和 ID 生成器等合成所需的信息。
package synthesizer

import (
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

// SynthesisContext 提供认证合成所需的上下文信息。
type SynthesisContext struct {
	// Config 是当前配置
	Config *config.Config
	// AuthDir 是包含认证文件的目录
	AuthDir string
	// Now 是用于时间戳的当前时间
	Now time.Time
	// IDGenerator 为认证条目生成稳定的 ID
	IDGenerator *StableIDGenerator
}
