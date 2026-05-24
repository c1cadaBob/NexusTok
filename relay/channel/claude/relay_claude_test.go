// Package claude - relay_claude_test.go
//
// 本文件包含 Claude 渠道中继适配器的单元测试，覆盖以下核心功能：
//
//  1. FormatClaudeResponseInfo 测试：
//     - message_start 事件的 usage 提取
//     - message_delta 事件的 usage 合并（完整字段和仅 output_tokens 场景）
//     - nil claudeInfo 的边界处理
//     - content_block_delta 事件的文本累积
//
//  2. buildOpenAIStyleUsageFromClaudeUsage 测试：
//     - Claude usage 到 OpenAI 风格 usage 的转换
//     - 缓存创建 token 分拆（5m/1h）的余数保留
//     - 聚合缓存创建 token 缺失时的回退逻辑
//
//  3. RequestOpenAI2ClaudeMessage 测试：
//     - 不支持的文件类型（非 PDF）的过滤
//     - PDF 文件转换为 document 内容块
//     - 纯文本文件（.txt）转换为 text 内容块
package claude

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/dto"
	"github.com/stretchr/testify/require"
)

// TestFormatClaudeResponseInfo_MessageStart 测试 message_start 事件的 usage 提取。
//
// 验证 FormatClaudeResponseInfo 能正确处理 message_start 事件：
//   - 从 message.usage 中提取 input_tokens 到 PromptTokens
//   - 从 message.usage 中提取 cache_read_input_tokens 到 CachedTokens
//   - 从 message.usage 中提取 cache_creation_input_tokens 到 CachedCreationTokens
//   - 从 message 中提取 id 到 ResponseId
//   - 从 message 中提取 model 到 Model
func TestFormatClaudeResponseInfo_MessageStart(t *testing.T) {
	claudeInfo := &ClaudeResponseInfo{
		Usage: &dto.Usage{},
	}
	claudeResponse := &dto.ClaudeResponse{
		Type: "message_start",
		Message: &dto.ClaudeMediaMessage{
			Id:    "msg_123",
			Model: "claude-3-5-sonnet",
			Usage: &dto.ClaudeUsage{
				InputTokens:              100,
				OutputTokens:             1,
				CacheCreationInputTokens: 50,
				CacheReadInputTokens:     30,
			},
		},
	}

	ok := FormatClaudeResponseInfo(claudeResponse, nil, claudeInfo)
	if !ok {
		t.Fatal("expected true")
	}
	if claudeInfo.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", claudeInfo.Usage.PromptTokens)
	}
	if claudeInfo.Usage.PromptTokensDetails.CachedTokens != 30 {
		t.Errorf("CachedTokens = %d, want 30", claudeInfo.Usage.PromptTokensDetails.CachedTokens)
	}
	if claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens != 50 {
		t.Errorf("CachedCreationTokens = %d, want 50", claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens)
	}
	if claudeInfo.ResponseId != "msg_123" {
		t.Errorf("ResponseId = %s, want msg_123", claudeInfo.ResponseId)
	}
	if claudeInfo.Model != "claude-3-5-sonnet" {
		t.Errorf("Model = %s, want claude-3-5-sonnet", claudeInfo.Model)
	}
}

