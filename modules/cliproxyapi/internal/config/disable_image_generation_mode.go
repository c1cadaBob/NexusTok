// Package config - disable_image_generation_mode.go
// 定义图像生成功能的三态配置模式，支持完全禁用、仅聊天端点禁用和完全启用三种状态。
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// DisableImageGenerationMode 是 disable-image-generation 配置的三态值类型。
//
// 支持以下值:
//   - false: 启用图像生成（默认）
//   - true: 完全禁用图像生成（包括 /v1/images/* 端点）
//   - "chat": 仅对非图像端点禁用，但保留 /v1/images/generations 和 /v1/images/edits 端点
//
// DisableImageGenerationMode is a tri-state config value for disable-image-generation.
type DisableImageGenerationMode int

const (
	// DisableImageGenerationOff 表示图像生成功能完全启用。
	DisableImageGenerationOff DisableImageGenerationMode = iota
	// DisableImageGenerationAll 表示图像生成功能完全禁用。
	DisableImageGenerationAll
	// DisableImageGenerationChat 表示仅在聊天端点禁用图像生成。
	DisableImageGenerationChat
)

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

func (m *DisableImageGenerationMode) UnmarshalYAML(value *yaml.Node) error {
	mode, err := parseDisableImageGenerationNode(value)
	if err != nil {
		return err
	}
	*m = mode
	return nil
}

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

func (m *DisableImageGenerationMode) UnmarshalJSON(data []byte) error {
	mode, err := parseDisableImageGenerationJSON(data)
	if err != nil {
		return err
	}
	*m = mode
	return nil
}

func parseDisableImageGenerationNode(value *yaml.Node) (DisableImageGenerationMode, error) {
	if value == nil {
		return DisableImageGenerationOff, nil
	}

	// First try a typed bool decode (covers unquoted true/false and YAML 1.1 bools).
	var b bool
	if err := value.Decode(&b); err == nil && value.Kind == yaml.ScalarNode && value.ShortTag() == "!!bool" {
		if b {
			return DisableImageGenerationAll, nil
		}
		return DisableImageGenerationOff, nil
	}

	// Fall back to string decoding (covers quoted "true"/"false" and "chat").
	var s string
	if err := value.Decode(&s); err != nil {
		return DisableImageGenerationOff, fmt.Errorf("invalid disable-image-generation value")
	}
	return parseDisableImageGenerationString(s)
}

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
