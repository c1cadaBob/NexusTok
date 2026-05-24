// config - disable_image_generation_mode_test.go
// 该文件测试禁用图片生成模式（DisableImageGenerationMode）的 YAML 和 JSON 反序列化。
// 测试覆盖了 false→Off、true→All、"chat"→Chat 的转换，以及 JSON 格式的相同行为。

package config

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestDisableImageGenerationMode_UnmarshalYAML 测试 YAML 反序列化：false→Off、true→All、"chat"→Chat。
func TestDisableImageGenerationMode_UnmarshalYAML(t *testing.T) {
	type wrapper struct {
		V DisableImageGenerationMode `yaml:"disable-image-generation"`
	}

	{
		var w wrapper
		if err := yaml.Unmarshal([]byte("disable-image-generation: false\n"), &w); err != nil {
			t.Fatalf("unmarshal false: %v", err)
		}
		if w.V != DisableImageGenerationOff {
			t.Fatalf("false => %v, want %v", w.V, DisableImageGenerationOff)
		}
	}

	{
		var w wrapper
		if err := yaml.Unmarshal([]byte("disable-image-generation: true\n"), &w); err != nil {
			t.Fatalf("unmarshal true: %v", err)
		}
		if w.V != DisableImageGenerationAll {
			t.Fatalf("true => %v, want %v", w.V, DisableImageGenerationAll)
		}
	}

	{
		var w wrapper
		if err := yaml.Unmarshal([]byte("disable-image-generation: chat\n"), &w); err != nil {
			t.Fatalf("unmarshal chat: %v", err)
		}
		if w.V != DisableImageGenerationChat {
			t.Fatalf("chat => %v, want %v", w.V, DisableImageGenerationChat)
		}
	}
}

// TestDisableImageGenerationMode_UnmarshalJSON 测试 JSON 反序列化：false→Off、true→All、"chat"→Chat。
func TestDisableImageGenerationMode_UnmarshalJSON(t *testing.T) {
	{
		var v DisableImageGenerationMode
		if err := json.Unmarshal([]byte("false"), &v); err != nil {
			t.Fatalf("unmarshal false: %v", err)
		}
		if v != DisableImageGenerationOff {
			t.Fatalf("false => %v, want %v", v, DisableImageGenerationOff)
		}
	}

	{
		var v DisableImageGenerationMode
		if err := json.Unmarshal([]byte("true"), &v); err != nil {
			t.Fatalf("unmarshal true: %v", err)
		}
		if v != DisableImageGenerationAll {
			t.Fatalf("true => %v, want %v", v, DisableImageGenerationAll)
		}
	}

	{
		var v DisableImageGenerationMode
		if err := json.Unmarshal([]byte(`"chat"`), &v); err != nil {
			t.Fatalf("unmarshal chat: %v", err)
		}
		if v != DisableImageGenerationChat {
			t.Fatalf("chat => %v, want %v", v, DisableImageGenerationChat)
		}
	}
}
