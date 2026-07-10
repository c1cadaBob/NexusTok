package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func configureOAuthFetchSetting(t *testing.T, enableSSRFProtection bool, allowPrivateIP bool) {
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
	// 测试中的 httptest server 使用随机端口。这里放开端口范围，确保断言聚焦在
	// 私网/回环地址本身，而不是被端口策略提前拦截。
	fetchSetting.AllowedPorts = []string{"1-65535"}
	fetchSetting.ApplyIPFilterForDomain = true
}

func configureOIDCSettings(t *testing.T, tokenEndpoint string, userInfoEndpoint string) {
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

func newTestGenericOAuthProvider(tokenEndpoint string, userInfoEndpoint string) *GenericOAuthProvider {
	return NewGenericOAuthProvider(&model.CustomOAuthProvider{
		Id:               1,
		Name:             "Test OAuth",
		Slug:             "test-oauth",
		Enabled:          true,
		ClientId:         "client-id",
		ClientSecret:     "client-secret",
		TokenEndpoint:    tokenEndpoint,
		UserInfoEndpoint: userInfoEndpoint,
		UserIdField:      "id",
		UsernameField:    "username",
		DisplayNameField: "name",
		EmailField:       "email",
	})
}

func TestValidateConfiguredOAuthEndpointURLRejectsPrivateURLWhenProtectionEnabled(t *testing.T) {
	configureOAuthFetchSetting(t, true, false)

	err := validateConfiguredOAuthEndpointURL("http://127.0.0.1/oauth/token")

	require.Error(t, err)
	require.Contains(t, err.Error(), "private IP address not allowed")
}

func TestValidateConfiguredOAuthEndpointURLAllowsPublicURLWhenProtectionEnabled(t *testing.T) {
	configureOAuthFetchSetting(t, true, false)

	err := validateConfiguredOAuthEndpointURL("https://93.184.216.34/oauth/token")

	require.NoError(t, err)
}

func TestGenericOAuthExchangeTokenBlocksPrivateEndpointBeforeRequest(t *testing.T) {
	configureOAuthFetchSetting(t, true, false)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer"}`))
	}))
	t.Cleanup(server.Close)

	provider := newTestGenericOAuthProvider(server.URL, "https://93.184.216.34/userinfo")
	token, err := provider.ExchangeToken(context.Background(), "code", nil)

	require.Nil(t, token)
	require.Error(t, err)
	require.Contains(t, err.Error(), "private IP address not allowed")
	require.False(t, called, "SSRF 预校验应在 token 请求发出前拦截私网端点")
}

func TestGenericOAuthGetUserInfoBlocksPrivateEndpointBeforeRequest(t *testing.T) {
	configureOAuthFetchSetting(t, true, false)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"u1","username":"alice","email":"alice@example.com"}`))
	}))
	t.Cleanup(server.Close)

	provider := newTestGenericOAuthProvider("https://93.184.216.34/oauth/token", server.URL)
	user, err := provider.GetUserInfo(context.Background(), &OAuthToken{AccessToken: "token", TokenType: "Bearer"})

	require.Nil(t, user)
	require.Error(t, err)
	require.Contains(t, err.Error(), "private IP address not allowed")
	require.False(t, called, "SSRF 预校验应在 userinfo 请求发出前拦截私网端点")
}

func TestGenericOAuthExchangeTokenAllowsConfiguredEndpointWhenProtectionDisabled(t *testing.T) {
	configureOAuthFetchSetting(t, false, false)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer","scope":"openid"}`))
	}))
	t.Cleanup(server.Close)

	provider := newTestGenericOAuthProvider(server.URL, "https://93.184.216.34/userinfo")
	token, err := provider.ExchangeToken(context.Background(), "code", nil)

	require.NoError(t, err)
	require.NotNil(t, token)
	require.Equal(t, "token", token.AccessToken)
	require.Equal(t, "Bearer", token.TokenType)
}

func TestOIDCExchangeTokenBlocksPrivateEndpointBeforeRequest(t *testing.T) {
	configureOAuthFetchSetting(t, true, false)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer"}`))
	}))
	t.Cleanup(server.Close)
	configureOIDCSettings(t, server.URL, "https://93.184.216.34/userinfo")

	token, err := (&OIDCProvider{}).ExchangeToken(context.Background(), "code", nil)

	require.Nil(t, token)
	require.Error(t, err)
	require.Contains(t, err.Error(), "private IP address not allowed")
	require.False(t, called, "SSRF 预校验应在 OIDC token 请求发出前拦截私网端点")
}

func TestOIDCGetUserInfoBlocksPrivateEndpointBeforeRequest(t *testing.T) {
	configureOAuthFetchSetting(t, true, false)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"sub":"u1","email":"alice@example.com"}`))
	}))
	t.Cleanup(server.Close)
	configureOIDCSettings(t, "https://93.184.216.34/oauth/token", server.URL)

	user, err := (&OIDCProvider{}).GetUserInfo(context.Background(), &OAuthToken{AccessToken: "token"})

	require.Nil(t, user)
	require.Error(t, err)
	require.Contains(t, err.Error(), "private IP address not allowed")
	require.False(t, called, "SSRF 预校验应在 OIDC userinfo 请求发出前拦截私网端点")
}
