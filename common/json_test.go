package common

// 本文件测试 JSON 工具函数 JsonRawMessageToString 的功能。
// 该函数负责将 json.RawMessage 转换为普通字符串，处理对象、嵌套字符串、null 和空值等多种场景。

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestJsonRawMessageToString 测试 JsonRawMessageToString 函数对不同类型输入的处理能力。
// 覆盖场景包括：JSON 对象、被引号包裹的 JSON 字符串、null 值和 nil 输入。
func TestJsonRawMessageToString(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
		want string
	}{
		{
			name: "object",
			data: json.RawMessage(`{"city":"Paris","days":0,"strict":false}`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "string",
			data: json.RawMessage(`"{\"city\":\"Paris\",\"days\":0,\"strict\":false}"`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "null",
			data: json.RawMessage(`null`),
			want: "",
		},
		{
			name: "empty",
			data: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 验证各种输入均能正确转换为目标字符串
			require.Equal(t, tt.want, JsonRawMessageToString(tt.data))
		})
	}
}