// TestFormatClaudeResponseInfo_MessageDelta_FullUsage 测试 message_delta 事件携带完整 usage 时的合并逻辑。
//
// 模拟原生 Anthropic 场景：message_start 已积累初始 usage，
// message_delta 携带完整的 input_tokens 和 output_tokens。
// 验证：
//   - PromptTokens 被 message_delta 的 InputTokens 覆盖（非叠加）
//   - CompletionTokens 被 message_delta 的 OutputTokens 设置
//   - TotalTokens 正确计算为 PromptTokens + CompletionTokens
//   - Done 标记被设置为 true
func TestFormatClaudeResponseInfo_MessageDelta_FullUsage(t *testing.T) {
	// message_start 先积累 usage
	claudeInfo := &ClaudeResponseInfo{
		Usage: &dto.Usage{
			PromptTokens: 100,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         30,
				CachedCreationTokens: 50,
			},
			CompletionTokens: 1,
		},
	}

	// message_delta 带完整 usage（原生 Anthropic 场景）
	claudeResponse := &dto.ClaudeResponse{
		Type: "message_delta",
		Usage: &dto.ClaudeUsage{
			InputTokens:              100,
			OutputTokens:             200,
			CacheCreationInputTokens: 50,
			CacheReadInputTokens:     30,
		},
	}

	ok := FormatClaudeResponseInfo(claudeResponse, nil, claudeInfo)
	if !ok {
		t.Fatal("expected true")
	}
	if claudeInfo.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", claudeInfo.Usage.PromptTokens)
	}
	if claudeInfo.Usage.CompletionTokens != 200 {
		t.Errorf("CompletionTokens = %d, want 200", claudeInfo.Usage.CompletionTokens)
	}
	if claudeInfo.Usage.TotalTokens != 300 {
		t.Errorf("TotalTokens = %d, want 300", claudeInfo.Usage.TotalTokens)
	}
	if !claudeInfo.Done {
		t.Error("expected Done = true")
	}
}

// TestFormatClaudeResponseInfo_MessageDelta_OnlyOutputTokens 测试 message_delta 仅包含 output_tokens 的场景。
//
// 模拟 AWS Bedrock 场景：message_start 已积累完整 usage（含 cache 字段），
// 但 message_delta 仅返回 output_tokens，缺少 input_tokens 和 cache 字段。
// 验证：
//   - PromptTokens 保持 message_start 的值（不被 message_delta 的 0 值覆盖）
//   - CompletionTokens 被 message_delta 的 OutputTokens 设置
//   - TotalTokens 正确计算
//   - cache 相关字段保持 message_start 的值（CachedTokens、CachedCreationTokens、
//     ClaudeCacheCreation5mTokens、ClaudeCacheCreation1hTokens）
//   - Done 标记被设置为 true
func TestFormatClaudeResponseInfo_MessageDelta_OnlyOutputTokens(t *testing.T) {
	// 模拟 Bedrock: message_start 已积累 usage
	claudeInfo := &ClaudeResponseInfo{
		Usage: &dto.Usage{
			PromptTokens: 100,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         30,
				CachedCreationTokens: 50,
			},
			CompletionTokens:            1,
			ClaudeCacheCreation5mTokens: 10,
			ClaudeCacheCreation1hTokens: 20,
		},
	}

	// Bedrock 的 message_delta 只有 output_tokens，缺少 input_tokens 和 cache 字段
	claudeResponse := &dto.ClaudeResponse{
		Type: "message_delta",
		Usage: &dto.ClaudeUsage{
			OutputTokens: 200,
			// InputTokens, CacheCreationInputTokens, CacheReadInputTokens 都是 0
		},
	}

	ok := FormatClaudeResponseInfo(claudeResponse, nil, claudeInfo)
	if !ok {
		t.Fatal("expected true")
	}
	// PromptTokens 应保持 message_start 的值（因为 message_delta 的 InputTokens=0，不更新）
	if claudeInfo.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", claudeInfo.Usage.PromptTokens)
	}
	if claudeInfo.Usage.CompletionTokens != 200 {
		t.Errorf("CompletionTokens = %d, want 200", claudeInfo.Usage.CompletionTokens)
	}
	if claudeInfo.Usage.TotalTokens != 300 {
		t.Errorf("TotalTokens = %d, want 300", claudeInfo.Usage.TotalTokens)
	}
	// cache 字段应保持 message_start 的值
	if claudeInfo.Usage.PromptTokensDetails.CachedTokens != 30 {
		t.Errorf("CachedTokens = %d, want 30", claudeInfo.Usage.PromptTokensDetails.CachedTokens)
	}
	if claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens != 50 {
		t.Errorf("CachedCreationTokens = %d, want 50", claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens)
	}
	if claudeInfo.Usage.ClaudeCacheCreation5mTokens != 10 {
		t.Errorf("ClaudeCacheCreation5mTokens = %d, want 10", claudeInfo.Usage.ClaudeCacheCreation5mTokens)
	}
	if claudeInfo.Usage.ClaudeCacheCreation1hTokens != 20 {
		t.Errorf("ClaudeCacheCreation1hTokens = %d, want 20", claudeInfo.Usage.ClaudeCacheCreation1hTokens)
	}
	if !claudeInfo.Done {
		t.Error("expected Done = true")
	}
}

