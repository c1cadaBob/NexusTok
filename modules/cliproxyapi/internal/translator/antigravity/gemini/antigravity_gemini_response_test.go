// Package gemini - antigravity_gemini_response_test.go
// 测试 Antigravity 到 Gemini 响应格式转换功能。
// 覆盖 usageMetadata 字段恢复（cpaUsageMetadata -> usageMetadata）、
// 非流式和流式响应的转换测试用例。
package gemini

import (
	"context"
	"testing"
)

// TestRestoreUsageMetadata 测试将 cpaUsageMetadata 字段重命名为 usageMetadata，
// 以及无该字段和空输入时的处理
func TestRestoreUsageMetadata(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "cpaUsageMetadata renamed to usageMetadata",
			input:    []byte(`{"modelVersion":"gemini-3-pro","cpaUsageMetadata":{"promptTokenCount":100,"candidatesTokenCount":200}}`),
			expected: `{"modelVersion":"gemini-3-pro","usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":200}}`,
		},
		{
			name:     "no cpaUsageMetadata unchanged",
			input:    []byte(`{"modelVersion":"gemini-3-pro","usageMetadata":{"promptTokenCount":100}}`),
			expected: `{"modelVersion":"gemini-3-pro","usageMetadata":{"promptTokenCount":100}}`,
		},
		{
			name:     "empty input",
			input:    []byte(`{}`),
			expected: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := restoreUsageMetadata(tt.input)
			if string(result) != tt.expected {
				t.Errorf("restoreUsageMetadata() = %s, want %s", string(result), tt.expected)
			}
		})
	}
}

// TestConvertAntigravityResponseToGeminiNonStream 测试非流式响应中
// cpaUsageMetadata 的恢复和 usageMetadata 的保留
func TestConvertAntigravityResponseToGeminiNonStream(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "cpaUsageMetadata restored in response",
			input:    []byte(`{"response":{"modelVersion":"gemini-3-pro","cpaUsageMetadata":{"promptTokenCount":100}}}`),
			expected: `{"modelVersion":"gemini-3-pro","usageMetadata":{"promptTokenCount":100}}`,
		},
		{
			name:     "usageMetadata preserved",
			input:    []byte(`{"response":{"modelVersion":"gemini-3-pro","usageMetadata":{"promptTokenCount":100}}}`),
			expected: `{"modelVersion":"gemini-3-pro","usageMetadata":{"promptTokenCount":100}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertAntigravityResponseToGeminiNonStream(context.Background(), "", nil, nil, tt.input, nil)
			if string(result) != tt.expected {
				t.Errorf("ConvertAntigravityResponseToGeminiNonStream() = %s, want %s", string(result), tt.expected)
			}
		})
	}
}

// TestConvertAntigravityResponseToGeminiStream 测试流式响应中
// cpaUsageMetadata 的恢复
func TestConvertAntigravityResponseToGeminiStream(t *testing.T) {
	ctx := context.WithValue(context.Background(), "alt", "")

	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "cpaUsageMetadata restored in streaming response",
			input:    []byte(`data: {"response":{"modelVersion":"gemini-3-pro","cpaUsageMetadata":{"promptTokenCount":100}}}`),
			expected: `{"modelVersion":"gemini-3-pro","usageMetadata":{"promptTokenCount":100}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := ConvertAntigravityResponseToGemini(ctx, "", nil, nil, tt.input, nil)
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}
			if string(results[0]) != tt.expected {
				t.Errorf("ConvertAntigravityResponseToGemini() = %s, want %s", string(results[0]), tt.expected)
			}
		})
	}
}
