package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/gin-gonic/gin"
)

func TestIsStaticAssetWebRateLimitExemptPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "默认前端入口脚本",
			path: "/static/js/index.6009f82119.js",
			want: true,
		},
		{
			name: "默认前端异步分片",
			path: "/static/js/async/chunk.12345678.js",
			want: true,
		},
		{
			name: "默认前端样式文件",
			path: "/static/css/index.173ebfb3b5.css",
			want: true,
		},
		{
			name: "默认前端字体文件",
			path: "/static/font/public-sans.035c7fe496.woff2",
			want: true,
		},
		{
			name: "经典前端资源",
			path: "/assets/index-BjSQIHtv.css",
			want: true,
		},
		{
			name: "站点 logo",
			path: "/logo.png",
			want: true,
		},
		{
			name: "站点 favicon",
			path: "/favicon.ico",
			want: true,
		},
		{
			name: "robots 文件",
			path: "/robots.txt",
			want: true,
		},
		{
			name: "排行榜动态页面",
			path: "/rankings",
			want: false,
		},
		{
			name: "价格动态页面",
			path: "/pricing",
			want: false,
		},
		{
			name: "后台动态页面",
			path: "/dashboard",
			want: false,
		},
		{
			name: "API 路径不走静态豁免",
			path: "/api/status",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStaticAssetWebRateLimitExemptPath(tt.path); got != tt.want {
				t.Fatalf("isStaticAssetWebRateLimitExemptPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestGlobalWebRateLimitExemptsStaticAssets(t *testing.T) {
	restore := setTestGlobalWebRateLimit(t, 1, 180)
	defer restore()

	router := newRateLimitTestRouter()
	for i := 0; i < 2; i++ {
		recorder := performRateLimitRequest(router, "/static/js/async/chunk.12345678.js")
		if recorder.Code != http.StatusOK {
			t.Fatalf("第 %d 次静态资源请求状态码 = %d, want %d", i+1, recorder.Code, http.StatusOK)
		}
	}
}

func TestGlobalWebRateLimitKeepsDynamicPageLimited(t *testing.T) {
	restore := setTestGlobalWebRateLimit(t, 1, 180)
	defer restore()

	router := newRateLimitTestRouter()
	first := performRateLimitRequest(router, "/rankings")
	if first.Code != http.StatusOK {
		t.Fatalf("首次动态页面请求状态码 = %d, want %d", first.Code, http.StatusOK)
	}

	second := performRateLimitRequest(router, "/rankings")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("第二次动态页面请求状态码 = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
}

func TestGlobalAPIRateLimitExemptsCaptureHelperDownloads(t *testing.T) {
	restore := setTestGlobalAPIRateLimit(t, 1, 180)
	defer restore()

	router := newAPIRateLimitTestRouter()
	paths := []string{
		"/api/channel/upstream-account/capture-helper.user.js",
		"/api/channel/upstream-account/capture-session/session-id/userscript.user.js",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			first := performRateLimitRequest(router, path)
			if first.Code != http.StatusOK {
				t.Fatalf("首次 helper 下载状态码 = %d, want %d", first.Code, http.StatusOK)
			}
			second := performRateLimitRequest(router, path)
			if second.Code != http.StatusOK {
				t.Fatalf("第二次 helper 下载状态码 = %d, want %d", second.Code, http.StatusOK)
			}
		})
	}

	limited := performRateLimitRequest(router, "/api/channel/1")
	if limited.Code != http.StatusOK {
		t.Fatalf("首次普通 API 状态码 = %d, want %d", limited.Code, http.StatusOK)
	}
	limited = performRateLimitRequest(router, "/api/channel/1")
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("第二次普通 API 状态码 = %d, want %d", limited.Code, http.StatusTooManyRequests)
	}
	if limited.Body.Len() == 0 {
		t.Fatal("限流响应正文为空")
	}
	if got := limited.Header().Get("Retry-After"); got == "" {
		t.Fatal("限流响应缺少 Retry-After")
	}
}