// TestFormatClaudeResponseInfo_NilClaudeInfo 测试 claudeInfo 参数为 nil 时的边界处理。
//
// 验证当 claudeInfo 为 nil 时，函数返回 false，不引发 panic。
func TestFormatClaudeResponseInfo_NilClaudeInfo(t *testing.T) {
	claudeResponse := &dto.ClaudeResponse{Type: "message_start"}
	ok := FormatClaudeResponseInfo(claudeResponse, nil, nil)
	if ok {
		t.Error("expected false for nil claudeInfo")
	}
}

// TestFormatClaudeResponseInfo_ContentBlockDelta 测试 content_block_delta 事件的文本累积。
//
// 验证 FormatClaudeResponseInfo 能正确将 content_block_delta 中的 text 增量
// 追加到 claudeInfo.ResponseText 中。
func TestFormatClaudeResponseInfo_ContentBlockDelta(t *testing.T) {
	text := "hello"
	claudeInfo := &ClaudeResponseInfo{
		Usage:        &dto.Usage{},
		ResponseText: strings.Builder{},
	}
	claudeResponse := &dto.ClaudeResponse{
		Type: "content_block_delta",
		Delta: &dto.ClaudeMediaMessage{
			Text: &text,
		},
	}

	ok := FormatClaudeResponseInfo(claudeResponse, nil, claudeInfo)
	if !ok {
		t.Fatal("expected true")
	}
	if claudeInfo.ResponseText.String() != "hello" {
		t.Errorf("ResponseText = %q, want %q", claudeInfo.ResponseText.String(), "hello")
	}
}

// TestBuildOpenAIStyleUsageFromClaudeUsage 测试 Claude usage 到 OpenAI 风格 usage 的基本转换。
//
// 验证：
//   - PromptTokens 和 InputTokens 等于 prompt_tokens + cached_tokens + cache_creation_tokens（=180）
//   - TotalTokens 等于 totalInputTokens + completionTokens（=200）
//   - UsageSemantic 被设置为 "openai"
//   - UsageSource 被设置为 "anthropic"
func TestBuildOpenAIStyleUsageFromClaudeUsage(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 20,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         30,
			CachedCreationTokens: 50,
		},
		ClaudeCacheCreation5mTokens: 10,
		ClaudeCacheCreation1hTokens: 20,
		UsageSemantic:               "anthropic",
	}

	openAIUsage := buildOpenAIStyleUsageFromClaudeUsage(usage)

	if openAIUsage.PromptTokens != 180 {
		t.Fatalf("PromptTokens = %d, want 180", openAIUsage.PromptTokens)
	}
	if openAIUsage.InputTokens != 180 {
		t.Fatalf("InputTokens = %d, want 180", openAIUsage.InputTokens)
	}
	if openAIUsage.TotalTokens != 200 {
		t.Fatalf("TotalTokens = %d, want 200", openAIUsage.TotalTokens)
	}
	if openAIUsage.UsageSemantic != "openai" {
		t.Fatalf("UsageSemantic = %s, want openai", openAIUsage.UsageSemantic)
	}
	if openAIUsage.UsageSource != "anthropic" {
		t.Fatalf("UsageSource = %s, want anthropic", openAIUsage.UsageSource)
	}
}

