// 包 xai - apply.go
// 该文件实现了 xAI Grok Responses API 模型的思考配置应用器。
// xAI 模型使用 OpenAI Responses API 兼容的 reasoning.effort 格式，支持离散级别。
// 通过嵌入 codex.Applier 复用 Codex 的实现逻辑。
package xai

import (
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/codex"
)

// Applier implements thinking.ProviderApplier for xAI models.
type Applier struct {
	codex.Applier
}

var _ thinking.ProviderApplier = (*Applier)(nil)

// NewApplier creates a new xAI thinking applier.
func NewApplier() *Applier {
	return &Applier{}
}

func init() {
	thinking.RegisterProvider("xai", NewApplier())
}
