// 本文件测试错误状态码重映射功能（ResetStatusCode），验证：
// - 支持字符串和整数两种格式的状态码映射值
// - 无效的字符串映射值会被跳过，保持原状态码不变
// - 状态码 200 不参与重映射（始终保持原值）
package service

import (
	"testing"

	"github.com/c1cada/NexusTok/types"
	"github.com/stretchr/testify/require"
)

// TestResetStatusCode 测试 ResetStatusCode 函数的各种场景，
// 包括字符串映射、整数映射、无效映射值以及 200 状态码的跳过逻辑。
func TestResetStatusCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		statusCode       int
		statusCodeConfig string
		expectedCode     int
	}{
		{
			name:             "map string value",       // 测试映射值为字符串格式（如 "503"）
			statusCode:       429,
			statusCodeConfig: `{"429":"503"}`,
			expectedCode:     503,
		},
		{
			name:             "map int value",           // 测试映射值为整数格式（如 503）
			statusCode:       429,
			statusCodeConfig: `{"429":503}`,
			expectedCode:     503,
		},
		{
			name:             "skip invalid string value", // 测试无效字符串映射值被跳过
			statusCode:       429,
			statusCodeConfig: `{"429":"bad-code"}`,
			expectedCode:     429,
		},
		{
			name:             "skip status code 200",    // 测试 200 状态码不参与重映射
			statusCode:       200,
			statusCodeConfig: `{"200":503}`,
			expectedCode:     200,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			newAPIError := &types.NexusTokError{
				StatusCode: tc.statusCode,
			}
			ResetStatusCode(newAPIError, tc.statusCodeConfig)
			require.Equal(t, tc.expectedCode, newAPIError.StatusCode) // 验证状态码重映射结果
		})
	}
}
