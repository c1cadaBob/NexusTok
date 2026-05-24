// 包 config - disable_image_generation_mode.go
// 该文件定义了禁用图像生成的三态配置模式。
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// DisableImageGenerationMode 是 disable-image-generation 的三态配置值。
//
// 支持：
//   - false: 启用
//   - true: 在所有地方禁用（包括 /v1/images/* 端点）
//   - "chat": 在所有非图像端点禁用，但在 /v1/images/generations 和 /v1/images/edits 启用
type DisableImageGenerationMode int

const (
	// DisableImageGenerationOff 表示图像生成功能启用
	DisableImageGenerationOff DisableImageGenerationMode = iota
	// DisableImageGenerationAll 表示在所有端点禁用图像生成
	DisableImageGenerationAll
	// DisableImageGenerationChat 表示仅在非图像端点禁用图像生成
	DisableImageGenerationChat
)

// String 返回 DisableImageGenerationMode 的字符串表示。
func (m DisableImageGenerationMode) String() string {
	switch m {
	case DisableImageGenerationOff:
		return "false"
	case DisableImageGenerationAll:
		return "true"
	case DisableImageGenerationChat:
		return "chat"
	default:
		return "false"
	}
}

// MarshalYAML 实现 yaml.Marshaler 接口，将模式序列化为 YAML 值。
func (m DisableImageGenerationMode) MarshalYAML() (any, error) {
	switch m {
	case DisableImageGenerationAll:
		return true, nil
	case DisableImageGenerationChat:
		return "chat", nil
	default:
		return false, nil
	}
}

// UnmarshalYAML 实现 yaml.Unmarshaler 接口，从 YAML 节点反序列化模式。
func (m *DisableImageGenerationMode) UnmarshalYAML(value *yaml.Node) error {
	mode, err := parseDisableImageGenerationNode(value)
	if err != nil {
		return err
	}
	*m = mode
	return nil
}

// MarshalJSON 实现 json.Marshaler 接口，将模式序列化为 JSON 值。
func (m DisableImageGenerationMode) MarshalJSON() ([]byte, error) {
	switch m {
	case DisableImageGenerationAll:
		return []byte("true"), nil
	case DisableImageGenerationChat:
		return json.Marshal("chat")
	default:
		return []byte("false"), nil
	}
}

// UnmarshalJSON 实现 json.Unmarshaler 接口，从 JSON 数据反序列化模式。
func (m *DisableImageGenerationMode) UnmarshalJSON(data []byte) error {
	mode, err := parseDisableImageGenerationJSON(data)
	if err != nil {
		return err
	}
	*m = mode
	return nil
}

// parseDisableImageGenerationNode 从 YAML 节点解析禁用图像生成模式。
func parseDisableImageGenerationNode(value *yaml.Node) (DisableImageGenerationMode, error) {
	if value == nil {
		return DisableImageGenerationOff, nil
	}

	// 首先尝试类型化布尔解码（覆盖未引用的 true/false 和 YAML 1.1 布尔值）。
	var b bool
	if err := value.Decode(&b); err == nil && value.Kind == yaml.ScalarNode && value.ShortTag() == "!!bool" {
		if b {
			return DisableImageGenerationAll, nil
		}
		return DisableImageGenerationOff, nil
	}

	// 回退到字符串解码（覆盖带引号的 "true"/"false" 和 "chat"）。
	var s string
	if err := value.Decode(&s); err != nil {
		return DisableImageGenerationOff, fmt.Errorf("invalid disable-image-generation value")
	}
	return parseDisableImageGenerationString(s)
}

// parseDisableImageGenerationJSON 从 JSON 数据解析禁用图像生成模式。
func parseDisableImageGenerationJSON(data []byte) (DisableImageGenerationMode, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return DisableImageGenerationOff, nil
	}

	// bool
	var b bool
	if err := json.Unmarshal(trimmed, &b); err == nil {
		if b {
			return DisableImageGenerationAll, nil
		}
		return DisableImageGenerationOff, nil
	}

	// string
	var s string
	if err := json.Unmarshal(trimmed, &s); err != nil {
		return DisableImageGenerationOff, fmt.Errorf("invalid disable-image-generation value")
	}
	return parseDisableImageGenerationString(s)
}

// parseDisableImageGenerationString 从字符串解析禁用图像生成模式。
func parseDisableImageGenerationString(s string) (DisableImageGenerationMode, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "", "false", "0", "off", "no":
		return DisableImageGenerationOff, nil
	case "true", "1", "on", "yes":
		return DisableImageGenerationAll, nil
	case "chat":
		return DisableImageGenerationChat, nil
	default:
		return DisableImageGenerationOff, fmt.Errorf("invalid disable-image-generation value %q (allowed: true, false, chat)", s)
	}
}