// TestBuildOpenAIStyleUsageFromClaudeUsagePreservesCacheCreationRemainder 测试缓存创建 token 的余数保留逻辑。
//
// 测试两种场景：
//  1. 聚合缓存（CachedCreationTokens=50）包含分拆缓存（5m=10 + 1h=20=30）之外的余数（20），
//     此时使用聚合值（50），总输入 token = 100 + 30 + 50 = 180。
//  2. 聚合缓存为 0，回退到分拆缓存之和（30），总输入 token = 100 + 30 + 30 = 160。
func TestBuildOpenAIStyleUsageFromClaudeUsagePreservesCacheCreationRemainder(t *testing.T) {
	tests := []struct {
		name                    string
		cachedCreationTokens    int
		cacheCreationTokens5m   int
		cacheCreationTokens1h   int
		expectedTotalInputToken int
	}{
		{
			name:                    "prefers aggregate when it includes remainder",
			cachedCreationTokens:    50,
			cacheCreationTokens5m:   10,
			cacheCreationTokens1h:   20,
			expectedTotalInputToken: 180,
		},
		{
			name:                    "falls back to split tokens when aggregate missing",
			cachedCreationTokens:    0,
			cacheCreationTokens5m:   10,
			cacheCreationTokens1h:   20,
			expectedTotalInputToken: 160,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &dto.Usage{
				PromptTokens:     100,
				CompletionTokens: 20,
				PromptTokensDetails: dto.InputTokenDetails{
					CachedTokens:         30,
					CachedCreationTokens: tt.cachedCreationTokens,
				},
				ClaudeCacheCreation5mTokens: tt.cacheCreationTokens5m,
				ClaudeCacheCreation1hTokens: tt.cacheCreationTokens1h,
				UsageSemantic:               "anthropic",
			}

			openAIUsage := buildOpenAIStyleUsageFromClaudeUsage(usage)

			if openAIUsage.PromptTokens != tt.expectedTotalInputToken {
				t.Fatalf("PromptTokens = %d, want %d", openAIUsage.PromptTokens, tt.expectedTotalInputToken)
			}
			if openAIUsage.InputTokens != tt.expectedTotalInputToken {
				t.Fatalf("InputTokens = %d, want %d", openAIUsage.InputTokens, tt.expectedTotalInputToken)
			}
		})
	}
}

// TestBuildOpenAIStyleUsageFromClaudeUsageDefaultsAggregateCacheCreationTo5m 测试聚合缓存创建 token 的默认分拆行为。
//
// 当只有聚合缓存创建 token（CachedCreationTokens=50）而没有分拆值时，
// NormalizeCacheCreationSplit 应将聚合值默认分配到 5 分钟缓存（ClaudeCacheCreation5mTokens=50），
// 而 1 小时缓存为 0（ClaudeCacheCreation1hTokens=0）。
func TestBuildOpenAIStyleUsageFromClaudeUsageDefaultsAggregateCacheCreationTo5m(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 20,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         30,
			CachedCreationTokens: 50,
		},
		UsageSemantic: "anthropic",
	}

	openAIUsage := buildOpenAIStyleUsageFromClaudeUsage(usage)

	require.Equal(t, 50, openAIUsage.ClaudeCacheCreation5mTokens)
	require.Equal(t, 0, openAIUsage.ClaudeCacheCreation1hTokens)
}

