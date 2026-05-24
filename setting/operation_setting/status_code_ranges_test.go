// status_code_ranges_test.go — HTTP 状态码范围解析与匹配的单元测试
// 职责：测试状态码范围的解析（包括逗号分隔、合并归一化、无效输入检测）
// 以及禁用/重试判断函数的正确性。

package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseHTTPStatusCodeRanges_CommaSeparated 验证逗号分隔的状态码解析
// 包括单个状态码和范围格式的混合使用
func TestParseHTTPStatusCodeRanges_CommaSeparated(t *testing.T) {
	ranges, err := ParseHTTPStatusCodeRanges("401,403,500-599")
	require.NoError(t, err)
	require.Equal(t, []StatusCodeRange{
		{Start: 401, End: 401},
		{Start: 403, End: 403},
		{Start: 500, End: 599},
	}, ranges)
}

// TestParseHTTPStatusCodeRanges_MergeAndNormalize 验证解析后的排序和合并功能
// 相邻或重叠的范围应被自动合并（如 401-403 和 402 合并为 401-403）
func TestParseHTTPStatusCodeRanges_MergeAndNormalize(t *testing.T) {
	ranges, err := ParseHTTPStatusCodeRanges("500-505,504,401,403,402")
	require.NoError(t, err)
	require.Equal(t, []StatusCodeRange{
		{Start: 401, End: 403},
		{Start: 500, End: 505},
	}, ranges)
}

// TestParseHTTPStatusCodeRanges_Invalid 验证各种无效输入的错误检测
func TestParseHTTPStatusCodeRanges_Invalid(t *testing.T) {
	_, err := ParseHTTPStatusCodeRanges("99,600,foo,500-400,500-")
	require.Error(t, err)
}

// TestParseHTTPStatusCodeRanges_NoComma_IsInvalid 验证非逗号分隔的输入被正确拒绝
func TestParseHTTPStatusCodeRanges_NoComma_IsInvalid(t *testing.T) {
	_, err := ParseHTTPStatusCodeRanges("401 403")
	require.Error(t, err)
}

// TestShouldDisableByStatusCode 验证自动禁用状态码匹配逻辑
func TestShouldDisableByStatusCode(t *testing.T) {
	// 保存原始配置，测试结束后恢复
	orig := AutomaticDisableStatusCodeRanges
	t.Cleanup(func() { AutomaticDisableStatusCodeRanges = orig })

	AutomaticDisableStatusCodeRanges = []StatusCodeRange{
		{Start: 401, End: 403},
		{Start: 500, End: 599},
	}

	require.True(t, ShouldDisableByStatusCode(401))
	require.True(t, ShouldDisableByStatusCode(403))
	require.False(t, ShouldDisableByStatusCode(404))
	require.True(t, ShouldDisableByStatusCode(500))
	require.False(t, ShouldDisableByStatusCode(200))
}

// TestShouldRetryByStatusCode 验证自动重试状态码匹配逻辑
func TestShouldRetryByStatusCode(t *testing.T) {
	orig := AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { AutomaticRetryStatusCodeRanges = orig })

	AutomaticRetryStatusCodeRanges = []StatusCodeRange{
		{Start: 429, End: 429},
		{Start: 500, End: 599},
	}

	require.True(t, ShouldRetryByStatusCode(429))
	require.True(t, ShouldRetryByStatusCode(500))
	require.False(t, ShouldRetryByStatusCode(504)) // 始终跳过重试
	require.False(t, ShouldRetryByStatusCode(524)) // 始终跳过重试
	require.False(t, ShouldRetryByStatusCode(400))
	require.False(t, ShouldRetryByStatusCode(200))
}

// TestShouldRetryByStatusCode_DefaultMatchesLegacyBehavior 验证默认配置与遗留行为一致
func TestShouldRetryByStatusCode_DefaultMatchesLegacyBehavior(t *testing.T) {
	require.False(t, ShouldRetryByStatusCode(200))
	require.False(t, ShouldRetryByStatusCode(400))
	require.True(t, ShouldRetryByStatusCode(401))
	require.False(t, ShouldRetryByStatusCode(408))
	require.True(t, ShouldRetryByStatusCode(429))
	require.True(t, ShouldRetryByStatusCode(500))
	require.False(t, ShouldRetryByStatusCode(504))
	require.False(t, ShouldRetryByStatusCode(524))
	require.True(t, ShouldRetryByStatusCode(599))
}

// TestIsAlwaysSkipRetryStatusCode 验证始终跳过重试的状态码判断
func TestIsAlwaysSkipRetryStatusCode(t *testing.T) {
	require.True(t, IsAlwaysSkipRetryStatusCode(504))
	require.True(t, IsAlwaysSkipRetryStatusCode(524))
	require.False(t, IsAlwaysSkipRetryStatusCode(500))
}
