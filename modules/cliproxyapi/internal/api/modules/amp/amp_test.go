// amp - amp_test.go
// AMP（API Management Proxy）模块的核心功能单元测试。
// 测试模块的以下关键行为：
// - 模块名称和初始化
// - 注册流程：有上游 URL 时启用代理、无上游 URL 时禁用代理但仍注册别名路由
// - 无效上游 URL 的错误处理
// - 配置更新时的缓存失效机制
// - 认证中间件的回退行为
// - 多上游 API Key 配置变更检测（含重复键和空白键的处理）
package amp

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/api/modules"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
)

// TestAmpModule_Name 测试模块名称应返回 "amp-routing"
func TestAmpModule_Name(t *testing.T) {
	m := New()
	if m.Name() != "amp-routing" {
		t.Fatalf("want amp-routing, got %s", m.Name())
	}
}

// TestAmpModule_New 测试 NewLegacy 构造函数：
// - accessManager 和 authMiddleware 正确设置
// - enabled 初始为 false
// - proxy 初始为 nil
func TestAmpModule_New(t *testing.T) {
	accessManager := sdkaccess.NewManager()
	authMiddleware := func(c *gin.Context) { c.Next() }

	m := NewLegacy(accessManager, authMiddleware)

	if m.accessManager != accessManager {
		t.Fatal("accessManager not set")
	}
	if m.authMiddleware_ == nil {
		t.Fatal("authMiddleware not set")
	}
	if m.enabled {
		t.Fatal("enabled should be false initially")
	}
	if m.proxy != nil {
		t.Fatal("proxy should be nil initially")
	}
}

// TestAmpModule_Register_WithUpstream 测试提供有效上游 URL 时的注册流程：
// - 模块应被启用
// - proxy 和 secretSource 应被初始化
func TestAmpModule_Register_WithUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Fake upstream to ensure URL is valid
	upstream := httptest.NewServer(nil)
	defer upstream.Close()

	accessManager := sdkaccess.NewManager()
	base := &handlers.BaseAPIHandler{}

	m := NewLegacy(accessManager, func(c *gin.Context) { c.Next() })

	cfg := &config.Config{
		AmpCode: config.AmpCode{
			UpstreamURL:    upstream.URL,
			UpstreamAPIKey: "test-key",
		},
	}

	ctx := modules.Context{Engine: r, BaseHandler: base, Config: cfg, AuthMiddleware: func(c *gin.Context) { c.Next() }}
	if err := m.Register(ctx); err != nil {
		t.Fatalf("register error: %v", err)
	}

	if !m.enabled {
		t.Fatal("module should be enabled with upstream URL")
	}
	if m.proxy == nil {
		t.Fatal("proxy should be initialized")
	}
	if m.secretSource == nil {
		t.Fatal("secretSource should be initialized")
	}
}

// TestAmpModule_Register_WithoutUpstream 测试没有上游 URL 时的注册流程：
// - 注册不应返回错误
// - 模块应被禁用，proxy 不应初始化
// - 但提供商别名路由仍然应被注册（确保路由始终可用）
func TestAmpModule_Register_WithoutUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	accessManager := sdkaccess.NewManager()
	base := &handlers.BaseAPIHandler{}

	m := NewLegacy(accessManager, func(c *gin.Context) { c.Next() })

	cfg := &config.Config{
		AmpCode: config.AmpCode{
			UpstreamURL: "", // No upstream
		},
	}

	ctx := modules.Context{Engine: r, BaseHandler: base, Config: cfg, AuthMiddleware: func(c *gin.Context) { c.Next() }}
	if err := m.Register(ctx); err != nil {
		t.Fatalf("register should not error without upstream: %v", err)
	}

	if m.enabled {
		t.Fatal("module should be disabled without upstream URL")
	}
	if m.proxy != nil {
		t.Fatal("proxy should not be initialized without upstream")
	}

	// But provider aliases should still be registered
	req := httptest.NewRequest("GET", "/api/provider/openai/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == 404 {
		t.Fatal("provider aliases should be registered even without upstream")
	}
}