// TestRequestOpenAI2ClaudeMessage_IgnoresUnsupportedFileContent 测试不支持的文件类型过滤。
//
// 验证当消息中包含非 PDF 文件（如 blob.bin）时，
// RequestOpenAI2ClaudeMessage 会忽略该文件内容，只保留文本部分。
// 结果中应只有 1 个 text 类型的内容块（"see attachment"）。
func TestRequestOpenAI2ClaudeMessage_IgnoresUnsupportedFileContent(t *testing.T) {
	request := dto.GeneralOpenAIRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.Message{
			{
				Role: "user",
				Content: []any{
					dto.MediaContent{
						Type: dto.ContentTypeText,
						Text: "see attachment",
					},
					dto.MediaContent{
						Type: dto.ContentTypeFile,
						File: &dto.MessageFile{
							FileName: "blob.bin",
							FileData: "JVBERi0xLjQK",
						},
					},
				},
			},
		},
	}

	claudeRequest, err := RequestOpenAI2ClaudeMessage(nil, request)
	require.NoError(t, err)
	require.Len(t, claudeRequest.Messages, 1)

	content, ok := claudeRequest.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, content, 1)
	require.Equal(t, "text", content[0].Type)
	require.NotNil(t, content[0].Text)
	require.Equal(t, "see attachment", *content[0].Text)
}

// TestRequestOpenAI2ClaudeMessage_SupportsPDFFileContent 测试 PDF 文件的转换。
//
// 验证当消息中包含 PDF 文件（spec.pdf）时，
// RequestOpenAI2ClaudeMessage 会将其转换为 Claude 的 document 类型内容块：
//   - type: "document"
//   - source.type: "base64"
//   - source.media_type: "application/pdf"
//   - source.data: 原始 base64 数据
//
// 同时验证文本内容（"summarize it"）被正确保留为 text 类型内容块。
func TestRequestOpenAI2ClaudeMessage_SupportsPDFFileContent(t *testing.T) {
	request := dto.GeneralOpenAIRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.Message{
			{
				Role: "user",
				Content: []any{
					dto.MediaContent{
						Type: dto.ContentTypeFile,
						File: &dto.MessageFile{
							FileName: "spec.pdf",
							FileData: "JVBERi0xLjQK",
						},
					},
					dto.MediaContent{
						Type: dto.ContentTypeText,
						Text: "summarize it",
					},
				},
			},
		},
	}

	claudeRequest, err := RequestOpenAI2ClaudeMessage(nil, request)
	require.NoError(t, err)
	require.Len(t, claudeRequest.Messages, 1)

	content, ok := claudeRequest.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, content, 2)
	require.Equal(t, "document", content[0].Type)
	require.NotNil(t, content[0].Source)
	require.Equal(t, "base64", content[0].Source.Type)
	require.Equal(t, "application/pdf", content[0].Source.MediaType)
	require.Equal(t, "JVBERi0xLjQK", content[0].Source.Data)
	require.Equal(t, "text", content[1].Type)
	require.NotNil(t, content[1].Text)
	require.Equal(t, "summarize it", *content[1].Text)
}

// TestRequestOpenAI2ClaudeMessage_ConvertsTextFileContentToText 测试纯文本文件的转换。
//
// 验证当消息中包含纯文本文件（notes.txt）时，
// RequestOpenAI2ClaudeMessage 会将 base64 编码的文件数据解码为纯文本，
// 然后转换为 Claude 的 text 类型内容块（而非 document）。
// 文本内容 "alpha\nbeta" 应被正确解码并保留换行符。
func TestRequestOpenAI2ClaudeMessage_ConvertsTextFileContentToText(t *testing.T) {
	request := dto.GeneralOpenAIRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.Message{
			{
				Role: "user",
				Content: []any{
					dto.MediaContent{
						Type: dto.ContentTypeFile,
						File: &dto.MessageFile{
							FileName: "notes.txt",
							FileData: base64.StdEncoding.EncodeToString([]byte("alpha\nbeta")),
						},
					},
				},
			},
		},
	}

	claudeRequest, err := RequestOpenAI2ClaudeMessage(nil, request)
	require.NoError(t, err)
	require.Len(t, claudeRequest.Messages, 1)

	content, ok := claudeRequest.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, content, 1)
	require.Equal(t, "text", content[0].Type)
	require.NotNil(t, content[0].Text)
	require.Equal(t, "alpha\nbeta", *content[0].Text)
}
