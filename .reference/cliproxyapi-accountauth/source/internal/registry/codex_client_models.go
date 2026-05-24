// 包 registry - codex_client_models.go
// 该文件嵌入了 Codex 客户端模型目录的 JSON 数据。
package registry

import _ "embed"

//go:embed models/codex_client_models.json
// codexClientModelsJSON 是嵌入的 Codex 客户端模型目录 JSON 数据。
var codexClientModelsJSON []byte

// GetCodexClientModelsJSON 返回嵌入的 Codex 客户端模型目录的副本。
// 返回的是深拷贝，修改返回值不会影响原始嵌入数据。
func GetCodexClientModelsJSON() []byte {
	return append([]byte(nil), codexClientModelsJSON...)
}
