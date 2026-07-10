package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func configureCustomOAuthFetchSetting(t *testing.T, enableSSRFProtection bool, allowPrivateIP bool) {
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
	fetchSetting.AllowedPorts = []string{"80", "443"}
	fetchSetting.ApplyIPFilterForDomain = true
}

func TestValidateCustomOAuthDiscoveryURLRejectsPrivateURLWhenProtectionEnabled(t *testing.T) {
	configureCustomOAuthFetchSetting(t, true, false)

	err := validateCustomOAuthDiscoveryURL("http://127.0.0.1/.well-known/openid-configuration")

	require.Error(t, err)
	require.Contains(t, err.Error(), "private IP address not allowed")
}

func TestValidateCustomOAuthDiscoveryURLAllowsPublicURLWhenProtectionEnabled(t *testing.T) {
	configureCustomOAuthFetchSetting(t, true, false)

	err := validateCustomOAuthDiscoveryURL("https://93.184.216.34/.well-known/openid-configuration")

	require.NoError(t, err)
}

func TestFetchCustomOAuthDiscoveryBlocksPrivateTargetBeforeRequest(t *testing.T) {
	configureCustomOAuthFetchSetting(t, true, false)
	gin.SetMode(gin.TestMode)

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"issuer":"http://example.com"}`))
	}))
	t.Cleanup(server.Close)

	body := []byte(`{"well_known_url":"` + server.URL + `"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/custom-oauth-provider/discovery", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	FetchCustomOAuthDiscovery(ctx)

	require.False(t, called, "SSRF 预校验应在真实 discovery 请求发出前拦截私网目标")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Discovery URL 安全校验失败")
}

func TestFetchCustomOAuthDiscoveryAllowsConfiguredTargetWhenProtectionDisabled(t *testing.T) {
	configureCustomOAuthFetchSetting(t, false, false)
	gin.SetMode(gin.TestMode)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"issuer":"https://provider.example","authorization_endpoint":"https://provider.example/oauth/authorize"}`))
	}))
	t.Cleanup(server.Close)

	body := []byte(`{"well_known_url":"` + server.URL + `"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/custom-oauth-provider/discovery", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	FetchCustomOAuthDiscovery(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.Contains(t, recorder.Body.String(), `"well_known_url":"`+server.URL+`"`)
	require.Contains(t, recorder.Body.String(), `"authorization_endpoint":"https://provider.example/oauth/authorize"`)
}
