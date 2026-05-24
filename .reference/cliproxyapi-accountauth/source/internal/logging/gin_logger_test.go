// logging - gin_logger_test.go
// Gin 日志恢复中间件测试
// 验证 GinLogrusRecovery 中间件对不同类型 panic 的处理行为：
// - http.ErrAbortHandler 类型的 panic 应被重新抛出（不捕获）
// - 普通 panic 应被捕获并返回 500 状态码
// - AI API 路径识别功能
package logging

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestGinLogrusRecoveryRepanicsErrAbortHandler 验证当 handler 中 panic
// http.ErrAbortHandler 时，GinLogrusRecovery 中间件不会捕获它，
// 而是将其重新抛出，让上层调用者处理。这是 Gin 框架的标准行为。
func TestGinLogrusRecoveryRepanicsErrAbortHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(GinLogrusRecovery())
	engine.GET("/abort", func(c *gin.Context) {
		panic(http.ErrAbortHandler)
	})

	req := httptest.NewRequest(http.MethodGet, "/abort", nil)
	recorder := httptest.NewRecorder()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic, got nil")
		}
		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("expected error panic, got %T", recovered)
		}
		if !errors.Is(err, http.ErrAbortHandler) {
			t.Fatalf("expected ErrAbortHandler, got %v", err)
		}
		if err != http.ErrAbortHandler {
			t.Fatalf("expected exact ErrAbortHandler sentinel, got %v", err)
		}
	}()

	engine.ServeHTTP(recorder, req)
}

// TestGinLogrusRecoveryHandlesRegularPanic 验证当 handler 中 panic
// 普通字符串（如 "boom"）时，中间件能够捕获该 panic 并返回 HTTP 500 状态码，
// 而不是让进程崩溃。
func TestGinLogrusRecoveryHandlesRegularPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(GinLogrusRecovery())
	engine.GET("/panic", func(c *gin.Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", recorder.Code)
	}
}

// TestIsAIAPIPathIncludesImages 验证 isAIAPIPath 函数能够正确识别
// 图片生成/编辑端点（/v1/images/*）和视频端点（/v1/videos/*）为 AI API 路径，
// 这些路径的日志处理方式与普通 API 路径不同。
func TestIsAIAPIPathIncludesImages(t *testing.T) {
	if !isAIAPIPath("/v1/images/generations") {
		t.Fatalf("expected /v1/images/generations to be treated as AI API path")
	}
	if !isAIAPIPath("/v1/images/edits") {
		t.Fatalf("expected /v1/images/edits to be treated as AI API path")
	}
	if !isAIAPIPath("/v1/videos") {
		t.Fatalf("expected /v1/videos to be treated as AI API path")
	}
	if !isAIAPIPath("/v1/videos/video_123") {
		t.Fatalf("expected /v1/videos/video_123 to be treated as AI API path")
	}
}
