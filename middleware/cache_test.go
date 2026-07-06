package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCacheHotReloadStaticAssetsNoCache(t *testing.T) {
	t.Setenv("NEXUSTOK_HOT_RELOAD", "true")

	router := newCacheTestRouter()
	recorder := performCacheRequest(router, "/static/css/index.css")

	if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("热更新静态资源 Cache-Control = %q, want %q", got, "no-cache")
	}
}

func TestCacheProductionStaticAssetsMaxAge(t *testing.T) {
	t.Setenv("NEXUSTOK_HOT_RELOAD", "false")

	router := newCacheTestRouter()
	recorder := performCacheRequest(router, "/static/css/index.173ebfb3b5.css")

	if got := recorder.Header().Get("Cache-Control"); got != "max-age=604800" {
		t.Fatalf("生产静态资源 Cache-Control = %q, want %q", got, "max-age=604800")
	}
}

func TestCacheKeepsHtmlAndBrandAssetsNoCache(t *testing.T) {
	t.Setenv("NEXUSTOK_HOT_RELOAD", "false")

	tests := []string{"/", "/logo.png", "/favicon.ico"}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			router := newCacheTestRouter()
			recorder := performCacheRequest(router, path)

			if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
				t.Fatalf("%s Cache-Control = %q, want %q", path, got, "no-cache")
			}
		})
	}
}

func newCacheTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Cache())
	router.GET("/*path", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return router
}

func performCacheRequest(router *gin.Engine, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
