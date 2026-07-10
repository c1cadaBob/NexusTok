package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func configureUptimeKumaFetchSetting(t *testing.T, enableSSRFProtection bool, allowPrivateIP bool) {
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

func TestValidateUptimeKumaFetchURLRejectsPrivateURLWhenProtectionEnabled(t *testing.T) {
	configureUptimeKumaFetchSetting(t, true, false)

	err := validateUptimeKumaFetchURL("http://127.0.0.1/api/status-page/public")

	require.Error(t, err)
	require.Contains(t, err.Error(), "private IP address not allowed")
}

func TestValidateUptimeKumaFetchURLAllowsPublicURLWhenProtectionEnabled(t *testing.T) {
	configureUptimeKumaFetchSetting(t, true, false)

	err := validateUptimeKumaFetchURL("https://93.184.216.34/api/status-page/public")

	require.NoError(t, err)
}

func TestGetAndDecodeBlocksPrivateTargetBeforeRequest(t *testing.T) {
	configureUptimeKumaFetchSetting(t, true, false)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	var payload struct {
		OK bool `json:"ok"`
	}
	err := getAndDecode(context.Background(), server.Client(), server.URL, &payload)

	require.Error(t, err)
	require.False(t, called, "SSRF 预校验应在真实请求发出前拦截私网目标")
}

func TestGetAndDecodeAllowsConfiguredTargetWhenProtectionDisabled(t *testing.T) {
	configureUptimeKumaFetchSetting(t, false, false)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	var payload struct {
		OK bool `json:"ok"`
	}
	err := getAndDecode(context.Background(), server.Client(), server.URL, &payload)

	require.NoError(t, err)
	require.True(t, payload.OK)
}