func TestBuildRedisRateLimitKeyUsesFixedWindow(t *testing.T) {
	now := time.Unix(370, 0)

	if got := buildRedisRateLimitKey("rateLimit:GA203.0.113.10", 180, now); got != "rateLimit:GA203.0.113.10:2" {
		t.Fatalf("固定窗口 key = %q, want %q", got, "rateLimit:GA203.0.113.10:2")
	}
	if got := buildRedisRateLimitKey("rateLimit:GA203.0.113.10", 0, now); got != "rateLimit:GA203.0.113.10:370" {
		t.Fatalf("无效 duration 应回退到 1 秒窗口，got %q", got)
	}
}

func TestRedisRateLimitExpireSeconds(t *testing.T) {
	if got := redisRateLimitExpireSeconds(180); got != 240 {
		t.Fatalf("expire seconds = %d, want 240", got)
	}
	if got := redisRateLimitExpireSeconds(0); got != 61 {
		t.Fatalf("无效 duration 的过期时间 = %d, want 61", got)
	}
}

func setTestGlobalWebRateLimit(t *testing.T, limit int, duration int64) func() {
	t.Helper()

	previousRedisEnabled := common.RedisEnabled
	previousGlobalWebRateLimitEnable := common.GlobalWebRateLimitEnable
	previousGlobalWebRateLimitNum := common.GlobalWebRateLimitNum
	previousGlobalWebRateLimitDuration := common.GlobalWebRateLimitDuration
	previousRateLimitKeyExpirationDuration := common.RateLimitKeyExpirationDuration
	previousInMemoryRateLimiter := inMemoryRateLimiter

	common.RedisEnabled = false
	common.GlobalWebRateLimitEnable = true
	common.GlobalWebRateLimitNum = limit
	common.GlobalWebRateLimitDuration = duration
	// 本组测试只验证请求窗口命中，不依赖后台过期清理。将清理间隔设为 0，
	// 避免测试重置全局内存限流器时，上一轮清理协程仍持有同一个 mutex 引发竞态。
	common.RateLimitKeyExpirationDuration = 0
	inMemoryRateLimiter = common.InMemoryRateLimiter{}

	return func() {
		common.RedisEnabled = previousRedisEnabled
		common.GlobalWebRateLimitEnable = previousGlobalWebRateLimitEnable
		common.GlobalWebRateLimitNum = previousGlobalWebRateLimitNum
		common.GlobalWebRateLimitDuration = previousGlobalWebRateLimitDuration
		common.RateLimitKeyExpirationDuration = previousRateLimitKeyExpirationDuration
		inMemoryRateLimiter = previousInMemoryRateLimiter
	}
}

func setTestGlobalAPIRateLimit(t *testing.T, limit int, duration int64) func() {
	t.Helper()

	previousRedisEnabled := common.RedisEnabled
	previousGlobalAPIRateLimitEnable := common.GlobalApiRateLimitEnable
	previousGlobalAPIRateLimitNum := common.GlobalApiRateLimitNum
	previousGlobalAPIRateLimitDuration := common.GlobalApiRateLimitDuration
	previousRateLimitKeyExpirationDuration := common.RateLimitKeyExpirationDuration
	previousInMemoryRateLimiter := inMemoryRateLimiter

	common.RedisEnabled = false
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = limit
	common.GlobalApiRateLimitDuration = duration
	common.RateLimitKeyExpirationDuration = 0
	inMemoryRateLimiter = common.InMemoryRateLimiter{}

	return func() {
		common.RedisEnabled = previousRedisEnabled
		common.GlobalApiRateLimitEnable = previousGlobalAPIRateLimitEnable
		common.GlobalApiRateLimitNum = previousGlobalAPIRateLimitNum
		common.GlobalApiRateLimitDuration = previousGlobalAPIRateLimitDuration
		common.RateLimitKeyExpirationDuration = previousRateLimitKeyExpirationDuration
		inMemoryRateLimiter = previousInMemoryRateLimiter
	}
}

func newRateLimitTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GlobalWebRateLimit())
	router.GET("/*path", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router
}

func newAPIRateLimitTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GlobalAPIRateLimit())
	router.GET("/*path", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router
}

func performRateLimitRequest(router http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = "203.0.113.10:12345"
	router.ServeHTTP(recorder, request)
	return recorder
}
