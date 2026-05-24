// 包 misc - credentials.go
// 该文件提供了凭据保存日志和元数据合并的辅助功能。
package misc

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

// credentialSeparator 是用于在日志中视觉分组相关凭据处理行的分隔符。
var credentialSeparator = strings.Repeat("-", 67)

// LogSavingCredentials 输出保存凭据时的一致日志消息。
//
// 参数：
//   - path: 凭据保存路径
func LogSavingCredentials(path string) {
	if path == "" {
		return
	}
	// Use filepath.Clean so logs remain stable even if callers pass redundant separators.
	fmt.Printf("Saving credentials to %s\n", filepath.Clean(path))
}

// LogCredentialSeparator 在日志中添加视觉分隔符，用于分组认证/密钥处理日志。
func LogCredentialSeparator() {
	log.Debug(credentialSeparator)
}

// MergeMetadata 将源结构体序列化为 map 并与提供的元数据合并。
// 如果源已经是 map 类型则直接拷贝，否则通过 JSON 序列化/反序列化转换。
//
// 参数：
//   - source: 源数据（结构体或 map）
//   - metadata: 要合并的额外元数据
//
// 返回：
//   - map[string]any: 合并后的数据
//   - error: 序列化失败时返回错误
func MergeMetadata(source any, metadata map[string]any) (map[string]any, error) {
	var data map[string]any

	// Fast path: if source is already a map, just copy it to avoid mutation of original
	if srcMap, ok := source.(map[string]any); ok {
		data = make(map[string]any, len(srcMap)+len(metadata))
		for k, v := range srcMap {
			data[k] = v
		}
	} else {
		// Slow path: marshal to JSON and back to map to respect JSON tags
		temp, err := json.Marshal(source)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal source: %w", err)
		}
		if err := json.Unmarshal(temp, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
		}
	}

	// Merge extra metadata
	if metadata != nil {
		if data == nil {
			data = make(map[string]any)
		}
		for k, v := range metadata {
			data[k] = v
		}
	}

	return data, nil
}
