// MiniMax 通道适配器的单元测试文件。
// 测试图片生成相关的请求 URL 构建、请求格式转换和响应处理功能。
package minimax

// 标准库导入
import (
	"encoding/json"          // JSON 序列化/反序列化
	"net/http"               // HTTP 状态码和响应
	"net/http/httptest"      // HTTP 测试工具
	"strings"                // 字符串操作
	"testing"                // Go 测试框架
	"time"                   // 时间处理

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"                        // 数据传输对象
	relaycommon "github.com/c1cada/NexusTok/relay/common"      // Relay 通用模块
	relayconstant "github.com/c1cada/NexusTok/relay/constant"  // Relay 常量定义

	// 第三方依赖
	"github.com/gin-gonic/gin" // Gin Web 框架
)

// TestGetRequestURLForImageGeneration 测试图片生成模式下的请求 URL 构建。
// 验证 GetRequestURL 在 RelayMode 为图片生成时，返回正确的 MiniMax 图片生成 API 地址。
func TestGetRequestURLForImageGeneration(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://api.minimax.chat",
		},
	}

	got, err := GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}

	want := "https://api.minimax.chat/v1/image_generation"
	if got != want {
		t.Fatalf("GetRequestURL() = %q, want %q", got, want)
	}
}

// TestConvertImageRequest 测试 OpenAI 图片请求到 MiniMax 格式的转换。
// 验证模型名称、提示词、数量、宽高比和响应格式的正确转换。
func TestConvertImageRequest(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "image-01",
	}
	request := dto.ImageRequest{
		Model:          "image-01",
		Prompt:         "a red fox in snowfall",
		Size:           "1536x1024",
		ResponseFormat: "url",
		N:              uintPtr(2),
	}

	got, err := adaptor.ConvertImageRequest(gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()), info, request)
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if payload["model"] != "image-01" {
		t.Fatalf("model = %#v, want %q", payload["model"], "image-01")
	}
	if payload["prompt"] != request.Prompt {
		t.Fatalf("prompt = %#v, want %q", payload["prompt"], request.Prompt)
	}
	if payload["n"] != float64(2) {
		t.Fatalf("n = %#v, want 2", payload["n"])
	}
	if payload["aspect_ratio"] != "3:2" {
		t.Fatalf("aspect_ratio = %#v, want %q", payload["aspect_ratio"], "3:2")
	}
	if payload["response_format"] != "url" {
		t.Fatalf("response_format = %#v, want %q", payload["response_format"], "url")
	}
}

// TestDoResponseForImageGeneration 测试图片生成响应的处理。
// 验证 MiniMax 原始响应能正确转换为 OpenAI 格式，
// 且不会暴露 MiniMax 原始的 image_urls 字段名。
func TestDoResponseForImageGeneration(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		StartTime: time.Unix(1700000000, 0),
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       httptest.NewRecorder().Result().Body,
	}
	resp.Body = ioNopCloser(`{"data":{"image_urls":["https://example.com/minimax.png"]}}`)

	adaptor := &Adaptor{}
	usage, err := adaptor.DoResponse(c, resp, info)
	if err != nil {
		t.Fatalf("DoResponse returned error: %v", err)
	}
	if usage == nil {
		t.Fatalf("DoResponse returned nil usage")
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `"url":"https://example.com/minimax.png"`) {
		t.Fatalf("response body = %s, want OpenAI image response with image URL", body)
	}
	if strings.Contains(body, `"image_urls"`) {
		t.Fatalf("response body = %s, should not expose raw MiniMax image_urls payload", body)
	}
}

// nopReadCloser 一个不执行任何操作的 io.ReadCloser 实现。
// 用于测试中模拟 HTTP 响应体。
type nopReadCloser struct {
	*strings.Reader
}

// Close 关闭读取器（空操作，始终返回 nil）。
func (n nopReadCloser) Close() error {
	return nil
}

// ioNopCloser 创建一个包装了字符串的 nopReadCloser。
// 用于在测试中模拟 HTTP 响应体。
func ioNopCloser(body string) nopReadCloser {
	return nopReadCloser{Reader: strings.NewReader(body)}
}

// uintPtr 创建一个指向 uint 值的指针。
// 辅助函数，用于在测试中构建指针类型的参数。
func uintPtr(v uint) *uint {
	return &v
}