// TestAmpModule_Register_InvalidUpstream 测试无效上游 URL 时注册应返回错误
func TestAmpModule_Register_InvalidUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	accessManager := sdkaccess.NewManager()
	base := &handlers.BaseAPIHandler{}

	m := NewLegacy(accessManager, func(c *gin.Context) { c.Next() })

	cfg := &config.Config{
		AmpCode: config.AmpCode{
			UpstreamURL: "://invalid-url",
		},
	}

	ctx := modules.Context{Engine: r, BaseHandler: base, Config: cfg, AuthMiddleware: func(c *gin.Context) { c.Next() }}
	if err := m.Register(ctx); err == nil {
		t.Fatal("expected error for invalid upstream URL")
	}
}

// TestAmpModule_OnConfigUpdated_CacheInvalidation 测试配置更新时缓存失效机制：
// 先预热缓存，然后更新配置，验证缓存被清除（变为 nil）
func TestAmpModule_OnConfigUpdated_CacheInvalidation(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "secrets.json")
	if err := os.WriteFile(p, []byte(`{"apiKey@https://ampcode.com/":"v1"}`), 0600); err != nil {
		t.Fatal(err)
	}

	m := &AmpModule{enabled: true}
	ms := NewMultiSourceSecretWithPath("", p, time.Minute)
	m.secretSource = ms
	m.lastConfig = &config.AmpCode{
		UpstreamAPIKey: "old-key",
	}

	// Warm the cache
	if _, err := ms.Get(context.Background()); err != nil {
		t.Fatal(err)
	}

	if ms.cache == nil {
		t.Fatal("expected cache to be set")
	}

	// Update config - should invalidate cache
	if err := m.OnConfigUpdated(&config.Config{AmpCode: config.AmpCode{UpstreamURL: "http://x", UpstreamAPIKey: "new-key"}}); err != nil {
		t.Fatal(err)
	}

	if ms.cache != nil {
		t.Fatal("expected cache to be invalidated")
	}
}

