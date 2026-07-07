package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/constant"
	"github.com/gin-gonic/gin"
)

func TestAnonymousRequestBodyLimitAllowsBodyAndKeepsReadable(t *testing.T) {
	restore := setAnonymousRequestBodyLimitKB(t, 1)
	defer restore()

	router := newAnonymousRequestBodyLimitTestRouter()
	recorder := performAnonymousRequestBodyLimitRequest(router, strings.Repeat("a", 1024))

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() != strings.Repeat("a", 1024) {
		t.Fatalf("controller 读取到的请求体不一致")
	}
}

func TestAnonymousRequestBodyLimitRejectsTooLargeBody(t *testing.T) {
	restore := setAnonymousRequestBodyLimitKB(t, 1)
	defer restore()

	router := newAnonymousRequestBodyLimitTestRouter()
	recorder := performAnonymousRequestBodyLimitRequest(router, strings.Repeat("a", 1025))

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestAnonymousRequestBodyLimitCanBeDisabled(t *testing.T) {
	restore := setAnonymousRequestBodyLimitKB(t, -1)
	defer restore()

	router := newAnonymousRequestBodyLimitTestRouter()
	body := strings.Repeat("a", 2048)
	recorder := performAnonymousRequestBodyLimitRequest(router, body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() != body {
		t.Fatalf("禁用限制后 controller 读取到的请求体不一致")
	}
}

func setAnonymousRequestBodyLimitKB(t *testing.T, limitKB int) func() {
	t.Helper()

	previousLimitKB := constant.AnonymousRequestBodyLimitKB
	constant.AnonymousRequestBodyLimitKB = limitKB

	return func() {
		constant.AnonymousRequestBodyLimitKB = previousLimitKB
	}
}

func newAnonymousRequestBodyLimitTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AnonymousRequestBodyLimit())
	router.POST("/anonymous", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.String(http.StatusOK, string(body))
	})
	return router
}

func performAnonymousRequestBodyLimitRequest(router http.Handler, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/anonymous", strings.NewReader(body))
	router.ServeHTTP(recorder, request)
	return recorder
}
