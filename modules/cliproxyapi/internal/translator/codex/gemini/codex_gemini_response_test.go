// gemini - codex_gemini_response_test.go
// 测试 Codex 响应格式到 Gemini 响应格式的流式/非流式转换逻辑，覆盖图片生成、输出回退和去重场景
package gemini

import (
	"context"
	"testing"

	"github.com/tidwall/gjson"
)

// TestConvertCodexResponseToGemini_StreamEmptyOutputUsesOutputItemDoneMessageFallback 测试流式空输出时的回退机制
// 当上游未提供顶层 output 数组时，应从 response.output_item.done 事件中回退提取完成内容
func TestConvertCodexResponseToGemini_StreamEmptyOutputUsesOutputItemDoneMessageFallback(t *testing.T) {
	ctx := context.Background()
	originalRequest := []byte(`{"tools":[]}`)
	var param any

	chunks := [][]byte{
		[]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]},\"output_index\":0}"),
		[]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}"),
	}

	var outputs [][]byte
	for _, chunk := range chunks {
		outputs = append(outputs, ConvertCodexResponseToGemini(ctx, "gemini-2.5-pro", originalRequest, nil, chunk, &param)...)
	}

	found := false
	for _, out := range outputs {
		if gjson.GetBytes(out, "candidates.0.content.parts.0.text").String() == "ok" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fallback content from response.output_item.done message; outputs=%q", outputs)
	}
}

// TestConvertCodexResponseToGemini_StreamPartialImageEmitsInlineData 测试流式部分图片的内联数据输出
// 验证 partial_image 事件能正确转为 Gemini 的 inlineData 格式（含 base64 数据和 mimeType），且重复图片块被抑制
func TestConvertCodexResponseToGemini_StreamPartialImageEmitsInlineData(t *testing.T) {
	ctx := context.Background()
	originalRequest := []byte(`{"tools":[]}`)
	var param any

	chunk := []byte(`data: {"type":"response.image_generation_call.partial_image","item_id":"ig_123","output_format":"png","partial_image_b64":"aGVsbG8=","partial_image_index":0}`)
	out := ConvertCodexResponseToGemini(ctx, "gemini-2.5-pro", originalRequest, nil, chunk, &param)
	if len(out) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out))
	}

	got := gjson.GetBytes(out[0], "candidates.0.content.parts.0.inlineData.data").String()
	if got != "aGVsbG8=" {
		t.Fatalf("expected inlineData.data %q, got %q; chunk=%s", "aGVsbG8=", got, string(out[0]))
	}

	gotMime := gjson.GetBytes(out[0], "candidates.0.content.parts.0.inlineData.mimeType").String()
	if gotMime != "image/png" {
		t.Fatalf("expected inlineData.mimeType %q, got %q; chunk=%s", "image/png", gotMime, string(out[0]))
	}

	out = ConvertCodexResponseToGemini(ctx, "gemini-2.5-pro", originalRequest, nil, chunk, &param)
	if len(out) != 0 {
		t.Fatalf("expected duplicate image chunk to be suppressed, got %d", len(out))
	}
}

// TestConvertCodexResponseToGemini_StreamImageGenerationCallDoneEmitsInlineData 测试图片生成完成事件的内联数据输出
// 验证 output_item.done 中图片数据能正确转为 Gemini inlineData，且与最后部分图片重复时被抑制
func TestConvertCodexResponseToGemini_StreamImageGenerationCallDoneEmitsInlineData(t *testing.T) {
	ctx := context.Background()
	originalRequest := []byte(`{"tools":[]}`)
	var param any

	out := ConvertCodexResponseToGemini(ctx, "gemini-2.5-pro", originalRequest, nil, []byte(`data: {"type":"response.image_generation_call.partial_image","item_id":"ig_123","output_format":"png","partial_image_b64":"aGVsbG8=","partial_image_index":0}`), &param)
	if len(out) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out))
	}

	out = ConvertCodexResponseToGemini(ctx, "gemini-2.5-pro", originalRequest, nil, []byte(`data: {"type":"response.output_item.done","item":{"id":"ig_123","type":"image_generation_call","output_format":"png","result":"aGVsbG8="}}`), &param)
	if len(out) != 0 {
		t.Fatalf("expected output_item.done to be suppressed when identical to last partial image, got %d", len(out))
	}

	out = ConvertCodexResponseToGemini(ctx, "gemini-2.5-pro", originalRequest, nil, []byte(`data: {"type":"response.output_item.done","item":{"id":"ig_123","type":"image_generation_call","output_format":"jpeg","result":"Ymll"}}`), &param)
	if len(out) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out))
	}

	got := gjson.GetBytes(out[0], "candidates.0.content.parts.0.inlineData.data").String()
	if got != "Ymll" {
		t.Fatalf("expected inlineData.data %q, got %q; chunk=%s", "Ymll", got, string(out[0]))
	}

	gotMime := gjson.GetBytes(out[0], "candidates.0.content.parts.0.inlineData.mimeType").String()
	if gotMime != "image/jpeg" {
		t.Fatalf("expected inlineData.mimeType %q, got %q; chunk=%s", "image/jpeg", gotMime, string(out[0]))
	}
}

// TestConvertCodexResponseToGemini_NonStreamImageGenerationCallAddsInlineDataPart 测试非流式图片生成的内联数据部分
// 验证非流式响应中 image_generation_call 输出能正确转为 Gemini 的 inlineData part
func TestConvertCodexResponseToGemini_NonStreamImageGenerationCallAddsInlineDataPart(t *testing.T) {
	ctx := context.Background()
	originalRequest := []byte(`{"tools":[]}`)

	raw := []byte(`{"type":"response.completed","response":{"id":"resp_123","created_at":1700000000,"usage":{"input_tokens":1,"output_tokens":1},"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]},{"type":"image_generation_call","output_format":"png","result":"aGVsbG8="}]}}`)
	out := ConvertCodexResponseToGeminiNonStream(ctx, "gemini-2.5-pro", originalRequest, nil, raw, nil)

	got := gjson.GetBytes(out, "candidates.0.content.parts.1.inlineData.data").String()
	if got != "aGVsbG8=" {
		t.Fatalf("expected inlineData.data %q, got %q; chunk=%s", "aGVsbG8=", got, string(out))
	}

	gotMime := gjson.GetBytes(out, "candidates.0.content.parts.1.inlineData.mimeType").String()
	if gotMime != "image/png" {
		t.Fatalf("expected inlineData.mimeType %q, got %q; chunk=%s", "image/png", gotMime, string(out))
	}
}
