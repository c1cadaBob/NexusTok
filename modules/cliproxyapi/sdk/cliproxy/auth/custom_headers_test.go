// auth - custom_headers_test.go
// 该文件包含自定义头提取和应用函数的单元测试，验证从元数据中提取头信息、
// 过滤空值和非字符串值、以及应用到 Auth Attributes 的正确性。
package auth

import (
	"reflect"
	"testing"
)

// TestExtractCustomHeadersFromMetadata 测试从元数据中提取自定义头的逻辑，
// 验证键值去空白、过滤空键和空值、忽略非字符串类型值。
func TestExtractCustomHeadersFromMetadata(t *testing.T) {
	meta := map[string]any{
		"headers": map[string]any{
			" X-Test ": " value ",
			"":         "ignored",
			"X-Empty":  "   ",
			"X-Num":    float64(1),
		},
	}

	got := ExtractCustomHeadersFromMetadata(meta)
	want := map[string]string{"X-Test": "value"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractCustomHeadersFromMetadata() = %#v, want %#v", got, want)
	}
}

// TestApplyCustomHeadersFromMetadata 测试将元数据中的自定义头应用到 Auth 的 Attributes 中，
// 验证新值覆盖旧值、空值不被应用、已有非头属性不被影响。
func TestApplyCustomHeadersFromMetadata(t *testing.T) {
	auth := &Auth{
		Metadata: map[string]any{
			"headers": map[string]string{
				"X-Test":  "new",
				"X-Empty": "   ",
			},
		},
		Attributes: map[string]string{
			"header:X-Test": "old",
			"keep":          "1",
		},
	}

	ApplyCustomHeadersFromMetadata(auth)

	if got := auth.Attributes["header:X-Test"]; got != "new" {
		t.Fatalf("header:X-Test = %q, want %q", got, "new")
	}
	if _, ok := auth.Attributes["header:X-Empty"]; ok {
		t.Fatalf("expected header:X-Empty to be absent, got %#v", auth.Attributes["header:X-Empty"])
	}
	if got := auth.Attributes["keep"]; got != "1" {
		t.Fatalf("keep = %q, want %q", got, "1")
	}
}
