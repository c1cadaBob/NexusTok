package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func configureLegacyOIDCFetchSetting(t *testing.T, enableSSRFProtection bool, allowPrivateIP bool) {
	t.Helper()
	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	t.Cleanup(func() {
		*fetchSetting = original
	})

	fetchSetting.EnableSSRFProtection = enableSSRFProtection
	fetchSetting.AllowPrivateIp = allowPrivateIP
	fetchSetting.DomainFilterMode = false
	fetchSetting.IpFilterMode = false
	fetchSetting.DomainList = nil
	fetchSetting.IpList = nil
	// httptest server 使用随机端口；这里放开端口范围，使断言聚焦在私网/回环地址。
	fetchSetting.AllowedPorts = []string{"1-65535"}
	fetchSetting.ApplyIPFilterForDomain = true
}

func configureLegacyOIDCSettings(t *testing.T, tokenEndpoint string, userInfoEndpoint string) {
	t.Helper()
	settings := system_setting.GetOIDCSettings()
	original := *settings
	t.Cleanup(func() {
		*settings = original
	})

	settings.Enabled = true
	settings.ClientId = "client-id"
	settings.ClientSecret = "client-secret"
	settings.TokenEndpoint = tokenEndpoint
	settings.UserInfoEndpoint = userInfoEndpoint
}

func TestValidateLegacyOIDCEndpointURLRejectsPrivateURLWhenProtectionEnabled(t *testing.T) {
	configureLegacyOIDCFetchSetting(t, true, false)

	err := validateLegacyOIDCEndpointURL("http://127.0.0.1/oauth/token")

	require.Error(t, err)
	require.Contains(t, err.Error(), "private IP address not allowed")
}

func TestLegacyOIDCBlocksPrivateTokenEndpointBeforeRequest(t *testing.T) {
	configureLegacyOIDCFetchSetting(t, true, false)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer"}`))
	}))
	t.Cleanup(server.Close)
	configureLegacyOIDCSettings(t, server.URL, "https://93.184.216.34/userinfo")

	user, err := getOidcUserInfoByCode("code")

	require.Nil(t, user)
	require.Error(t, err)
	require.Contains(t, err.Error(), "无法连接至 OIDC 服务器")
	require.False(t, called, "SSRF 预校验应在旧 OIDC token 请求发出前拦截私网端点")
}

func TestLegacyOIDCBlocksPrivateUserInfoEndpointBeforeRequest(t *testing.T) {
	configureLegacyOIDCFetchSetting(t, true, false)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer"}`))
	}))
	t.Cleanup(tokenServer.Close)

	userInfoCalled := false
	userInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userInfoCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"sub":"u1","email":"alice@example.com"}`))
	}))
	t.Cleanup(userInfoServer.Close)

	configureLegacyOIDCSettings(t, tokenServer.URL, userInfoServer.URL)

	user, err := getOidcUserInfoByCode("code")

	require.Nil(t, user)
	require.Error(t, err)
	require.Contains(t, err.Error(), "无法连接至 OIDC 服务器")
	require.False(t, userInfoCalled, "SSRF 预校验应在旧 OIDC userinfo 请求发出前拦截私网端点")
}

func TestLegacyOIDCAllowsConfiguredEndpointsWhenProtectionDisabled(t *testing.T) {
	configureLegacyOIDCFetchSetting(t, false, false)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer"}`))
	}))
	t.Cleanup(tokenServer.Close)

	userInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"sub":"u1","email":"alice@example.com","preferred_username":"alice","name":"Alice"}`))
	}))
	t.Cleanup(userInfoServer.Close)

	configureLegacyOIDCSettings(t, tokenServer.URL, userInfoServer.URL)

	user, err := getOidcUserInfoByCode("code")

	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, "u1", user.OpenID)
	require.Equal(t, "alice@example.com", user.Email)
}
