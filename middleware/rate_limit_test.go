package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/model"
	"github.com/c1cadaBob/NexusTok/service"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useRateLimitMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()

	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	require.NoError(t, redisClient.Ping(context.Background()).Err())

	common.RedisEnabled = true
	common.RDB = redisClient
	t.Cleanup(func() {
		_ = redisClient.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
	})

	return redisServer, redisClient
}

func performRateLimitRequest(router http.Handler, path string, remoteAddr string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = remoteAddr
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestRedisIPRateLimiterThresholdTTLAndNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/limited", rateLimitFactory(2, 37, "TEST"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	remoteAddr := "192.0.2.10:12345"
	legacyKey := "rateLimit:TEST192.0.2.10"
	_, err := redisServer.Push(legacyKey, "legacy-list-entry")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", remoteAddr).Code)
	limitedResponse := performRateLimitRequest(router, "/limited", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, limitedResponse.Code)
	assert.Equal(t, "37", limitedResponse.Header().Get("Retry-After"))

	key := redisIPRateLimitKey("TEST", "192.0.2.10")
	count, err := redisServer.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "3", count)
	assert.Equal(t, 37*time.Second, redisServer.TTL(key))
	assert.True(t, redisServer.Exists(legacyKey), "the v2 counter must not touch an old list key")
}

func TestGlobalAPIRateLimitSkipsOnlyVerifiedAdministrators(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupDashboardAuthMiddlewareTest(t)
	redisServer, _ := useRateLimitMiniRedis(t)

	previousEnabled := common.GlobalApiRateLimitEnable
	previousNum := common.GlobalApiRateLimitNum
	previousDuration := common.GlobalApiRateLimitDuration
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = previousEnabled
		common.GlobalApiRateLimitNum = previousNum
		common.GlobalApiRateLimitDuration = previousDuration
	})
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 1
	common.GlobalApiRateLimitDuration = 60
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))

	now := time.Now().Unix()
	createUser := func(t *testing.T, username string, role int) (*model.User, string) {
		t.Helper()
		token := username + "-pat"
		user := &model.User{
			Username: username, Password: "password-placeholder", Role: role,
			Status: common.UserStatusEnabled, Group: "default", AccessToken: &token,
			AuthVersion: 1, AffCode: "middleware-rate-" + username,
		}
		require.NoError(t, model.DB.Create(user).Error)
		session := &model.UserSession{
			SID: username + "-session", UserID: user.Id, Version: 1,
			UserAuthVersion: user.AuthVersion, Status: model.UserSessionStatusActive,
			RefreshHash: username + "-refresh-hash", LoginMethod: "password",
			CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
		}
		require.NoError(t, model.CreateUserSession(session))
		identity := service.AuthIdentity{
			UserID: user.Id, SessionID: session.SID,
			UserAuthVersion: session.UserAuthVersion, SessionVersion: session.Version,
		}
		accessToken, _, err := service.IssueAccessToken(identity)
		require.NoError(t, err)
		return user, accessToken
	}

	admin, adminAccessToken := createUser(t, "rate-admin", common.RoleAdminUser)
	root, rootAccessToken := createUser(t, "rate-root", common.RoleRootUser)
	_, ordinaryAccessToken := createUser(t, "rate-ordinary", common.RoleCommonUser)

	newRouter := func() *gin.Engine {
		router := gin.New()
		require.NoError(t, router.SetTrustedProxies(nil))
		router.GET("/limited", GlobalAPIRateLimit(), func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})
		return router
	}
	request := func(router http.Handler, token string, remoteAddr string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/limited", nil)
		request.RemoteAddr = remoteAddr
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		router.ServeHTTP(recorder, request)
		return recorder
	}

	t.Run("anonymous requests use the GA bucket", func(t *testing.T) {
		router := newRouter()
		assert.Equal(t, http.StatusNoContent, request(router, "", "192.0.2.100:12345").Code)
		assert.Equal(t, http.StatusTooManyRequests, request(router, "", "192.0.2.100:12345").Code)
		count, err := redisServer.Get(redisIPRateLimitKey("GA", "192.0.2.100"))
		require.NoError(t, err)
		assert.Equal(t, "2", count)
	})

	t.Run("ordinary user requests remain limited", func(t *testing.T) {
		router := newRouter()
		assert.Equal(t, http.StatusNoContent, request(router, ordinaryAccessToken, "192.0.2.101:12345").Code)
		assert.Equal(t, http.StatusTooManyRequests, request(router, ordinaryAccessToken, "192.0.2.101:12345").Code)
		count, err := redisServer.Get(redisIPRateLimitKey("GA", "192.0.2.101"))
		require.NoError(t, err)
		assert.Equal(t, "2", count)
	})

	t.Run("verified admin and root requests bypass GA", func(t *testing.T) {
		router := newRouter()
		for range 3 {
			assert.Equal(t, http.StatusNoContent, request(router, adminAccessToken, "192.0.2.102:12345").Code)
			assert.Equal(t, http.StatusNoContent, request(router, rootAccessToken, "192.0.2.103:12345").Code)
		}
		assert.False(t, redisServer.Exists(redisIPRateLimitKey("GA", "192.0.2.102")))
		assert.False(t, redisServer.Exists(redisIPRateLimitKey("GA", "192.0.2.103")))
		assert.Equal(t, common.RoleAdminUser, admin.Role)
		assert.Equal(t, common.RoleRootUser, root.Role)
	})

	t.Run("verified admin PAT bypasses GA", func(t *testing.T) {
		router := newRouter()
		for range 3 {
			assert.Equal(t, http.StatusNoContent, request(router, *admin.AccessToken, "192.0.2.104:12345").Code)
		}
		assert.False(t, redisServer.Exists(redisIPRateLimitKey("GA", "192.0.2.104")))
	})

	t.Run("verified admin API token bypasses GA", func(t *testing.T) {
		require.NoError(t, model.DB.Create(&model.Token{
			UserId:         admin.Id,
			Key:            "legacyadminapitoken",
			Status:         common.TokenStatusEnabled,
			UnlimitedQuota: true,
		}).Error)
		router := newRouter()
		for range 3 {
			assert.Equal(t, http.StatusNoContent, request(router, "sk-legacyadminapitoken", "192.0.2.108:12345").Code)
		}
		assert.False(t, redisServer.Exists(redisIPRateLimitKey("GA", "192.0.2.108")))
	})

	t.Run("expired or tampered admin tokens remain limited", func(t *testing.T) {
		router := newRouter()
		identity := service.AuthIdentity{
			UserID: admin.Id, SessionID: "expired-admin-session",
			UserAuthVersion: admin.AuthVersion, SessionVersion: 1,
		}
		expired := issueExpiredDashboardAccessToken(t, identity)
		tampered := tamperDashboardToken(adminAccessToken)
		assert.Equal(t, http.StatusNoContent, request(router, expired, "192.0.2.105:12345").Code)
		assert.Equal(t, http.StatusTooManyRequests, request(router, expired, "192.0.2.105:12345").Code)
		assert.Equal(t, http.StatusNoContent, request(router, tampered, "192.0.2.106:12345").Code)
		assert.Equal(t, http.StatusTooManyRequests, request(router, tampered, "192.0.2.106:12345").Code)
		assert.True(t, redisServer.Exists(redisIPRateLimitKey("GA", "192.0.2.105")))
		assert.True(t, redisServer.Exists(redisIPRateLimitKey("GA", "192.0.2.106")))

		revokedPAT := *admin.AccessToken
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", admin.Id).Update("access_token", nil).Error)
		assert.Equal(t, http.StatusNoContent, request(router, revokedPAT, "192.0.2.107:12345").Code)
		assert.Equal(t, http.StatusTooManyRequests, request(router, revokedPAT, "192.0.2.107:12345").Code)
		assert.True(t, redisServer.Exists(redisIPRateLimitKey("GA", "192.0.2.107")))
	})
}