// TestAmpModule_OnConfigUpdated_NotEnabled 测试禁用状态下配置更新不应返回错误或 panic
func TestAmpModule_OnConfigUpdated_NotEnabled(t *testing.T) {
	m := &AmpModule{enabled: false}

	// Should not error or panic when disabled
	if err := m.OnConfigUpdated(&config.Config{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestAmpModule_OnConfigUpdated_URLRemoved 测试上游 URL 被移除时的配置更新：
// 应记录警告但不返回错误
func TestAmpModule_OnConfigUpdated_URLRemoved(t *testing.T) {
	m := &AmpModule{enabled: true}
	ms := NewMultiSourceSecret("", 0)
	m.secretSource = ms

	// Config update with empty URL - should log warning but not error
	cfg := &config.Config{AmpCode: config.AmpCode{UpstreamURL: ""}}

	if err := m.OnConfigUpdated(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestAmpModule_OnConfigUpdated_NonMultiSourceSecret 测试当 secretSource 是 StaticSecretSource 时，
// OnConfigUpdated 不应 panic
func TestAmpModule_OnConfigUpdated_NonMultiSourceSecret(t *testing.T) {
	// Test that OnConfigUpdated doesn't panic with StaticSecretSource
	m := &AmpModule{enabled: true}
	m.secretSource = NewStaticSecretSource("static-key")

	cfg := &config.Config{AmpCode: config.AmpCode{UpstreamURL: "http://example.com"}}

	// Should not error or panic
	if err := m.OnConfigUpdated(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestAmpModule_AuthMiddleware_Fallback 测试当模块没有设置认证中间件时，
// getAuthMiddleware 应返回一个回退中间件（非 nil），且该中间件应允许请求通过
func TestAmpModule_AuthMiddleware_Fallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Create module with no auth middleware
	m := &AmpModule{authMiddleware_: nil}

	// Get the fallback middleware via getAuthMiddleware
	ctx := modules.Context{Engine: r, AuthMiddleware: nil}
	middleware := m.getAuthMiddleware(ctx)

	if middleware == nil {
		t.Fatal("getAuthMiddleware should return a fallback, not nil")
	}

	// Test that it works
	called := false
	r.GET("/test", middleware, func(c *gin.Context) {
		called = true
		c.String(200, "ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !called {
		t.Fatal("fallback middleware should allow requests through")
	}
}

// TestAmpModule_SecretSource_FromConfig 测试配置中的显式 API Key 能通过 secretSource 正确返回
func TestAmpModule_SecretSource_FromConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	upstream := httptest.NewServer(nil)
	defer upstream.Close()

	accessManager := sdkaccess.NewManager()
	base := &handlers.BaseAPIHandler{}

	m := NewLegacy(accessManager, func(c *gin.Context) { c.Next() })

	// Config with explicit API key
	cfg := &config.Config{
		AmpCode: config.AmpCode{
			UpstreamURL:    upstream.URL,
			UpstreamAPIKey: "config-key",
		},
	}

	ctx := modules.Context{Engine: r, BaseHandler: base, Config: cfg, AuthMiddleware: func(c *gin.Context) { c.Next() }}
	if err := m.Register(ctx); err != nil {
		t.Fatalf("register error: %v", err)
	}

	// Secret source should be MultiSourceSecret with config key
	if m.secretSource == nil {
		t.Fatal("secretSource should be set")
	}

	// Verify it returns the config key
	key, err := m.secretSource.Get(context.Background())
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if key != "config-key" {
		t.Fatalf("want config-key, got %s", key)
	}
}

// TestAmpModule_ProviderAliasesAlwaysRegistered 测试无论是否有上游 URL，
// 提供商别名路由都应始终被注册
func TestAmpModule_ProviderAliasesAlwaysRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scenarios := []struct {
		name      string
		configURL string
	}{
		{"with_upstream", "http://example.com"},
		{"without_upstream", ""},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			r := gin.New()
			accessManager := sdkaccess.NewManager()
			base := &handlers.BaseAPIHandler{}

			m := NewLegacy(accessManager, func(c *gin.Context) { c.Next() })

			cfg := &config.Config{AmpCode: config.AmpCode{UpstreamURL: scenario.configURL}}

			ctx := modules.Context{Engine: r, BaseHandler: base, Config: cfg, AuthMiddleware: func(c *gin.Context) { c.Next() }}
			if err := m.Register(ctx); err != nil && scenario.configURL != "" {
				t.Fatalf("register error: %v", err)
			}

			// Provider aliases should always be available
			req := httptest.NewRequest("GET", "/api/provider/openai/models", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code == 404 {
				t.Fatal("provider aliases should be registered")
			}
		})
	}
}

// TestAmpModule_hasUpstreamAPIKeysChanged_DetectsRemovedKeyWithDuplicateInput 测试当新配置中
// 包含重复键（k1, k1）但移除了旧键（k2）时，变更检测应正确识别差异
func TestAmpModule_hasUpstreamAPIKeysChanged_DetectsRemovedKeyWithDuplicateInput(t *testing.T) {
	m := &AmpModule{}

	oldCfg := &config.AmpCode{
		UpstreamAPIKeys: []config.AmpUpstreamAPIKeyEntry{
			{UpstreamAPIKey: "u1", APIKeys: []string{"k1", "k2"}},
		},
	}
	newCfg := &config.AmpCode{
		UpstreamAPIKeys: []config.AmpUpstreamAPIKeyEntry{
			{UpstreamAPIKey: "u1", APIKeys: []string{"k1", "k1"}},
		},
	}

	if !m.hasUpstreamAPIKeysChanged(oldCfg, newCfg) {
		t.Fatal("expected change to be detected when k2 is removed but new list contains duplicates")
	}
}

// TestAmpModule_hasUpstreamAPIKeysChanged_IgnoresEmptyAndWhitespaceKeys 测试变更检测
// 应忽略空白和空字符串键的差异
func TestAmpModule_hasUpstreamAPIKeysChanged_IgnoresEmptyAndWhitespaceKeys(t *testing.T) {
	m := &AmpModule{}

	oldCfg := &config.AmpCode{
		UpstreamAPIKeys: []config.AmpUpstreamAPIKeyEntry{
			{UpstreamAPIKey: "u1", APIKeys: []string{"k1", "k2"}},
		},
	}
	newCfg := &config.AmpCode{
		UpstreamAPIKeys: []config.AmpUpstreamAPIKeyEntry{
			{UpstreamAPIKey: "u1", APIKeys: []string{"  k1  ", "", "k2", "   "}},
		},
	}

	if m.hasUpstreamAPIKeysChanged(oldCfg, newCfg) {
		t.Fatal("expected no change when only whitespace/empty entries differ")
	}
}
