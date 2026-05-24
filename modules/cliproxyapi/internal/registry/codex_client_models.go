// registry - codex_client_models.go
// 本文件提供了 Codex 客户端模型目录的嵌入数据访问。
package registry

import _ "embed"

//go:embed models/codex_client_models.json
var codexClientModelsJSON []byte

// GetCodexClientModelsJSON 返回嵌入的 Codex 客户端模型目录数据的副本。
func GetCodexClientModelsJSON() []byte {
	return append([]byte(nil), codexClientModelsJSON...)
}