func TestGlobalAPIRateLimitSkipsAuthenticationBootstrapRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	previousEnabled := common.GlobalApiRateLimitEnable
	previousNum := common.GlobalApiRateLimitNum
	previousDuration := common.GlobalApiRateLimitDuration
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = previousEnabled
		common.GlobalApiRateLimitNum = previousNum
		common.GlobalApiRateLimitDuration = previousDuration
	})
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 1
	common.GlobalApiRateLimitDuration = 60

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.Any("/api/status", GlobalAPIRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.Any("/api/setup", GlobalAPIRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.Any("/api/user/login", GlobalAPIRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.Any("/api/user/login/encryption-key", GlobalAPIRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.Any("/api/user/auth/refresh", GlobalAPIRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.Any("/api/ordinary", GlobalAPIRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := func(path, remoteAddr string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = remoteAddr
		router.ServeHTTP(recorder, req)
		return recorder
	}

	for index, path := range []string{
		"/api/status",
		"/api/setup",
		"/api/user/login",
		"/api/user/login/encryption-key",
		"/api/user/auth/refresh",
	} {
		remoteIP := fmt.Sprintf("192.0.2.%d", 120+index)
		remoteAddr := remoteIP + ":12345"
		for range 3 {
			assert.Equal(t, http.StatusNoContent, request(path, remoteAddr).Code, path)
		}
		assert.False(t, redisServer.Exists(redisIPRateLimitKey("GA", remoteIP)))
	}

	ordinaryIP := "192.0.2.130"
	assert.Equal(t, http.StatusNoContent, request("/api/ordinary", ordinaryIP+":12345").Code)
	assert.Equal(t, http.StatusTooManyRequests, request("/api/ordinary", ordinaryIP+":12345").Code)
	count, err := redisServer.Get(redisIPRateLimitKey("GA", ordinaryIP))
	require.NoError(t, err)
	assert.Equal(t, "2", count)
}

func TestGlobalWebRateLimitSkipsStaticAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	previousEnabled := common.GlobalWebRateLimitEnable
	previousNum := common.GlobalWebRateLimitNum
	previousDuration := common.GlobalWebRateLimitDuration
	t.Cleanup(func() {
		common.GlobalWebRateLimitEnable = previousEnabled
		common.GlobalWebRateLimitNum = previousNum
		common.GlobalWebRateLimitDuration = previousDuration
	})
	common.GlobalWebRateLimitEnable = true
	common.GlobalWebRateLimitNum = 1
	common.GlobalWebRateLimitDuration = 60

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.Any("/*path", GlobalWebRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for range 3 {
		assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/static/js/index.js", "192.0.2.140:12345").Code)
		assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/assets/index.css", "192.0.2.141:12345").Code)
	}
	assert.False(t, redisServer.Exists(redisIPRateLimitKey("GW", "192.0.2.140")))
	assert.False(t, redisServer.Exists(redisIPRateLimitKey("GW", "192.0.2.141")))

	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/sign-in", "192.0.2.142:12345").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/sign-in", "192.0.2.142:12345").Code)
	count, err := redisServer.Get(redisIPRateLimitKey("GW", "192.0.2.142"))
	require.NoError(t, err)
	assert.Equal(t, "2", count)
}

func TestRedisUserRateLimiterUsesSharedFixedWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	router := gin.New()
	router.GET(
		"/limited",
		func(c *gin.Context) { c.Set("id", 42) },
		userRateLimitFactory(1, 23, "USER"),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", "192.0.2.20:12345").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/limited", "198.51.100.20:12345").Code)

	key := redisUserRateLimitKey("USER", 42)
	assert.True(t, redisServer.Exists(key))
	assert.Equal(t, 23*time.Second, redisServer.TTL(key))
}

func TestRedisEmailVerificationRateLimiterPreservesResponseAndTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/verify", EmailVerificationRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	remoteAddr := "192.0.2.30:12345"
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/verify", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/verify", remoteAddr).Code)
	response := performRateLimitRequest(router, "/verify", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, response.Code)
	assert.JSONEq(t, `{"success":false,"message":"发送过于频繁，请等待 30 秒后再试"}`, response.Body.String())

	key := redisIPRateLimitKey(EmailVerificationRateLimitMark, "192.0.2.30")
	assert.True(t, redisServer.Exists(key))
	assert.Equal(t, time.Duration(EmailVerificationDuration)*time.Second, redisServer.TTL(key))
}

