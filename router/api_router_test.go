package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/c1cadaBob/NexusTok/common"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticationRoutesUseIndependentRateLimitScopes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	require.NoError(t, redisClient.Ping(context.Background()).Err())
	defer redisClient.Close()

	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	previousCriticalEnabled := common.CriticalRateLimitEnable
	previousCriticalNum := common.CriticalRateLimitNum
	previousCriticalDuration := common.CriticalRateLimitDuration
	previousRefreshNum := common.AuthRefreshRateLimitNum
	previousRefreshDuration := common.AuthRefreshRateLimitDuration
	previousGlobalAPIEnabled := common.GlobalApiRateLimitEnable
	previousGlobalAPINum := common.GlobalApiRateLimitNum
	previousGlobalAPIDuration := common.GlobalApiRateLimitDuration
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
		common.CriticalRateLimitEnable = previousCriticalEnabled
		common.CriticalRateLimitNum = previousCriticalNum
		common.CriticalRateLimitDuration = previousCriticalDuration
		common.AuthRefreshRateLimitNum = previousRefreshNum
		common.AuthRefreshRateLimitDuration = previousRefreshDuration
		common.GlobalApiRateLimitEnable = previousGlobalAPIEnabled
		common.GlobalApiRateLimitNum = previousGlobalAPINum
		common.GlobalApiRateLimitDuration = previousGlobalAPIDuration
	})
	common.RedisEnabled = true
	common.RDB = redisClient
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = 1
	common.CriticalRateLimitDuration = 60
	common.AuthRefreshRateLimitNum = 1
	common.AuthRefreshRateLimitDuration = 60
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 1
	common.GlobalApiRateLimitDuration = 60

	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(nil))
	require.NotPanics(t, func() {
		SetApiRouter(engine)
	})

	request := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{"))
		req.RemoteAddr = "192.0.2.80:12345"
		engine.ServeHTTP(recorder, req)
		return recorder
	}

	loginFirst := request("/api/user/login")
	assert.NotEqual(t, http.StatusTooManyRequests, loginFirst.Code)
	assert.Equal(t, http.StatusTooManyRequests, request("/api/user/login").Code)
	loginCount, err := redisServer.Get("rateLimit:v2:ip:CT:auth-login:192.0.2.80")
	require.NoError(t, err)
	assert.Equal(t, "2", loginCount)
	assert.False(t, redisServer.Exists("rateLimit:v2:ip:GA:192.0.2.80"))

	registerFirst := request("/api/user/register")
	assert.NotEqual(t, http.StatusTooManyRequests, registerFirst.Code)
	assert.Equal(t, http.StatusTooManyRequests, request("/api/user/register").Code)

	refreshFirst := request("/api/user/auth/refresh")
	assert.Equal(t, http.StatusUnauthorized, refreshFirst.Code)
	refreshLimited := request("/api/user/auth/refresh")
	assert.Equal(t, http.StatusTooManyRequests, refreshLimited.Code)
	assert.Equal(t, "60", refreshLimited.Header().Get("Retry-After"))
	refreshCount, err := redisServer.Get("rateLimit:v2:ip:CT:auth-refresh:192.0.2.80")
	require.NoError(t, err)
	assert.Equal(t, "2", refreshCount)
	assert.True(t, redisServer.Exists("rateLimit:v2:ip:GA:192.0.2.80"))

	assert.True(t, redisServer.Exists("rateLimit:v2:ip:CT:auth-login:192.0.2.80"))
	assert.True(t, redisServer.Exists("rateLimit:v2:ip:CT:auth-register:192.0.2.80"))
	assert.True(t, redisServer.Exists("rateLimit:v2:ip:CT:auth-refresh:192.0.2.80"))
	assert.False(t, redisServer.Exists("rateLimit:v2:ip:CT:192.0.2.80"))
}
