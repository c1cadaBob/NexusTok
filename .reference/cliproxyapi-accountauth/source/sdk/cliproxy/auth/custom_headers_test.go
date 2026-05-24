// auth - custom_headers_test.go
// 自定义请求头功能测试
// 验证从认证元数据中提取和应用自定义请求头的功能：
// - ExtractCustomHeadersFromMetadata：从 metadata 中提取 headers 映射
// - ApplyCustomHeadersFromMetadata：将自定义请求头应用到认证属性中
package auth

import (
	"reflect"
	"testing"
)

// TestExtractCustomHeadersFromMetadata 验证从元数据中提取自定义请求头：
// - 去除键名和值的首尾空格
// - 忽略空键名
// - 忽略纯空格的值
// - 将数值类型转换为字符串
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

// TestApplyCustomHeadersFromMetadata 验证应用自定义请求头：
// - 新的请求头覆盖已有的同名属性
// - 空值的请求头不被应用
// - 非请求头属性保持不变
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