func TestRedisFixedWindowIsAtomicUnderConcurrency(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	const (
		requestCount = 20
		maximumCount = 7
		duration     = int64(41)
	)
	key := redisIPRateLimitKey("CONCURRENT", "192.0.2.40")

	var allowedCount atomic.Int64
	errorsFound := make(chan error, requestCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(requestCount)
	for range requestCount {
		go func() {
			defer waitGroup.Done()
			allowed, _, _, err := redisFixedWindowTake(context.Background(), key, maximumCount, duration)
			if err != nil {
				errorsFound <- err
				return
			}
			if allowed {
				allowedCount.Add(1)
			}
		}()
	}
	waitGroup.Wait()
	close(errorsFound)
	for err := range errorsFound {
		require.NoError(t, err)
	}

	assert.Equal(t, int64(maximumCount), allowedCount.Load())
	count, err := redisServer.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "20", count)
	assert.Equal(t, time.Duration(duration)*time.Second, redisServer.TTL(key))
}

func TestRedisFixedWindowResetsAtBoundary(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	const duration = int64(10)
	key := redisIPRateLimitKey("BOUNDARY", "192.0.2.50")

	for range 2 {
		allowed, _, _, err := redisFixedWindowTake(context.Background(), key, 2, duration)
		require.NoError(t, err)
		assert.True(t, allowed)
	}
	allowed, _, _, err := redisFixedWindowTake(context.Background(), key, 2, duration)
	require.NoError(t, err)
	assert.False(t, allowed)

	// This reset is intentional fixed-window behavior. A client can consume one
	// full allowance immediately before and another immediately after a boundary.
	redisServer.FastForward(time.Duration(duration) * time.Second)
	for range 2 {
		allowed, _, _, err = redisFixedWindowTake(context.Background(), key, 2, duration)
		require.NoError(t, err)
		assert.True(t, allowed)
	}
}

func TestRedisFixedWindowRepairsCounterWithoutTTL(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	const duration = int64(29)
	key := redisIPRateLimitKey("MISSING-TTL", "192.0.2.51")
	redisServer.Set(key, "5")

	allowed, count, ttl, err := redisFixedWindowTake(context.Background(), key, 3, duration)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, int64(6), count)
	assert.Equal(t, duration, ttl)
	assert.Equal(t, time.Duration(duration)*time.Second, redisServer.TTL(key))

	redisServer.FastForward(time.Duration(duration) * time.Second)
	assert.False(t, redisServer.Exists(key), "a recovered counter must not remain permanently rate-limited")
}

func TestCriticalRateLimitScopesUseIndependentRedisBuckets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	previousCriticalNum := common.CriticalRateLimitNum
	previousCriticalDuration := common.CriticalRateLimitDuration
	previousCriticalEnabled := common.CriticalRateLimitEnable
	previousRefreshNum := common.AuthRefreshRateLimitNum
	previousRefreshDuration := common.AuthRefreshRateLimitDuration
	t.Cleanup(func() {
		common.CriticalRateLimitNum = previousCriticalNum
		common.CriticalRateLimitDuration = previousCriticalDuration
		common.CriticalRateLimitEnable = previousCriticalEnabled
		common.AuthRefreshRateLimitNum = previousRefreshNum
		common.AuthRefreshRateLimitDuration = previousRefreshDuration
	})
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = 2
	common.CriticalRateLimitDuration = 31
	common.AuthRefreshRateLimitNum = 2
	common.AuthRefreshRateLimitDuration = 43

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/login", CriticalRateLimitScope("auth-login"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/refresh", AuthRefreshRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	remoteAddr := "192.0.2.70:12345"
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/login", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/login", remoteAddr).Code)
	loginLimited := performRateLimitRequest(router, "/login", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, loginLimited.Code)
	assert.Equal(t, "31", loginLimited.Header().Get("Retry-After"))

	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/refresh", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/refresh", remoteAddr).Code)
	refreshLimited := performRateLimitRequest(router, "/refresh", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, refreshLimited.Code)
	assert.Equal(t, "43", refreshLimited.Header().Get("Retry-After"))

	loginKey := redisIPRateLimitKey("CT:auth-login", "192.0.2.70")
	refreshKey := redisIPRateLimitKey("CT:auth-refresh", "192.0.2.70")
	loginCount, err := redisServer.Get(loginKey)
	require.NoError(t, err)
	refreshCount, err := redisServer.Get(refreshKey)
	require.NoError(t, err)
	assert.Equal(t, "3", loginCount)
	assert.Equal(t, "3", refreshCount)
	assert.False(t, redisServer.Exists(redisIPRateLimitKey("CT", "192.0.2.70")))
}

func TestCriticalRateLimitScopesUseIndependentMemoryBuckets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	previousCriticalNum := common.CriticalRateLimitNum
	previousCriticalDuration := common.CriticalRateLimitDuration
	previousCriticalEnabled := common.CriticalRateLimitEnable
	previousRefreshNum := common.AuthRefreshRateLimitNum
	previousRefreshDuration := common.AuthRefreshRateLimitDuration
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
		common.CriticalRateLimitNum = previousCriticalNum
		common.CriticalRateLimitDuration = previousCriticalDuration
		common.CriticalRateLimitEnable = previousCriticalEnabled
		common.AuthRefreshRateLimitNum = previousRefreshNum
		common.AuthRefreshRateLimitDuration = previousRefreshDuration
	})
	common.RedisEnabled = false
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = 1
	common.CriticalRateLimitDuration = 60
	common.AuthRefreshRateLimitNum = 1
	common.AuthRefreshRateLimitDuration = 60

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/login", CriticalRateLimitScope("auth-login"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/refresh", AuthRefreshRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	remoteAddr := "198.51.100.240:12345"
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/login", remoteAddr).Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/login", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/refresh", remoteAddr).Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/refresh", remoteAddr).Code)
}

func TestRedisFailurePolicies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, redisClient := useRateLimitMiniRedis(t)
	require.NoError(t, redisClient.Close())

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/ip", rateLimitFactory(1, 30, "FAIL-IP"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET(
		"/user",
		func(c *gin.Context) { c.Set("id", 7) },
		userRateLimitFactory(1, 30, "FAIL-USER"),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	router.GET("/email", EmailVerificationRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	ipResponse := performRateLimitRequest(router, "/ip", "192.0.2.60:12345")
	assert.Equal(t, http.StatusInternalServerError, ipResponse.Code)
	assert.Empty(t, ipResponse.Body.String())
	userResponse := performRateLimitRequest(router, "/user", "192.0.2.61:12345")
	assert.Equal(t, http.StatusInternalServerError, userResponse.Code)
	assert.Empty(t, userResponse.Body.String())
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/email", "192.0.2.62:12345").Code)
}
