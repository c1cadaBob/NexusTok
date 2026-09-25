package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/constant"
	"github.com/c1cadaBob/NexusTok/model"
	"github.com/c1cadaBob/NexusTok/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestRealPlatformSiteAdaptersReadOnly(t *testing.T) {
	newAPIBaseURL := strings.TrimSpace(os.Getenv("NEXUSTOK_REAL_NEWAPI_BASE_URL"))
	newAPIUsername := strings.TrimSpace(os.Getenv("NEXUSTOK_REAL_NEWAPI_USERNAME"))
	newAPIPassword := os.Getenv("NEXUSTOK_REAL_NEWAPI_PASSWORD")
	sub2APIBaseURL := strings.TrimSpace(os.Getenv("NEXUSTOK_REAL_SUB2API_BASE_URL"))
	sub2APIUsername := strings.TrimSpace(os.Getenv("NEXUSTOK_REAL_SUB2API_USERNAME"))
	sub2APIPassword := os.Getenv("NEXUSTOK_REAL_SUB2API_PASSWORD")
	if newAPIBaseURL == "" || newAPIUsername == "" || newAPIPassword == "" ||
		sub2APIBaseURL == "" || sub2APIUsername == "" || sub2APIPassword == "" {
		t.Skip("未配置真实平台只读验证环境变量")
	}

	testCases := []struct {
		name       string
		adapter    PlatformSiteAdapter
		baseURL    string
		username   string
		password   string
		wantModels bool
	}{
		{
			name:       "NewAPI",
			adapter:    NewNewAPIAdapter(nil),
			baseURL:    newAPIBaseURL,
			username:   newAPIUsername,
			password:   newAPIPassword,
			wantModels: true,
		},
		{
			name:       "Sub2API",
			adapter:    NewSub2APIAdapter(nil),
			baseURL:    sub2APIBaseURL,
			username:   sub2APIUsername,
			password:   sub2APIPassword,
			wantModels: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			session, err := testCase.adapter.Authenticate(
				context.Background(),
				testCase.baseURL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: testCase.username,
					Password: testCase.password,
				},
			)
			require.NoError(t, err)
			snapshot, err := testCase.adapter.FetchSnapshot(context.Background(), session)
			require.NoError(t, err)
			require.NotEmpty(t, snapshot.Keys)

			hasModels := false
			for _, key := range snapshot.Keys {
				if key.ModelsSynced && len(key.Models) > 0 && key.Secret != "" {
					hasModels = true
					break
				}
			}
			assert.Equal(t, testCase.wantModels, hasModels)
		})
	}
}

func TestValidatePlatformSiteURLAllowsHTTPAndPrivateTargets(t *testing.T) {
	for _, rawURL := range []string{
		"http://127.0.0.1",
		"http://localhost",
		"http://10.0.0.1:8080",
		"http://[::1]:8080",
	} {
		t.Run(rawURL, func(t *testing.T) {
			require.NoError(t, validatePlatformSiteURL(rawURL))
		})
	}
}

func TestPlatformSiteHTTPClientAllowsPrivateHTTPWithoutGlobalSSRFClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client, err := newPlatformSiteHTTPClient()
	require.NoError(t, err)
	response, err := client.Get(server.URL)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func TestPlatformSiteCaptureSessionBindsUserAndConsumesAfterSave(t *testing.T) {
	start, err := StartPlatformSiteCaptureSession(7, PlatformSiteCaptureStartRequest{
		Platform: model.PlatformSub2API,
		BaseURL:  "http://127.0.0.1:8080",
		AuthType: model.UpstreamAuthAccessToken,
	}, "http://127.0.0.1:3003")
	require.NoError(t, err)
	require.NotEmpty(t, start.CaptureID)
	require.Contains(t, start.UserscriptURL, "install_token=")
	require.Contains(t, start.HandoffURL, platformSiteCaptureHandoffParam+"=")
	require.Equal(t, "http://127.0.0.1:8080", start.Origin)

	record, found, err := platformSiteCaptureCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, record.Secret)

	_, err = CompletePlatformSiteCaptureSession(start.CaptureID, PlatformSiteCaptureCompleteRequest{
		CaptureSecret: record.Secret,
		Platform:      model.PlatformSub2API,
		AuthType:      model.UpstreamAuthAccessToken,
		Origin:        "http://127.0.0.1:8080",
		AccessToken:   "sub2-access-token",
		RefreshToken:  "sub2-refresh-token",
		ExpiresIn:     3600,
		AuthUser:      map[string]any{"id": 17},
	})
	require.NoError(t, err)

	status, err := GetPlatformSiteCaptureStatus(7, start.CaptureID, "http://127.0.0.1:3003")
	require.NoError(t, err)
	require.Equal(t, platformSiteCaptureStatusComplete, status.Status)
	require.NotNil(t, status.Summary)
	require.Equal(t, "sub...ken", status.Summary.AccessTokenMasked)
	require.True(t, status.Summary.RefreshTokenPresent)
	require.Greater(t, status.Summary.TokenExpiresAt, common.GetTimestamp())

	resolved, err := ResolvePlatformSiteCapture(
		7,
		start.CaptureID,
		0,
		model.PlatformSub2API,
		model.UpstreamAuthAccessToken,
	)
	require.NoError(t, err)
	require.Equal(t, "sub2-access-token", resolved.Credential.AccessToken)
	require.Equal(t, "sub2-refresh-token", resolved.Credential.RefreshToken)

	require.NoError(t, ConsumePlatformSiteCapture(7, start.CaptureID, 0))
	_, err = ResolvePlatformSiteCapture(
		7,
		start.CaptureID,
		0,
		model.PlatformSub2API,
		model.UpstreamAuthAccessToken,
	)
	require.Error(t, err)
}

func TestPlatformSiteCaptureSessionAutoSelectsCredentialPriority(t *testing.T) {
	tests := []struct {
		name       string
		request    PlatformSiteCaptureCompleteRequest
		wantAuth   string
		wantAccess string
		wantAdmin  string
		wantCookie string
	}{
		{
			name: "access token wins over admin key and cookie",
			request: PlatformSiteCaptureCompleteRequest{
				AccessToken:  "access-token",
				RefreshToken: "refresh-token",
				AdminKey:     "admin-key",
				Cookie:       "session=secret",
				AuthUser:     map[string]any{"id": 23},
			},
			wantAuth:   model.UpstreamAuthAccessToken,
			wantAccess: "access-token",
		},
		{
			name: "admin key wins when access token is missing",
			request: PlatformSiteCaptureCompleteRequest{
				AdminKey: "admin-key",
				Cookie:   "session=secret",
			},
			wantAuth:  model.UpstreamAuthAdminKey,
			wantAdmin: "admin-key",
		},
		{
			name: "cookie is used as last resort",
			request: PlatformSiteCaptureCompleteRequest{
				Cookie: "session=secret",
			},
			wantAuth:   model.UpstreamAuthCookie,
			wantCookie: "session=secret",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			start, err := StartPlatformSiteCaptureSession(17, PlatformSiteCaptureStartRequest{
				Platform: model.PlatformNewAPI,
				BaseURL:  "http://127.0.0.1:8181",
				AuthType: PlatformSiteCaptureAuthAuto,
			}, "http://127.0.0.1:3003")
			require.NoError(t, err)
			record, found, err := platformSiteCaptureCache.Get(start.CaptureID)
			require.NoError(t, err)
			require.True(t, found)

			request := testCase.request
			request.CaptureSecret = record.Secret
			request.Platform = model.PlatformNewAPI
			request.AuthType = PlatformSiteCaptureAuthAuto
			request.Origin = record.Origin
			status, err := CompletePlatformSiteCaptureSession(start.CaptureID, request)
			require.NoError(t, err)
			require.NotNil(t, status.Summary)
			assert.Equal(t, PlatformSiteCaptureAuthAuto, status.AuthType)
			assert.Equal(t, testCase.wantAuth, status.Summary.AuthType)

			resolved, err := ResolvePlatformSiteCapture(
				17,
				start.CaptureID,
				0,
				model.PlatformNewAPI,
				PlatformSiteCaptureAuthAuto,
			)
			require.NoError(t, err)
			assert.Equal(t, testCase.wantAuth, resolved.Credential.AuthType)
			assert.Equal(t, testCase.wantAccess, resolved.Credential.AccessToken)
			assert.Equal(t, testCase.wantAdmin, resolved.Credential.AdminKey)
			assert.Equal(t, testCase.wantCookie, resolved.Credential.Cookie)
		})
	}
}

func TestPlatformSiteCaptureSessionAutoFailsWithoutReadableCredential(t *testing.T) {
	start, err := StartPlatformSiteCaptureSession(18, PlatformSiteCaptureStartRequest{
		Platform: model.PlatformNewAPI,
		BaseURL:  "http://127.0.0.1:8182",
		AuthType: PlatformSiteCaptureAuthAuto,
	}, "http://127.0.0.1:3003")
	require.NoError(t, err)
	record, found, err := platformSiteCaptureCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)

	_, err = CompletePlatformSiteCaptureSession(start.CaptureID, PlatformSiteCaptureCompleteRequest{
		CaptureSecret: record.Secret,
		Platform:      model.PlatformNewAPI,
		AuthType:      PlatformSiteCaptureAuthAuto,
		Origin:        record.Origin,
	})
	require.Error(t, err)

	status, err := GetPlatformSiteCaptureStatus(18, start.CaptureID, "http://127.0.0.1:3003")
	require.NoError(t, err)
	assert.Equal(t, platformSiteCaptureStatusFailed, status.Status)
	assert.Equal(t, "自动配置未采集到可用登录态", status.Message)
}

func TestPlatformSiteCaptureSessionForPasswordStoresOnlyCachedAccessToken(t *testing.T) {
	start, err := StartPlatformSiteCaptureSession(19, PlatformSiteCaptureStartRequest{
		Platform:  model.PlatformSub2API,
		BaseURL:   "http://127.0.0.1:8183",
		AuthType:  model.UpstreamAuthPassword,
		ChannelID: 77,
	}, "http://127.0.0.1:3003")
	require.NoError(t, err)
	record, found, err := platformSiteCaptureCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, model.UpstreamAuthPassword, record.AuthType)

	status, err := CompletePlatformSiteCaptureSession(start.CaptureID, PlatformSiteCaptureCompleteRequest{
		CaptureSecret: record.Secret,
		Platform:      model.PlatformSub2API,
		AuthType:      model.UpstreamAuthAccessToken,
		Origin:        record.Origin,
		AccessToken:   "captured-access",
		RefreshToken:  "captured-refresh",
		ExpiresIn:     3600,
		AuthUser:      map[string]any{"id": 29},
	})
	require.NoError(t, err)
	require.NotNil(t, status.Summary)
	assert.Equal(t, model.UpstreamAuthPassword, status.AuthType)
	assert.Equal(t, model.UpstreamAuthAccessToken, status.Summary.AuthType)
	assert.True(t, status.Summary.RefreshTokenPresent)

	resolved, err := ResolvePlatformSiteCapture(
		19,
		start.CaptureID,
		77,
		model.PlatformSub2API,
		model.UpstreamAuthPassword,
	)
	require.NoError(t, err)
	assert.Equal(t, model.UpstreamAuthAccessToken, resolved.Credential.AuthType)
	assert.Equal(t, "captured-access", resolved.Credential.AccessToken)
	assert.Equal(t, "captured-refresh", resolved.Credential.RefreshToken)
	assert.Empty(t, resolved.Credential.AdminKey)
	assert.Empty(t, resolved.Credential.Cookie)
}

func TestPlatformSiteCaptureSessionRejectsAuthTypeDowngradeAndCrossUserAccess(t *testing.T) {
	start, err := StartPlatformSiteCaptureSession(8, PlatformSiteCaptureStartRequest{
		Platform:  model.PlatformNewAPI,
		BaseURL:   "http://localhost:8081",
		AuthType:  model.UpstreamAuthAdminKey,
		ChannelID: 42,
	}, "http://localhost:3003")
	require.NoError(t, err)
	record, found, err := platformSiteCaptureCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)

	_, err = GetPlatformSiteCaptureStatus(9, start.CaptureID, "http://localhost:3003")
	require.Error(t, err)

	_, err = CompletePlatformSiteCaptureSession(start.CaptureID, PlatformSiteCaptureCompleteRequest{
		CaptureSecret: record.Secret,
		Platform:      model.PlatformNewAPI,
		AuthType:      model.UpstreamAuthAdminKey,
		Origin:        "http://localhost:8081",
		AccessToken:   "ordinary-access-token",
	})
	require.Error(t, err)

	_, err = CompletePlatformSiteCaptureSession(start.CaptureID, PlatformSiteCaptureCompleteRequest{
		CaptureSecret: record.Secret,
		Platform:      model.PlatformNewAPI,
		AuthType:      model.UpstreamAuthAdminKey,
		Origin:        "http://localhost:8081",
		AdminKey:      "admin-key",
	})
	require.NoError(t, err)

	resolved, err := ResolvePlatformSiteCapture(
		8,
		start.CaptureID,
		42,
		model.PlatformNewAPI,
		model.UpstreamAuthAdminKey,
	)
	require.NoError(t, err)
	require.Equal(t, "admin-key", resolved.Credential.AdminKey)

	_, err = CompletePlatformSiteCaptureSession(start.CaptureID, PlatformSiteCaptureCompleteRequest{
		CaptureSecret: record.Secret,
		Origin:        "http://localhost:8081",
		AdminKey:      "another-admin-key",
	})
	require.Error(t, err)
}

func TestPlatformSiteCaptureSessionRejectsMixedCredentialTypes(t *testing.T) {
	tests := []struct {
		name       string
		authType   string
		credential PlatformSiteCaptureCompleteRequest
	}{
		{
			name:     "access token with admin key",
			authType: model.UpstreamAuthAccessToken,
			credential: PlatformSiteCaptureCompleteRequest{
				AccessToken: "access-token",
				AdminKey:    "admin-key",
			},
		},
		{
			name:     "access token with cookie",
			authType: model.UpstreamAuthAccessToken,
			credential: PlatformSiteCaptureCompleteRequest{
				AccessToken: "access-token",
				Cookie:      "session=secret",
			},
		},
		{
			name:     "admin key with refresh token",
			authType: model.UpstreamAuthAdminKey,
			credential: PlatformSiteCaptureCompleteRequest{
				AdminKey:     "admin-key",
				RefreshToken: "refresh-token",
			},
		},
		{
			name:     "cookie with refresh token",
			authType: model.UpstreamAuthCookie,
			credential: PlatformSiteCaptureCompleteRequest{
				Cookie:       "session=secret",
				RefreshToken: "refresh-token",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, err := StartPlatformSiteCaptureSession(11, PlatformSiteCaptureStartRequest{
				Platform: model.PlatformNewAPI,
				BaseURL:  "http://127.0.0.1:8083",
				AuthType: test.authType,
			}, "http://127.0.0.1:3003")
			require.NoError(t, err)
			record, found, err := platformSiteCaptureCache.Get(start.CaptureID)
			require.NoError(t, err)
			require.True(t, found)

			request := test.credential
			request.CaptureSecret = record.Secret
			request.Platform = model.PlatformNewAPI
			request.AuthType = test.authType
			request.Origin = "http://127.0.0.1:8083"
			_, err = CompletePlatformSiteCaptureSession(start.CaptureID, request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "采集结果不是")
		})
	}
}

func TestPlatformSiteCaptureRejectsInvalidAndUnrelatedURLs(t *testing.T) {
	t.Run("invalid URL is not silently ignored", func(t *testing.T) {
		start, err := StartPlatformSiteCaptureSession(12, PlatformSiteCaptureStartRequest{
			Platform: model.PlatformNewAPI,
			BaseURL:  "http://127.0.0.1:8084",
			AuthType: model.UpstreamAuthAccessToken,
		}, "http://127.0.0.1:3003")
		require.NoError(t, err)
		record, found, err := platformSiteCaptureCache.Get(start.CaptureID)
		require.NoError(t, err)
		require.True(t, found)

		_, err = CompletePlatformSiteCaptureSession(start.CaptureID, PlatformSiteCaptureCompleteRequest{
			CaptureSecret:     record.Secret,
			Platform:          model.PlatformNewAPI,
			AuthType:          model.UpstreamAuthAccessToken,
			Origin:            record.Origin,
			ManagementBaseURL: "://invalid",
			AccessToken:       "access-token",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "地址格式错误")
	})

	t.Run("unrelated URL is rejected", func(t *testing.T) {
		start, err := StartPlatformSiteCaptureSession(12, PlatformSiteCaptureStartRequest{
			Platform: model.PlatformNewAPI,
			BaseURL:  "http://127.0.0.1:8085",
			AuthType: model.UpstreamAuthAccessToken,
		}, "http://127.0.0.1:3003")
		require.NoError(t, err)
		record, found, err := platformSiteCaptureCache.Get(start.CaptureID)
		require.NoError(t, err)
		require.True(t, found)

		_, err = CompletePlatformSiteCaptureSession(start.CaptureID, PlatformSiteCaptureCompleteRequest{
			CaptureSecret: record.Secret,
			Platform:      model.PlatformNewAPI,
			AuthType:      model.UpstreamAuthAccessToken,
			Origin:        record.Origin,
			RelayBaseURL:  "http://127.0.0.1:8086",
			AccessToken:   "access-token",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "不属于目标站点")
	})
}

func TestCaptureRelatedPlatformURLRequiresStrictOriginRelationship(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		candidate string
		want      bool
	}{
		{
			name:      "api host can map to adjacent management host",
			baseURL:   "https://api.github.com",
			candidate: "https://github.com",
			want:      true,
		},
		{
			name:      "different port is rejected",
			baseURL:   "https://api.example.com:8443",
			candidate: "https://example.com:443",
			want:      false,
		},
		{
			name:      "different scheme is rejected",
			baseURL:   "https://api.example.com",
			candidate: "http://example.com",
			want:      false,
		},
		{
			name:      "unrelated subdomain is rejected",
			baseURL:   "https://console.example.com",
			candidate: "https://api.example.com",
			want:      false,
		},
		{
			name:      "private hosts do not use api host heuristics",
			baseURL:   "http://api.127.0.0.1",
			candidate: "http://127.0.0.1",
			want:      false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, captureRelatedPlatformURL(
				testCase.baseURL,
				testCase.candidate,
			))
		})
	}
}

func TestPlatformSiteCaptureUserscriptsReadExplicitBrowserFieldsOnly(t *testing.T) {
	start, err := StartPlatformSiteCaptureSession(10, PlatformSiteCaptureStartRequest{
		Platform: model.PlatformSub2API,
		BaseURL:  "http://127.0.0.1:8082",
		AuthType: model.UpstreamAuthCookie,
	}, "https://nexus.example.com")
	require.NoError(t, err)
	record, found, err := platformSiteCaptureCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)

	script, err := RenderPlatformSiteCaptureUserscript(
		start.CaptureID,
		record.InstallToken,
		"https://nexus.example.com",
	)
	require.NoError(t, err)
	require.Contains(t, script, "document.cookie")
	require.Contains(t, script, "x-api-key")
	require.Contains(t, script, "/api/v1/auth/refresh")
	require.Contains(t, script, "api_base_url")
	require.Contains(t, script, "readStructured")
	require.Contains(t, script, "userIDFromToken")
	require.NotContains(t, script, record.Secret)

	helper, err := RenderPlatformSiteCaptureHelper("https://nexus.example.com")
	require.NoError(t, err)
	require.Contains(t, helper, "@match        http://*/*")
	require.Contains(t, helper, "@match        https://*/*")
	require.NotContains(t, helper, record.Secret)
}

type platformSiteRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn platformSiteRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func platformSiteJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestNewAPIAdapterPasswordAuthenticationAndSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/user/login":
			_, hasTurnstile := request.URL.Query()["turnstile"]
			assert.True(t, hasTurnstile)
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"username":"operator"`)
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"success":true,"data":{"token":"newapi-session"}}`))
		case request.URL.Path == "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":10}}`))
		case request.URL.Path == "/api/user/self":
			assert.Equal(t, "Bearer newapi-session", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":12.5,"used_quota":3}}`))
		case request.URL.Path == "/api/user/self/groups":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"default":{"ratio":0.7,"desc":"默认组"}}}`))
		case request.URL.Path == "/api/user/models":
			_, _ = writer.Write([]byte(`{"success":true,"data":["gpt-4o","claude-3-7-sonnet"]}`))
		case request.URL.Path == "/api/token/":
			assert.Equal(t, "1", request.URL.Query().Get("p"))
			assert.Equal(t, "100", request.URL.Query().Get("page_size"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":7,"name":"primary","key":"sk-newapi","group":"default","quota":8,"expired_time":"4102444800","model_limits":"gpt-4o,claude-3-7-sonnet"}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
		Password: "synthetic-password",
	})
	require.NoError(t, err)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, 1.25, snapshot.Balance)
	assert.Equal(t, int64(3), snapshot.UsedQuota)
	assert.True(t, snapshot.UsedQuotaSet)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "sk-newapi", snapshot.Keys[0].Secret)
	assert.Equal(t, int64(8), *snapshot.Keys[0].RemainQuota)
	assert.Equal(t, []string{"gpt-4o", "claude-3-7-sonnet"}, snapshot.Keys[0].Models)
	assert.Equal(t, 0.7, snapshot.Keys[0].SourceConversionRatio)
}

func TestNewAPIAdapterFallbacksBatchRevealUnlimitedQuotaAndPerKeyModels(t *testing.T) {
	loginAttempts := 0
	client := &http.Client{
		Transport: platformSiteRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch {
			case request.Method == http.MethodPost && request.URL.Path == "/api/user/login":
				_, hasTurnstile := request.URL.Query()["turnstile"]
				assert.True(t, hasTurnstile)
				body, readErr := io.ReadAll(request.Body)
				require.NoError(t, readErr)
				loginAttempts++
				if strings.Contains(string(body), `"username"`) &&
					!strings.Contains(string(body), `"email"`) {
					return platformSiteJSONResponse(http.StatusUnauthorized, `{"success":false,"message":"bad username field"}`), nil
				}
				assert.Contains(t, string(body), `"email":"operator@example.com"`)
				return platformSiteJSONResponse(http.StatusOK, `{"success":true,"data":{"access_token":"newapi-session","user":{"uid":888}}}`), nil
			case request.URL.Path == "/api/status":
				return platformSiteJSONResponse(http.StatusOK, `{"success":true,"data":{"quota_per_unit":500000}}`), nil
			case request.URL.Path == "/api/user/self":
				return platformSiteJSONResponse(http.StatusNotFound, `{"success":false,"message":"missing"}`), nil
			case request.URL.Path == "/api/user/me":
				assert.Equal(t, "Bearer newapi-session", request.Header.Get("Authorization"))
				assert.Equal(t, "888", request.Header.Get("New-API-User"))
				return platformSiteJSONResponse(http.StatusOK, `{"success":true,"data":{"user":{"quota":5000000,"used_quota":1000000}}}`), nil
			case request.URL.Path == "/api/user/self/groups":
				return platformSiteJSONResponse(http.StatusOK, `{"success":true,"data":{"default":{"ratio":0.1}}}`), nil
			case request.URL.Path == "/api/token/":
				return platformSiteJSONResponse(http.StatusNotFound, `{"success":false,"message":"missing"}`), nil
			case request.URL.Path == "/api/token":
				return platformSiteJSONResponse(http.StatusOK, `{"success":true,"data":{"records":[{"id":"7","name":"primary","key":"sk-****","group":"default","unlimited_quota":true,"remain_quota":0}],"total":1,"page_size":100}}`), nil
			case request.Method == http.MethodPost && request.URL.Path == "/api/token/batch/keys":
				return platformSiteJSONResponse(http.StatusOK, `{"success":true,"data":{"keys":{"7":"fixture-newapi-real-key"}}}`), nil
			case request.URL.Path == "/v1/models":
				assert.Equal(t, "Bearer fixture-newapi-real-key", request.Header.Get("Authorization"))
				assert.Equal(t, "fixture-newapi-real-key", request.Header.Get("x-api-key"))
				assert.Empty(t, request.Header.Get("Cookie"))
				return platformSiteJSONResponse(http.StatusOK, `{"data":[{"id":"gpt-5.5"},{"id":"gpt-4o"}]}`), nil
			default:
				return platformSiteJSONResponse(http.StatusNotFound, `{"success":false,"message":"unexpected"}`), nil
			}
		}),
	}
	adapter := NewNewAPIAdapter(client)
	session, err := adapter.Authenticate(context.Background(), "https://example.com", model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator@example.com",
		Password: "synthetic-password",
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, loginAttempts, 2)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, 10.0, snapshot.Balance)
	assert.Equal(t, int64(1000000), snapshot.UsedQuota)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "fixture-newapi-real-key", snapshot.Keys[0].Secret)
	assert.Nil(t, snapshot.Keys[0].RemainQuota)
	assert.Equal(t, []string{"gpt-5.5", "gpt-4o"}, snapshot.Keys[0].Models)
	assert.True(t, snapshot.Keys[0].ModelsSynced)
	assert.Equal(t, 0.1, snapshot.Keys[0].SourceConversionRatio)
}

func TestNewAPIAdapterBatchRevealPreservesNumericTokenIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/user/login":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"access_token":"newapi-session","user":{"id":88}}}`))
		case request.URL.Path == "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":500000}}`))
		case request.URL.Path == "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case request.URL.Path == "/api/user/self/groups":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"default":{"ratio":1}}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/token/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":7,"name":"primary","key":"sk-****","group":"default","models":["gpt-5.5"]}],"total":1,"page_size":100}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/token/batch/keys":
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"ids":[7]`)
			assert.NotContains(t, string(body), `"ids":["7"]`)
			_, _ = writer.Write([]byte(`{"success":true,"data":{"keys":{"7":"fixture-newapi-real-key"}}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
		Password: "synthetic-password",
	})
	require.NoError(t, err)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "fixture-newapi-real-key", snapshot.Keys[0].Secret)
	assert.Equal(t, []string{"gpt-5.5"}, snapshot.Keys[0].Models)
}

func TestNewAPITokenKeyParsesDirectDataString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/token/7/key":
			_, _ = writer.Write([]byte(`{"success":true,"data":"fixture-direct-data-key"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	key, err := fetchNewAPITokenKey(context.Background(), session, "7")
	require.NoError(t, err)
	assert.Equal(t, "fixture-direct-data-key", key)
}

func TestNewAPIAdapterPasswordAuthenticationAddsCompatUserHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/user/login":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":275,"username":"operator","status":1}}`))
		case request.URL.Path == "/api/user/self":
			assert.Empty(t, request.Header.Get("Authorization"))
			assert.Equal(t, "275", request.Header.Get("New-API-User"))
			assert.Equal(t, "275", request.Header.Get("X-ModelFlare-User"))
			assert.Equal(t, "275", request.Header.Get("User-id"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":12.5,"used_quota":3}}`))
		case request.URL.Path == "/api/token/":
			assert.Equal(t, "275", request.Header.Get("New-API-User"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":7,"name":"primary","key":"sk-newapi","models":["gpt-4o"]}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
		Password: "synthetic-password",
	})
	require.NoError(t, err)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, []string{"gpt-4o"}, snapshot.Keys[0].Models)
}

func TestNewAPIAdapterAdminKeySkipsPasswordLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/user/login" {
			t.Fatalf("admin key authentication must not submit a password login")
		}
		if request.URL.Path == "/api/user/self" {
			assert.Equal(t, "admin-secret", request.Header.Get("x-api-key"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":1}}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	_, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthAdminKey,
		AdminKey: "admin-secret",
	})
	require.NoError(t, err)
}

func TestPlatformSitePasswordAuthenticationReusesCachedAccessToken(t *testing.T) {
	tests := []struct {
		name        string
		newAdapter  func(*http.Client) PlatformSiteAdapter
		loginPath   string
		refreshPath string
		selfPath    string
		loginBody   string
		selfBody    string
	}{
		{
			name:        "NewAPI",
			newAdapter:  func(client *http.Client) PlatformSiteAdapter { return NewNewAPIAdapter(client) },
			loginPath:   "/api/user/login",
			refreshPath: "/api/user/auth/refresh",
			selfPath:    "/api/user/self",
			loginBody:   `{"success":true,"data":{"access_token":"session","refresh_token":"rotated"}}`,
			selfBody:    `{"success":true,"data":{"quota":1}}`,
		},
		{
			name:        "Sub2API",
			newAdapter:  func(client *http.Client) PlatformSiteAdapter { return NewSub2APIAdapter(client) },
			loginPath:   "/api/v1/auth/login",
			refreshPath: "/api/v1/auth/refresh",
			selfPath:    "/api/v1/auth/me",
			loginBody:   `{"code":0,"data":{"access_token":"session","refresh_token":"rotated"}}`,
			selfBody:    `{"code":0,"data":{"balance":1}}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case testCase.refreshPath:
					t.Fatalf("缓存访问令牌有效时不应尝试刷新令牌")
				case testCase.loginPath:
					t.Fatalf("缓存访问令牌有效时不应重新登录")
				case testCase.selfPath:
					assert.Equal(t, "Bearer stale-access", request.Header.Get("Authorization"))
					_, _ = writer.Write([]byte(testCase.selfBody))
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			credential := model.PlatformSiteCredential{
				AuthType:       model.UpstreamAuthPassword,
				Username:       "operator",
				Password:       "synthetic-password",
				AccessToken:    "stale-access",
				RefreshToken:   "stale-refresh",
				TokenExpiresAt: common.GetTimestamp() + 3600,
			}
			session, err := testCase.newAdapter(server.Client()).Authenticate(context.Background(), server.URL, credential)
			require.NoError(t, err)
			assert.Nil(t, session.CredentialUpdate)
		})
	}
}

func TestPlatformSitePasswordAuthenticationRefreshesExpiredCachedAccessToken(t *testing.T) {
	tests := []struct {
		name        string
		adapter     func(*http.Client) PlatformSiteAdapter
		refreshPath string
		loginPath   string
		currentPath string
		refreshBody string
		currentBody string
	}{
		{
			name:        "NewAPI",
			adapter:     func(client *http.Client) PlatformSiteAdapter { return NewNewAPIAdapter(client) },
			refreshPath: "/api/user/auth/refresh",
			loginPath:   "/api/user/login",
			currentPath: "/api/user/self",
			refreshBody: `{"success":true,"data":{"access_token":"refreshed-access","refresh_token":"refreshed-refresh","expires_in":3600}}`,
			currentBody: `{"success":true,"data":{"quota":1}}`,
		},
		{
			name:        "Sub2API",
			adapter:     func(client *http.Client) PlatformSiteAdapter { return NewSub2APIAdapter(client) },
			refreshPath: "/api/v1/auth/refresh",
			loginPath:   "/api/v1/auth/login",
			currentPath: "/api/v1/auth/me",
			refreshBody: `{"code":0,"data":{"access_token":"refreshed-access","refresh_token":"refreshed-refresh","expires_in":3600}}`,
			currentBody: `{"code":0,"data":{"balance":1}}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			refreshRequests := 0
			currentRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case testCase.loginPath:
					t.Fatalf("缓存访问令牌过期但刷新成功时不应重新登录")
				case testCase.refreshPath:
					refreshRequests++
					assert.Equal(t, "Bearer expired-access", request.Header.Get("Authorization"))
					_, _ = writer.Write([]byte(testCase.refreshBody))
				case testCase.currentPath:
					currentRequests++
					assert.Equal(t, "Bearer refreshed-access", request.Header.Get("Authorization"))
					_, _ = writer.Write([]byte(testCase.currentBody))
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			session, err := testCase.adapter(server.Client()).Authenticate(
				context.Background(),
				server.URL,
				model.PlatformSiteCredential{
					AuthType:       model.UpstreamAuthPassword,
					Username:       "operator",
					Password:       "synthetic-password",
					AccessToken:    "expired-access",
					RefreshToken:   "old-refresh",
					TokenExpiresAt: common.GetTimestamp() - 1,
				},
			)
			require.NoError(t, err)
			assert.Equal(t, 1, refreshRequests)
			assert.Equal(t, 1, currentRequests)
			require.NotNil(t, session.CredentialUpdate)
			assert.Equal(t, "operator", session.CredentialUpdate.Username)
			assert.Equal(t, "synthetic-password", session.CredentialUpdate.Password)
			assert.Equal(t, "refreshed-access", session.CredentialUpdate.AccessToken)
			assert.Equal(t, "refreshed-refresh", session.CredentialUpdate.RefreshToken)
		})
	}
}

func TestPlatformSitePasswordAuthenticationFallsBackToPasswordAndKeepsDualCredential(t *testing.T) {
	tests := []struct {
		name        string
		adapter     func(*http.Client) PlatformSiteAdapter
		refresh     string
		login       string
		current     string
		loginBody   string
		currentBody string
	}{
		{
			name:        "NewAPI",
			adapter:     func(client *http.Client) PlatformSiteAdapter { return NewNewAPIAdapter(client) },
			refresh:     "/api/user/auth/refresh",
			login:       "/api/user/login",
			current:     "/api/user/self",
			loginBody:   `{"success":true,"data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"user_id":42}}`,
			currentBody: `{"success":true,"data":{"quota":1}}`,
		},
		{
			name:        "Sub2API",
			adapter:     func(client *http.Client) PlatformSiteAdapter { return NewSub2APIAdapter(client) },
			refresh:     "/api/v1/auth/refresh",
			login:       "/api/v1/auth/login",
			current:     "/api/v1/auth/me",
			loginBody:   `{"code":0,"data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"user_id":42}}`,
			currentBody: `{"code":0,"data":{"balance":1}}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			loginRequests := 0
			refreshRequests := 0
			currentRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case testCase.refresh:
					refreshRequests++
					http.Error(writer, `{"success":false,"message":"expired"}`, http.StatusUnauthorized)
				case testCase.login:
					loginRequests++
					_, _ = writer.Write([]byte(testCase.loginBody))
				case testCase.current:
					currentRequests++
					if currentRequests == 1 {
						http.Error(writer, `{"success":false,"message":"expired"}`, http.StatusUnauthorized)
						return
					}
					assert.Equal(t, "Bearer new-access", request.Header.Get("Authorization"))
					_, _ = writer.Write([]byte(testCase.currentBody))
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			session, err := testCase.adapter(server.Client()).Authenticate(
				context.Background(),
				server.URL,
				model.PlatformSiteCredential{
					AuthType:       model.UpstreamAuthPassword,
					Username:       "operator",
					Password:       "synthetic-password",
					AccessToken:    "old-access",
					RefreshToken:   "old-refresh",
					TokenExpiresAt: common.GetTimestamp() + 3600,
				},
			)
			require.NoError(t, err)
			assert.Equal(t, 1, refreshRequests)
			assert.Equal(t, 1, loginRequests)
			require.NotNil(t, session.CredentialUpdate)
			assert.Equal(t, "operator", session.CredentialUpdate.Username)
			assert.Equal(t, "synthetic-password", session.CredentialUpdate.Password)
			assert.Equal(t, "new-access", session.CredentialUpdate.AccessToken)
			assert.Equal(t, "new-refresh", session.CredentialUpdate.RefreshToken)
			assert.Greater(t, session.CredentialUpdate.TokenExpiresAt, common.GetTimestamp())
		})
	}
}

func TestPlatformSitePasswordLoginClearsStaleRefreshTokenWhenRotationOmitsIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/refresh":
			http.Error(writer, `{"code":401,"message":"expired"}`, http.StatusUnauthorized)
		case "/api/v1/auth/login":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"new-access","expires_in":3600}}`))
		case "/api/v1/auth/me":
			if request.Header.Get("Authorization") == "Bearer old-access" {
				http.Error(writer, `{"code":401,"message":"expired"}`, http.StatusUnauthorized)
				return
			}
			assert.Equal(t, "Bearer new-access", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := NewSub2APIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType:       model.UpstreamAuthPassword,
			Username:       "operator",
			Password:       "synthetic-password",
			AccessToken:    "old-access",
			RefreshToken:   "old-refresh",
			TokenExpiresAt: common.GetTimestamp() + 3600,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, "new-access", session.CredentialUpdate.AccessToken)
	assert.Empty(t, session.CredentialUpdate.RefreshToken)
}

func TestSub2APIAdapterParsesRealLoginEnvelopeWithoutCredentialDrift(t *testing.T) {
	loginRequests := 0
	authMeRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/refresh":
			http.Error(writer, `{"code":401,"message":"expired"}`, http.StatusUnauthorized)
		case "/api/v1/auth/login":
			loginRequests++
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"email":"operator@example.com"`)
			assert.Contains(t, string(body), `"password":"synthetic-password"`)
			_, _ = writer.Write([]byte(`{
				"code": 0,
				"message": "success",
				"data": {
					"access_token": "synthetic-access-token",
					"refresh_token": "synthetic-refresh-token",
					"expires_in": 3600,
					"token_type": "bearer",
					"user": {}
				}
			}`))
		case "/api/v1/auth/me":
			authMeRequests++
			if authMeRequests == 1 {
				http.Error(writer, `{"code":401,"message":"expired"}`, http.StatusUnauthorized)
				return
			}
			assert.Equal(t, "Bearer synthetic-access-token", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":1}}`))
		case "/api/v1/keys":
			t.Fatalf("Authenticate 不应获取密钥列表")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	credential := model.PlatformSiteCredential{
		AuthType:       model.UpstreamAuthPassword,
		Username:       "operator@example.com",
		Password:       "synthetic-password",
		AccessToken:    "stale-access-token",
		RefreshToken:   "stale-refresh-token",
		TokenExpiresAt: common.GetTimestamp() + 3600,
	}
	session, err := NewSub2APIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		credential,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, loginRequests)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, "operator@example.com", session.CredentialUpdate.Username)
	assert.Equal(t, "synthetic-password", session.CredentialUpdate.Password)
	assert.Equal(t, "synthetic-access-token", session.CredentialUpdate.AccessToken)
	assert.Equal(t, "synthetic-refresh-token", session.CredentialUpdate.RefreshToken)
	assert.Greater(t, session.CredentialUpdate.TokenExpiresAt, common.GetTimestamp())
}

func TestPlatformSiteAccessTokenAuthenticationDoesNotFallbackToPassword(t *testing.T) {
	tests := []struct {
		name        string
		newAdapter  func(*http.Client) PlatformSiteAdapter
		refreshPath string
		loginPath   string
	}{
		{
			name:        "NewAPI",
			newAdapter:  func(client *http.Client) PlatformSiteAdapter { return NewNewAPIAdapter(client) },
			refreshPath: "/api/user/auth/refresh",
			loginPath:   "/api/user/login",
		},
		{
			name:        "Sub2API",
			newAdapter:  func(client *http.Client) PlatformSiteAdapter { return NewSub2APIAdapter(client) },
			refreshPath: "/api/v1/auth/refresh",
			loginPath:   "/api/v1/auth/login",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case testCase.loginPath:
					t.Fatalf("访问令牌刷新失败后不应回退账号密码")
				case testCase.refreshPath:
					http.Error(writer, `{"success":false}`, http.StatusUnauthorized)
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			_, err := testCase.newAdapter(server.Client()).Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
				AuthType:       model.UpstreamAuthAccessToken,
				Username:       "operator",
				Password:       "synthetic-password",
				AccessToken:    "stale-access",
				RefreshToken:   "stale-refresh",
				TokenExpiresAt: common.GetTimestamp() + 3600,
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPlatformSiteAuth)
		})
	}
}

func TestNewAPIAdapterRefreshesRotatingSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/auth/refresh":
			assert.Equal(t, "Bearer old-access", request.Header.Get("Authorization"))
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"refresh_token":"old-refresh"`)
			_, _ = writer.Write([]byte(`{"success":true,"data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}}`))
		case "/api/user/self":
			assert.Equal(t, "Bearer new-access", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType:       model.UpstreamAuthAccessToken,
		AccessToken:    "old-access",
		RefreshToken:   "old-refresh",
		TokenExpiresAt: common.GetTimestamp() - 1,
	})
	require.NoError(t, err)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, "new-access", session.CredentialUpdate.AccessToken)
	assert.Equal(t, "new-refresh", session.CredentialUpdate.RefreshToken)
	assert.Greater(t, session.CredentialUpdate.TokenExpiresAt, common.GetTimestamp())
}

func TestNewAPIAdapterRejectsInteractiveLoginVerification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/user/login" {
			_, _ = writer.Write([]byte(`{"success":true,"data":{"require_2fa":true,"flow_token":"flow"}}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	_, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
		Password: "synthetic-password",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteAuth)
	assert.NotContains(t, err.Error(), "flow")
}

func TestSub2APIAdapterRefreshesRotatingSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/refresh":
			assert.Equal(t, "Bearer old-access", request.Header.Get("Authorization"))
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"refresh_token":"old-refresh"`)
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}}`))
		case "/api/v1/auth/me":
			assert.Equal(t, "Bearer new-access", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType:       model.UpstreamAuthAccessToken,
		AccessToken:    "old-access",
		RefreshToken:   "old-refresh",
		TokenExpiresAt: common.GetTimestamp() - 1,
	})
	require.NoError(t, err)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, "new-access", session.CredentialUpdate.AccessToken)
	assert.Equal(t, "new-refresh", session.CredentialUpdate.RefreshToken)
}

func TestSub2APIAdapterAdminKeyReadsNestedCredentialAndPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			assert.Equal(t, "admin-secret", request.Header.Get("x-api-key"))
			_, _ = writer.Write([]byte(`{"data":{"balance":4,"used_quota":1}}`))
		case "/api/v1/groups/rates":
			_, _ = writer.Write([]byte(`{"data":{"default":0.5}}`))
		case "/v1/models":
			_, _ = writer.Write([]byte(`{"data":[{"id":"gpt-4o"}]}`))
		case "/api/v1/admin/accounts":
			assert.Equal(t, "apikey", request.URL.Query().Get("type"))
			assert.Equal(t, "1", request.URL.Query().Get("page"))
			assert.Equal(t, "name", request.URL.Query().Get("sort_by"))
			assert.Equal(t, "asc", request.URL.Query().Get("sort_order"))
			_, _ = writer.Write([]byte(`{"data":{"accounts":[{"id":"account-1","name":"managed","group":"default","quota":9}],"total":1,"page_size":100}}`))
		case "/api/v1/admin/accounts/data":
			_, _ = writer.Write([]byte(`{"data":{"accounts":[{"id":"account-1","credentials":{"api_key":"sk-sub2api"}}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthAdminKey,
		AdminKey: "admin-secret",
	})
	require.NoError(t, err)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, float64(4), snapshot.Balance)
	assert.Equal(t, int64(500000), snapshot.UsedQuota)
	assert.True(t, snapshot.UsedQuotaSet)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "sk-sub2api", snapshot.Keys[0].Secret)
	assert.Equal(t, 0.5, snapshot.Keys[0].ConversionRatio)
	assert.Equal(t, []string{"gpt-4o"}, snapshot.Keys[0].Models)
}

func TestSub2APIAdapterUsesProfileUsageGroupAliasesAndModelAliases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"sub2api-session"}}`))
		case "/api/v1/auth/me":
			assert.Equal(t, "Bearer sub2api-session", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":9,"balance":0}}`))
		case "/api/v1/user/profile":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":6.5}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"totalActualCost":"2"}}`))
		case "/api/v1/groups/available":
			_, _ = writer.Write([]byte(`{"code":0,"data":[{"id":"group-1","name":"default","rate_multiplier":0.25}]}`))
		case "/api/v1/groups/rates":
			_, _ = writer.Write([]byte(`{"code":0,"data":{}}`))
		case "/api/v1/keys":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[{"id":"key-1","name":"primary","key":"sk-sub2api","group":{"id":"group-1","name":"default"},"model_limits":"gpt-4o,gemini-2.5-pro","quota":9,"quota_used":2}],"total":1,"page_size":100}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator@example.com",
		Password: "synthetic-password",
	})
	require.NoError(t, err)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, 6.5, snapshot.Balance)
	assert.Equal(t, int64(1000000), snapshot.UsedQuota)
	assert.True(t, snapshot.UsedQuotaSet)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "default", snapshot.Keys[0].Group)
	assert.Equal(t, 0.25, snapshot.Keys[0].SourceConversionRatio)
	assert.Equal(t, []string{"gpt-4o", "gemini-2.5-pro"}, snapshot.Keys[0].Models)
	assert.True(t, snapshot.Keys[0].ModelsSynced)
	assert.Equal(t, int64(1000000), snapshot.Keys[0].UsedQuota)
	assert.True(t, snapshot.Keys[0].UsedQuotaSet)
	require.NotNil(t, snapshot.Keys[0].RemainQuota)
	assert.Equal(t, int64(3500000), *snapshot.Keys[0].RemainQuota)
}

func TestSub2APIAdapterUsesAccountQuotaPriorityAndKeyFallback(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	testCases := []struct {
		name             string
		me               string
		profile          string
		usage            string
		keys             string
		wantUsedQuota    int64
		wantUsedQuotaSet bool
	}{
		{
			name:             "current user takes priority",
			me:               `{"used_quota":"6"}`,
			profile:          `{"used_quota":"2"}`,
			usage:            `{"total_actual_cost":"3","today_actual_cost":"4"}`,
			wantUsedQuota:    3000000,
			wantUsedQuotaSet: true,
		},
		{
			name:             "profile takes priority over dashboard",
			me:               `{}`,
			profile:          `{"used_quota":"2.5"}`,
			usage:            `{"total_actual_cost":"3","today_actual_cost":"4"}`,
			wantUsedQuota:    1250000,
			wantUsedQuotaSet: true,
		},
		{
			name:             "cumulative dashboard value takes priority over daily value",
			me:               `{}`,
			profile:          `{}`,
			usage:            `{"total_actual_cost":"3.25","today_actual_cost":"4"}`,
			wantUsedQuota:    1625000,
			wantUsedQuotaSet: true,
		},
		{
			name:             "zero account value is preserved",
			me:               `{"used_quota":0}`,
			profile:          `{"used_quota":"2"}`,
			usage:            `{"total_actual_cost":"3"}`,
			wantUsedQuota:    0,
			wantUsedQuotaSet: true,
		},
		{
			name:    "missing account values leave key aggregation to persistence",
			me:      `{}`,
			profile: `{}`,
			usage:   `{}`,
			keys: `{"items":[
				{"id":"key-1","name":"one","key":"sk-one","models":["gpt-4o"],"quota":10,"quota_used":"1.5"},
				{"id":"key-2","name":"two","key":"sk-two","models":["gpt-4o"],"quota":5,"quota_used":2}
			],"total":2,"page_size":100}`,
			wantUsedQuota:    0,
			wantUsedQuotaSet: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/api/v1/auth/login":
					_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"sub2api-session"}}`))
				case "/api/v1/auth/me":
					_, _ = writer.Write([]byte(`{"code":0,"data":` + testCase.me + `}`))
				case "/api/v1/user/profile":
					_, _ = writer.Write([]byte(`{"code":0,"data":` + testCase.profile + `}`))
				case "/api/v1/usage/dashboard/stats":
					_, _ = writer.Write([]byte(`{"code":0,"data":` + testCase.usage + `}`))
				case "/api/v1/groups/available", "/api/v1/groups/rates":
					_, _ = writer.Write([]byte(`{"code":0,"data":[]}`))
				case "/api/v1/keys":
					keys := testCase.keys
					if keys == "" {
						keys = `{"items":[{"id":"key-1","name":"one","key":"sk-one","models":["gpt-4o"],"quota":10,"quota_used":"1.5"}],"total":1,"page_size":100}`
					}
					_, _ = writer.Write([]byte(`{"code":0,"data":` + keys + `}`))
				default:
					http.NotFound(writer, request)
				}
			}))
			t.Cleanup(server.Close)

			session, err := NewSub2APIAdapter(server.Client()).Authenticate(
				context.Background(),
				server.URL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: "operator@example.com",
					Password: "synthetic-password",
				},
			)
			require.NoError(t, err)

			snapshot, err := NewSub2APIAdapter(server.Client()).FetchSnapshot(context.Background(), session)
			require.NoError(t, err)
			assert.Equal(t, testCase.wantUsedQuota, snapshot.UsedQuota)
			assert.Equal(t, testCase.wantUsedQuotaSet, snapshot.UsedQuotaSet)
			require.NotEmpty(t, snapshot.Keys)
			assert.Equal(t, int64(750000), snapshot.Keys[0].UsedQuota)
			assert.True(t, snapshot.Keys[0].UsedQuotaSet)
			require.NotNil(t, snapshot.Keys[0].RemainQuota)
			assert.Equal(t, int64(4250000), *snapshot.Keys[0].RemainQuota)
			if len(snapshot.Keys) > 1 {
				assert.Equal(t, int64(1000000), snapshot.Keys[1].UsedQuota)
				require.NotNil(t, snapshot.Keys[1].RemainQuota)
				assert.Equal(t, int64(1500000), *snapshot.Keys[1].RemainQuota)
			}
		})
	}
}

func TestSub2APIAdapterDiscoversRelayModelsAndTreatsZeroQuotaAsUnlimited(t *testing.T) {
	loginAttempts := 0
	modelRequests := 0
	client := &http.Client{
		Transport: platformSiteRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch {
			case request.Method == http.MethodGet && (request.URL.Path == "" || request.URL.Path == "/"):
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader(`<script>window.__APP_CONFIG__={"api_base_url":"https://example.com/v1"}</script>`)),
				}, nil
			case request.Method == http.MethodPost && request.URL.Path == "/api/v1/auth/login":
				body, readErr := io.ReadAll(request.Body)
				require.NoError(t, readErr)
				loginAttempts++
				if strings.Contains(string(body), `"email"`) &&
					!strings.Contains(string(body), `"username"`) {
					return platformSiteJSONResponse(http.StatusUnauthorized, `{"code":401,"message":"email field is unsupported"}`), nil
				}
				return platformSiteJSONResponse(http.StatusOK, `{"code":0,"data":{"access_token":"sub2api-session"}}`), nil
			case request.Method == http.MethodGet && request.URL.Path == "/api/v1/auth/me":
				return platformSiteJSONResponse(http.StatusOK, `{"code":0,"data":{"balance":3}}`), nil
			case request.Method == http.MethodGet && request.URL.Path == "/api/v1/user/profile":
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
			case request.Method == http.MethodGet && request.URL.Path == "/api/v1/usage/dashboard/stats":
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
			case request.Method == http.MethodGet && request.URL.Path == "/api/v1/groups/available":
				return platformSiteJSONResponse(http.StatusOK, `{"code":0,"data":[]}`), nil
			case request.Method == http.MethodGet && request.URL.Path == "/api/v1/groups/rates":
				return platformSiteJSONResponse(http.StatusOK, `{"code":0,"data":{}}`), nil
			case request.Method == http.MethodGet && request.URL.Path == "/api/v1/keys":
				return platformSiteJSONResponse(http.StatusOK, `{"code":0,"data":{"items":[{"id":"key-1","name":"unlimited","key":"sk-sub2api","quota":0,"quota_used":42}],"total":1,"page_size":100}}`), nil
			case request.Method == http.MethodGet && request.URL.Path == "/v1/models":
				modelRequests++
				assert.Equal(t, "Bearer sk-sub2api", request.Header.Get("Authorization"))
				assert.Equal(t, "sk-sub2api", request.Header.Get("x-api-key"))
				return platformSiteJSONResponse(http.StatusOK, `{"data":[{"id":"gpt-5.5"}]}`), nil
			default:
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
			}
		}),
	}
	adapter := NewSub2APIAdapter(client)
	session, err := adapter.Authenticate(context.Background(), "https://example.com/", model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator@example.com",
		Password: "synthetic-password",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/v1", session.ModelBaseURL)
	assert.GreaterOrEqual(t, loginAttempts, 2)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	require.Len(t, snapshot.Keys, 1)
	assert.Nil(t, snapshot.Keys[0].RemainQuota)
	assert.Equal(t, []string{"gpt-5.5"}, snapshot.Keys[0].Models)
	assert.True(t, snapshot.Keys[0].ModelsSynced)
	assert.Equal(t, 1, modelRequests)
}

func TestSub2APIAdapterResolvesRelativeRelayURLFromPageConfig(t *testing.T) {
	client := &http.Client{
		Transport: platformSiteRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch {
			case request.Method == http.MethodGet && (request.URL.Path == "" || request.URL.Path == "/"):
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader(`<script>window.__APP_CONFIG__={"api_base_url":"/v1"}</script>`)),
				}, nil
			case request.Method == http.MethodPost && request.URL.Path == "/api/v1/auth/login":
				return platformSiteJSONResponse(http.StatusOK, `{"code":0,"message":"success","data":{"access_token":"sub2api-session"}}`), nil
			case request.Method == http.MethodGet && request.URL.Path == "/api/v1/auth/me":
				return platformSiteJSONResponse(http.StatusOK, `{"code":0,"message":"success","data":{"balance":3}}`), nil
			default:
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404,"message":"not found"}`), nil
			}
		}),
	}

	session, err := NewSub2APIAdapter(client).Authenticate(
		context.Background(),
		"https://example.com/",
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/v1", session.ModelBaseURL)
}

func TestSub2APIAdapterDiscoversManagementURLFromStrictAPISubdomain(t *testing.T) {
	var candidateLoginRequests int
	client := &http.Client{
		Transport: platformSiteRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch {
			case request.URL.Host == "api.example.com" &&
				request.Method == http.MethodGet &&
				(request.URL.Path == "" || request.URL.Path == "/"):
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
			case request.URL.Host == "example.com" &&
				request.Method == http.MethodGet &&
				(request.URL.Path == "" || request.URL.Path == "/"):
				assert.Empty(t, request.Header.Get("Authorization"))
				assert.Empty(t, request.Header.Get("Cookie"))
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
					Body: io.NopCloser(strings.NewReader(
						`<script>window.__APP_CONFIG__={"api_base_url":"https://api.example.com/v1"}</script>`,
					)),
				}, nil
			case request.URL.Host == "example.com" &&
				request.Method == http.MethodPost &&
				request.URL.Path == "/api/v1/auth/login":
				candidateLoginRequests++
				assert.Empty(t, request.Header.Get("Cookie"))
				return platformSiteJSONResponse(
					http.StatusOK,
					`{"code":0,"data":{"access_token":"sub2api-session"}}`,
				), nil
			case request.URL.Host == "example.com" &&
				request.Method == http.MethodGet &&
				request.URL.Path == "/api/v1/auth/me":
				assert.Equal(t, "Bearer sub2api-session", request.Header.Get("Authorization"))
				return platformSiteJSONResponse(http.StatusOK, `{"code":0,"data":{"balance":3}}`), nil
			default:
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
			}
		}),
	}

	session, err := NewSub2APIAdapter(client).Authenticate(
		context.Background(),
		"https://api.example.com",
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", session.BaseURL)
	assert.Equal(t, "https://example.com", session.ManagementBaseURL)
	assert.Equal(t, "https://api.example.com/v1", session.ModelBaseURL)
	assert.Equal(t, 1, candidateLoginRequests)
}

func TestSub2APIAdapterDoesNotUseUnverifiedManagementCandidate(t *testing.T) {
	var candidateLoginRequests int
	client := &http.Client{
		Transport: platformSiteRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch {
			case request.URL.Host == "api.example.com" &&
				request.Method == http.MethodGet &&
				(request.URL.Path == "" || request.URL.Path == "/"):
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
			case request.URL.Host == "example.com" &&
				request.Method == http.MethodGet &&
				(request.URL.Path == "" || request.URL.Path == "/"):
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body: io.NopCloser(strings.NewReader(
						`<script>window.__APP_CONFIG__={"api_base_url":"https://other.example.com/v1"}</script>`,
					)),
				}, nil
			case request.URL.Host == "example.com" &&
				request.Method == http.MethodPost &&
				request.URL.Path == "/api/v1/auth/login":
				candidateLoginRequests++
				return platformSiteJSONResponse(http.StatusUnauthorized, `{"code":401}`), nil
			case request.URL.Host == "api.example.com" &&
				request.Method == http.MethodPost &&
				request.URL.Path == "/api/v1/auth/login":
				return platformSiteJSONResponse(
					http.StatusOK,
					`{"code":0,"data":{"access_token":"sub2api-session"}}`,
				), nil
			case request.URL.Host == "api.example.com" &&
				request.Method == http.MethodGet &&
				request.URL.Path == "/api/v1/auth/me":
				return platformSiteJSONResponse(http.StatusOK, `{"code":0,"data":{"balance":3}}`), nil
			default:
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
			}
		}),
	}

	session, err := NewSub2APIAdapter(client).Authenticate(
		context.Background(),
		"https://api.example.com",
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com", session.BaseURL)
	assert.Equal(t, "https://api.example.com", session.ManagementBaseURL)
	assert.Zero(t, candidateLoginRequests)
}

func TestSafePlatformSiteErrorIncludesHTTPStatusWithoutResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && (request.URL.Path == "" || request.URL.Path == "/") {
			writer.Header().Set("Content-Type", "text/html")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte("secret upstream response body"))
			return
		}
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte("secret login response body"))
	}))
	defer server.Close()

	_, err := NewSub2APIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.Error(t, err)
	message := SafePlatformSiteError(err)
	assert.Contains(t, message, "HTTP 404")
	assert.NotContains(t, message, "secret upstream response body")
	assert.NotContains(t, message, "secret login response body")
}

func TestSub2APILoginResponseDiagnosticsClassifyNonJSONResponses(t *testing.T) {
	tests := []struct {
		name              string
		contentType       string
		expectedMediaType string
		body              string
		responseType      string
	}{
		{
			name:              "html",
			contentType:       "text/html; charset=utf-8",
			expectedMediaType: "text/html",
			body:              "<!doctype html><html><body>SECRET_RESPONSE_BODY synthetic-password session-cookie synthetic-access-token</body></html>",
			responseType:      "html",
		},
		{
			name:              "plain text",
			contentType:       "text/plain; charset=utf-8",
			expectedMediaType: "text/plain",
			body:              "SECRET_RESPONSE_BODY synthetic-password session-cookie synthetic-access-token",
			responseType:      "plain_text",
		},
		{
			name:              "invalid json",
			contentType:       "application/json",
			expectedMediaType: "application/json",
			body:              "SECRET_RESPONSE_BODY synthetic-password session-cookie synthetic-access-token",
			responseType:      "invalid_json",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodPost {
					writer.Header().Set("Content-Type", testCase.contentType)
					writer.WriteHeader(http.StatusOK)
					_, _ = writer.Write([]byte(testCase.body))
					return
				}
				writer.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			_, err := NewSub2APIAdapter(server.Client()).Authenticate(
				context.Background(),
				server.URL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: "operator@example.com",
					Password: "synthetic-password",
				},
			)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrSub2APILoginResponse)
			var responseErr *platformSiteResponseError
			require.ErrorAs(t, err, &responseErr)
			assert.Equal(t, testCase.responseType, responseErr.diagnostics.responseType)
			assert.Equal(t, http.StatusOK, responseErr.diagnostics.statusCode)
			assert.Equal(t, server.URL+"/api/v1/auth/login", responseErr.diagnostics.finalURL)
			assert.False(t, responseErr.diagnostics.redirected)

			message := SafePlatformSiteError(err)
			assert.Contains(t, message, "HTTP 200")
			assert.Contains(t, message, testCase.responseType)
			assert.Contains(t, message, testCase.expectedMediaType)
			assert.Contains(t, message, server.URL+"/api/v1/auth/login")
			assert.Contains(t, message, "未发生重定向")
			assert.NotContains(t, message, "SECRET_RESPONSE_BODY")
			assert.NotContains(t, message, "synthetic-password")
			assert.NotContains(t, message, "session-cookie")
			assert.NotContains(t, message, "synthetic-access-token")
		})
	}
}

func TestSub2APILoginResponseDiagnosticsTracksRedirectWithoutQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost:
			writer.Header().Set("Location", "/challenge?token=SECRET_REDIRECT_TOKEN")
			writer.WriteHeader(http.StatusFound)
		case request.URL.Path == "/challenge":
			writer.Header().Set("Content-Type", "text/html")
			_, _ = writer.Write([]byte("<html>verification page</html>"))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := newPlatformSiteHTTPClient()
	require.NoError(t, err)
	_, err = NewSub2APIAdapter(client).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSub2APILoginResponse)
	var responseErr *platformSiteResponseError
	require.ErrorAs(t, err, &responseErr)
	assert.Equal(t, "html", responseErr.diagnostics.responseType)
	assert.True(t, responseErr.diagnostics.redirected)
	assert.Equal(t, server.URL+"/challenge", responseErr.diagnostics.finalURL)

	message := SafePlatformSiteError(err)
	assert.Contains(t, message, "发生重定向")
	assert.Contains(t, message, server.URL+"/challenge")
	assert.NotContains(t, message, "SECRET_REDIRECT_TOKEN")
	assert.NotContains(t, message, "synthetic-password")
}

func TestSub2APILoginJSONAuthErrorKeepsHTTPClassification(t *testing.T) {
	requestedPaths := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			requestedPaths = append(requestedPaths, request.URL.Path)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte(`{"code":401,"message":"invalid credentials SECRET_RESPONSE_BODY"}`))
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := NewSub2APIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSub2APILoginHTTPStatus)
	assert.NotErrorIs(t, err, ErrSub2APILoginResponse)
	assert.NotEmpty(t, requestedPaths)
	for _, path := range requestedPaths {
		assert.Equal(t, "/api/v1/auth/login", path)
	}
	message := SafePlatformSiteError(err)
	assert.Contains(t, message, "HTTP 401")
	assert.NotContains(t, message, "SECRET_RESPONSE_BODY")
	assert.NotContains(t, message, "synthetic-password")
}

func TestSub2APILoginInteractiveErrorStopsFallbackPaths(t *testing.T) {
	requestedPaths := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		requestedPaths = append(requestedPaths, request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{
			"code": 400,
			"message": "turnstile verification failed",
			"reason": "TURNSTILE_VERIFICATION_FAILED",
			"debug": "SECRET_RESPONSE_BODY synthetic-password session-cookie synthetic-access-token"
		}`))
	}))
	defer server.Close()

	_, err := NewSub2APIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSub2APILoginInteractive)
	require.ErrorIs(t, err, ErrSub2APILoginHTTPStatus)
	assert.NotErrorIs(t, err, ErrSub2APILoginResponse)
	assert.Equal(t, []string{"/api/v1/auth/login"}, requestedPaths)

	message := SafePlatformSiteError(err)
	assert.Contains(t, message, "Turnstile")
	assert.Contains(t, message, "HTTP 400")
	assert.Contains(t, message, "TURNSTILE_VERIFICATION_FAILED")
	assert.Contains(t, message, "后台不会自动绕过")
	assert.NotContains(t, message, "SECRET_RESPONSE_BODY")
	assert.NotContains(t, message, "synthetic-password")
	assert.NotContains(t, message, "session-cookie")
	assert.NotContains(t, message, "synthetic-access-token")
}

func TestSub2APILoginTriesNextPathOnlyWhenRouteIsMissing(t *testing.T) {
	requestedPaths := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		requestedPaths = append(requestedPaths, request.URL.Path)
		switch request.URL.Path {
		case "/api/v1/auth/login", "/api/auth/login":
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"code":404,"reason":"ROUTE_NOT_FOUND"}`))
		case "/auth/login":
			writer.Header().Set("Content-Type", "text/html")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte("<html>login page</html>"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := NewSub2APIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSub2APILoginResponse)
	assert.Equal(t, []string{
		"/api/v1/auth/login",
		"/api/auth/login",
		"/auth/login",
	}, requestedPaths)
	message := SafePlatformSiteError(err)
	assert.Contains(t, message, "响应类型：html")
	assert.Contains(t, message, "HTTP 200")
	assert.NotContains(t, message, "synthetic-password")
}

func TestSub2APIAdapterResolvesMaskedKeyFromKeyDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":1}}`))
		case "/api/v1/keys":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[{"id":"key-1","name":"masked","key":"sk-****"}]}}`))
		case "/api/v1/keys/key-1":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":"key-1","key":"sk-detail","models":["gpt-5.5"]}}`))
		case "/v1/models":
			assert.Equal(t, "Bearer sk-detail", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"data":[{"id":"gpt-5.5"}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := NewSub2APIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType:    model.UpstreamAuthAccessToken,
			AccessToken: "session-token",
		},
	)
	require.NoError(t, err)

	snapshot, err := NewSub2APIAdapter(server.Client()).FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "sk-detail", snapshot.Keys[0].Secret)
	assert.True(t, snapshot.Keys[0].ModelsSynced)
	assert.Equal(t, []string{"gpt-5.5"}, snapshot.Keys[0].Models)
}

func TestNewAPIAdapterReturnsUnavailableKeyWhenKeyRevealFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":12.5}}`))
		case "/api/user/models":
			_, _ = writer.Write([]byte(`{"success":true,"data":["gpt-4o"]}`))
		case "/api/token/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":7,"name":"primary","key":"sk-****"}]}}`))
		case "/api/token/7/key":
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"success":false,"message":"verification required"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType:    model.UpstreamAuthAccessToken,
		AccessToken: "session-token",
	})
	require.NoError(t, err)
	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "7", snapshot.Keys[0].ExternalID)
	assert.Equal(t, upstreamKeySyncErrorSecretUnavailable, snapshot.Keys[0].SyncError)
	assert.Empty(t, snapshot.Keys[0].Secret)
}

func TestPersistPlatformSiteSnapshotIsolatesUnavailableKeys(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-site-isolation-test-secret"
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.UpstreamKey{},
		&model.UpstreamKeyAbility{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	priority := int64(3)
	channel := &model.Channel{
		Id:           12,
		Name:         "site",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
		Priority:     &priority,
	}
	require.NoError(t, db.Create(channel).Error)
	account := &model.PlatformSiteAccount{
		ChannelID:       channel.Id,
		Platform:        model.PlatformNewAPI,
		BaseURL:         "https://upstream.example",
		ConversionRatio: 0.1,
	}
	require.NoError(t, db.Create(account).Error)

	oldSecret, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{AccessToken: "sk-old"})
	require.NoError(t, err)
	oldKey := &model.UpstreamKey{
		ChannelID:        channel.Id,
		ExternalID:       "old-key",
		Name:             "old",
		SecretCiphertext: oldSecret,
		Models:           "gpt-4o",
		Status:           model.UpstreamKeyStatusEnabled,
	}
	require.NoError(t, db.Create(oldKey).Error)
	require.NoError(t, db.Create(&model.UpstreamKeyAbility{
		UpstreamKeyID: oldKey.ID,
		Group:         "default",
		Model:         "gpt-4o",
		Enabled:       true,
	}).Error)

	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance:   7,
		UsedQuota: 123456,
		Models:    []string{"gpt-4o"},
		Keys: []UpstreamKeySnapshot{
			{
				ExternalID: "old-key",
				Name:       "old",
				Models:     []string{"gpt-4o"},
				SyncError:  upstreamKeySyncErrorSecretUnavailable,
			},
			{
				ExternalID: "new-key",
				Name:       "new",
				Models:     []string{"gpt-4o"},
				SyncError:  upstreamKeySyncErrorSecretUnavailable,
			},
			{
				ExternalID:   "healthy-key",
				Name:         "healthy",
				Secret:       "sk-healthy",
				Group:        "default",
				Models:       []string{"gpt-4o"},
				ModelsSynced: true,
			},
		},
	}))

	var savedOld model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "old-key").First(&savedOld).Error)
	assert.Equal(t, model.UpstreamKeyStatusAutoDisabled, savedOld.Status)
	assert.Equal(t, upstreamKeySyncErrorSecretUnavailable, savedOld.DisabledReason)
	assert.Equal(t, "gpt-4o", savedOld.Models)
	oldCredential, err := model.DecryptPlatformSiteCredential(savedOld.SecretCiphertext)
	require.NoError(t, err)
	assert.Equal(t, "sk-old", oldCredential.AccessToken)
	var oldAbilityCount int64
	require.NoError(t, db.Model(&model.UpstreamKeyAbility{}).
		Where("upstream_key_id = ?", savedOld.ID).
		Count(&oldAbilityCount).Error)
	assert.Zero(t, oldAbilityCount)

	var newKey model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "new-key").First(&newKey).Error)
	assert.Equal(t, model.UpstreamKeyStatusAutoDisabled, newKey.Status)
	assert.Equal(t, upstreamKeySyncErrorSecretUnavailable, newKey.DisabledReason)
	assert.False(t, newKey.ModelsSynced)
	assert.Empty(t, newKey.SecretCiphertext)
	var newAbilityCount int64
	require.NoError(t, db.Model(&model.UpstreamKeyAbility{}).
		Where("upstream_key_id = ?", newKey.ID).
		Count(&newAbilityCount).Error)
	assert.Zero(t, newAbilityCount)

	var healthyKey model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "healthy-key").First(&healthyKey).Error)
	healthyCredential, err := model.DecryptPlatformSiteCredential(healthyKey.SecretCiphertext)
	require.NoError(t, err)
	assert.Equal(t, "sk-healthy", healthyCredential.AccessToken)

	var savedAccount model.PlatformSiteAccount
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedAccount).Error)
	assert.Equal(t, 7.0, savedAccount.Balance)
	assert.Equal(t, int64(123456), savedAccount.UsedQuota)

	var savedChannel model.Channel
	require.NoError(t, db.First(&savedChannel, channel.Id).Error)
	assert.Equal(t, 7.0, savedChannel.Balance)
	assert.Equal(t, int64(123456), savedChannel.UsedQuota)
}

func TestPersistPlatformSiteSnapshotSeparatesSub2APIManagementAndRelayURLs(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.UpstreamKey{},
		&model.UpstreamKeyAbility{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	channel := &model.Channel{
		Name:         "Sub2API site",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
	}
	require.NoError(t, db.Create(channel).Error)
	account := &model.PlatformSiteAccount{
		ChannelID:       channel.Id,
		Platform:        model.PlatformSub2API,
		BaseURL:         "https://api.example.com",
		RelayBaseURL:    "https://api.example.com",
		ConversionRatio: 0.1,
	}
	require.NoError(t, db.Create(account).Error)

	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance:           8,
		UsedQuota:         21,
		UsedQuotaSet:      true,
		ManagementBaseURL: "https://example.com",
		RelayBaseURL:      "https://api.example.com/v1",
	}))

	var savedAccount model.PlatformSiteAccount
	require.NoError(t, db.First(&savedAccount, account.ID).Error)
	assert.Equal(t, "https://example.com", savedAccount.BaseURL)
	assert.Equal(t, "https://api.example.com/v1", savedAccount.RelayBaseURL)
	assert.Equal(t, int64(21), savedAccount.UsedQuota)

	var savedChannel model.Channel
	require.NoError(t, db.First(&savedChannel, channel.Id).Error)
	require.NotNil(t, savedChannel.BaseURL)
	assert.Equal(t, "https://api.example.com", *savedChannel.BaseURL)
	assert.Equal(t, int64(21), savedChannel.UsedQuota)
}

func TestPersistPlatformSiteCredentialStoresOnlyEncryptedRotatedValues(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-site-credential-update-test-secret"
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformSiteAccount{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	account := &model.PlatformSiteAccount{
		ChannelID:            15,
		Platform:             model.PlatformSub2API,
		BaseURL:              "https://upstream.example",
		AuthType:             model.UpstreamAuthAccessToken,
		CredentialCiphertext: "old-ciphertext",
		CredentialKeyVersion: "v1",
	}
	require.NoError(t, db.Create(account).Error)
	credential := model.PlatformSiteCredential{
		AccessToken:    "rotated-access",
		RefreshToken:   "rotated-refresh",
		TokenExpiresAt: common.GetTimestamp() + 3600,
	}
	require.NoError(t, persistPlatformSiteCredential(account, credential))

	var saved model.PlatformSiteAccount
	require.NoError(t, db.First(&saved, account.ID).Error)
	assert.NotContains(t, saved.CredentialCiphertext, credential.AccessToken)
	assert.NotContains(t, saved.CredentialCiphertext, credential.RefreshToken)
	assert.NotEqual(t, "old-ciphertext", saved.CredentialCiphertext)
	decrypted, err := model.DecryptPlatformSiteCredential(saved.CredentialCiphertext)
	require.NoError(t, err)
	assert.Equal(t, model.UpstreamAuthAccessToken, decrypted.AuthType)
	assert.Equal(t, credential.AccessToken, decrypted.AccessToken)
	assert.Equal(t, credential.RefreshToken, decrypted.RefreshToken)
	assert.Equal(t, credential.TokenExpiresAt, decrypted.TokenExpiresAt)
}

func TestSyncPlatformSiteFailurePreservesLastSuccessfulSnapshot(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.CryptoSecret = "upstream-site-sync-failure-test-secret"
	common.MemoryCacheEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.UpstreamKey{},
		&model.UpstreamKeyAbility{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	failSync := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if failSync && request.URL.Path == "/api/user/self" {
			http.Error(writer, `{"success":false}`, http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":5000000,"used_quota":1000000}}`))
		case "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/token/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":7,"name":"primary","key":"sk-sync-stable","models":["gpt-5.5"]}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	previousHTTPClient := httpClient
	previousProtectedHTTPClient := ssrfProtectedHTTPClient
	previousFetchSetting := *system_setting.GetFetchSetting()
	httpClient = server.Client()
	ssrfProtectedHTTPClient = server.Client()
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	t.Cleanup(func() {
		httpClient = previousHTTPClient
		ssrfProtectedHTTPClient = previousProtectedHTTPClient
		*system_setting.GetFetchSetting() = previousFetchSetting
	})

	channel := &model.Channel{
		Id:           901,
		Name:         "sync-reuse",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
	}
	require.NoError(t, db.Create(channel).Error)
	credentialCiphertext, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{
		AuthType:    model.UpstreamAuthAccessToken,
		AccessToken: "session-token",
	})
	require.NoError(t, err)
	account := &model.PlatformSiteAccount{
		ChannelID:            channel.Id,
		Platform:             model.PlatformNewAPI,
		BaseURL:              server.URL,
		AuthType:             model.UpstreamAuthAccessToken,
		CredentialCiphertext: credentialCiphertext,
		ConversionRatio:      0.1,
		SyncStatus:           model.UpstreamSiteSyncIdle,
	}
	require.NoError(t, db.Create(account).Error)

	require.NoError(t, SyncUpstreamSite(context.Background(), channel.Id))
	var savedAccount model.PlatformSiteAccount
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedAccount).Error)
	require.Equal(t, model.UpstreamSiteSyncSuccess, savedAccount.SyncStatus)
	require.NotZero(t, savedAccount.LastSyncAt)
	successfulSyncAt := savedAccount.LastSyncAt

	var savedKey model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedKey).Error)
	var savedAbility model.UpstreamKeyAbility
	require.NoError(t, db.Where("upstream_key_id = ?", savedKey.ID).First(&savedAbility).Error)
	successfulBalance := savedAccount.Balance

	failSync = true
	require.Error(t, SyncUpstreamSite(context.Background(), channel.Id))

	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedAccount).Error)
	assert.Equal(t, model.UpstreamSiteSyncFailed, savedAccount.SyncStatus)
	assert.Equal(t, successfulSyncAt, savedAccount.LastSyncAt)
	assert.Equal(t, successfulBalance, savedAccount.Balance)
	assert.Equal(t, 1, savedAccount.ConsecutiveFailures)
	assert.Zero(t, savedAccount.DisabledAt)

	var failedKey model.UpstreamKey
	require.NoError(t, db.First(&failedKey, savedKey.ID).Error)
	assert.Equal(t, savedKey.SecretCiphertext, failedKey.SecretCiphertext)
	assert.Equal(t, savedKey.Models, failedKey.Models)
	assert.Equal(t, savedKey.ConversionRatio, failedKey.ConversionRatio)
	assert.Equal(t, savedKey.Weight, failedKey.Weight)
	var preservedAbility model.UpstreamKeyAbility
	require.NoError(t, db.Where("upstream_key_id = ?", failedKey.ID).First(&preservedAbility).Error)
	assert.Equal(t, savedAbility.Model, preservedAbility.Model)
}

func TestSyncPlatformSitePersistsRotatedCredentialBeforeSnapshot(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.CryptoSecret = "upstream-site-credential-before-snapshot-test-secret"
	common.MemoryCacheEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.UpstreamKey{},
		&model.UpstreamKeyAbility{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	newUserRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/auth/refresh":
			http.Error(writer, `{"success":false,"message":"expired"}`, http.StatusUnauthorized)
		case "/api/user/login":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}}`))
		case "/api/user/self":
			if request.Header.Get("Authorization") == "Bearer old-access" {
				http.Error(writer, `{"success":false,"message":"expired"}`, http.StatusUnauthorized)
				return
			}
			if request.Header.Get("Authorization") != "Bearer new-access" {
				http.Error(writer, `{"success":false}`, http.StatusUnauthorized)
				return
			}
			newUserRequests++
			if newUserRequests == 1 {
				_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":5,"used_quota":1}}`))
				return
			}
			http.Error(writer, `{"success":false,"message":"snapshot unavailable"}`, http.StatusBadGateway)
		case "/api/user/me", "/api/user/profile", "/api/user/info":
			http.Error(writer, `{"success":false,"message":"snapshot unavailable"}`, http.StatusBadGateway)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	channel := &model.Channel{
		Id:           902,
		Name:         "credential-before-snapshot",
		Type:         constant.ChannelTypeNewAPI,
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
	}
	require.NoError(t, db.Create(channel).Error)
	ciphertext, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{
		AuthType:       model.UpstreamAuthPassword,
		Username:       "operator",
		Password:       "synthetic-password",
		AccessToken:    "old-access",
		RefreshToken:   "old-refresh",
		TokenExpiresAt: common.GetTimestamp() + 3600,
	})
	require.NoError(t, err)
	account := &model.PlatformSiteAccount{
		ChannelID:            channel.Id,
		Platform:             model.PlatformNewAPI,
		BaseURL:              server.URL,
		AuthType:             model.UpstreamAuthPassword,
		CredentialCiphertext: ciphertext,
		CredentialKeyVersion: "v1",
		ConversionRatio:      1,
		SyncStatus:           model.UpstreamSiteSyncIdle,
	}
	require.NoError(t, db.Create(account).Error)

	require.Error(t, SyncUpstreamSite(context.Background(), channel.Id))

	var savedAccount model.PlatformSiteAccount
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedAccount).Error)
	assert.Equal(t, model.UpstreamSiteSyncFailed, savedAccount.SyncStatus)
	credential, err := model.DecryptPlatformSiteCredential(savedAccount.CredentialCiphertext)
	require.NoError(t, err)
	assert.Equal(t, model.UpstreamAuthPassword, credential.AuthType)
	assert.Equal(t, "operator", credential.Username)
	assert.Equal(t, "synthetic-password", credential.Password)
	assert.Equal(t, "new-access", credential.AccessToken)
	assert.Equal(t, "new-refresh", credential.RefreshToken)
	assert.Greater(t, credential.TokenExpiresAt, common.GetTimestamp())
}

func TestPersistPlatformSiteSnapshotRebuildsAbilitiesAndAutoDisablesUnavailableKeys(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-site-test-secret"
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.UpstreamKey{},
		&model.UpstreamKeyAbility{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	priority := int64(4)
	channel := &model.Channel{
		Id:           11,
		Name:         "site",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
		Priority:     &priority,
	}
	require.NoError(t, db.Create(channel).Error)
	account := &model.PlatformSiteAccount{
		ChannelID:       channel.Id,
		Platform:        model.PlatformNewAPI,
		BaseURL:         "https://upstream.example",
		ConversionRatio: 0.1,
	}
	require.NoError(t, db.Create(account).Error)

	remaining := int64(0)
	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance: 5,
		Models:  []string{"gpt-4o"},
		Keys: []UpstreamKeySnapshot{{
			ExternalID:   "key-1",
			Name:         "key",
			Secret:       "sk-test",
			Group:        "default",
			Models:       []string{"gpt-4o"},
			ModelsSynced: true,
			UsedQuota:    4,
			UsedQuotaSet: true,
			RemainQuota:  &remaining,
		}, {
			ExternalID:   "key-2",
			Name:         "key-2",
			Secret:       "sk-test-2",
			Group:        "default",
			Models:       []string{"gpt-4o"},
			ModelsSynced: true,
			UsedQuota:    7,
			UsedQuotaSet: true,
			RemainQuota:  &remaining,
		}},
	}))

	var key model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&key).Error)
	assert.Equal(t, model.UpstreamKeyStatusAutoDisabled, key.Status)
	assert.Equal(t, "密钥剩余额度不足", key.DisabledReason)
	var savedAccount model.PlatformSiteAccount
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedAccount).Error)
	assert.Equal(t, 5.0, savedAccount.Balance)
	assert.Equal(t, int64(11), savedAccount.UsedQuota)
	var savedChannel model.Channel
	require.NoError(t, db.First(&savedChannel, channel.Id).Error)
	assert.Equal(t, int64(11), savedChannel.UsedQuota)
	assert.Equal(t, 1900, key.Weight)

	var ability model.Ability
	assert.ErrorIs(t, db.Where(&model.Ability{
		ChannelId: channel.Id,
		Group:     "default",
		Model:     "gpt-4o",
	}).First(&ability).Error, gorm.ErrRecordNotFound)
	var keyAbility model.UpstreamKeyAbility
	require.NoError(t, db.Where(&model.UpstreamKeyAbility{
		UpstreamKeyID: key.ID,
		Group:         "",
		Model:         "gpt-4o",
	}).First(&keyAbility).Error)
	assert.True(t, keyAbility.Enabled)

	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance: 5,
		Models:  []string{"gpt-4o"},
		Keys:    nil,
	}))

	remaining = 100
	require.NoError(t, db.Model(&model.UpstreamKey{}).
		Where("channel_id = ? AND external_id = ?", channel.Id, "key-1").
		Updates(map[string]any{
			"status":          model.UpstreamKeyStatusEnabled,
			"disabled_reason": "",
			"remain_quota":    remaining,
		}).Error)

	var missingKey model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "key-1").First(&missingKey).Error)
	require.NotZero(t, missingKey.MissingSince)
	assert.False(t, missingKey.IsRoutable(time.Now()))

	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance: 5,
		Models:  []string{"gpt-4o"},
		Keys: []UpstreamKeySnapshot{{
			ExternalID:   "key-1",
			Name:         "key",
			Secret:       "sk-test",
			Group:        "default",
			Models:       []string{"gpt-4o"},
			ModelsSynced: true,
			RemainQuota:  &remaining,
		}},
	}))

	var restoredKey model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "key-1").First(&restoredKey).Error)
	assert.Equal(t, model.UpstreamKeyStatusEnabled, restoredKey.Status)
	assert.Zero(t, restoredKey.MissingSince)
	assert.True(t, restoredKey.IsRoutable(time.Now()))
}

func TestPersistPlatformSiteSnapshotUsedQuotaDatabaseMatrix(t *testing.T) {
	// MySQL/PostgreSQL DSN must identify an isolated scratch database. The
	// matrix uses a unique table prefix and must never touch the hot database.
	for _, dialect := range []struct {
		name string
		dsn  string
		open func(string) gorm.Dialector
	}{
		{
			name: "sqlite",
			dsn:  fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_")),
			open: func(dsn string) gorm.Dialector { return sqlite.Open(dsn) },
		},
		{
			name: "mysql",
			dsn:  strings.TrimSpace(os.Getenv("TEST_UPSTREAM_SITE_MYSQL_DSN")),
			open: func(dsn string) gorm.Dialector { return mysql.Open(dsn) },
		},
		{
			name: "postgres",
			dsn:  strings.TrimSpace(os.Getenv("TEST_UPSTREAM_SITE_POSTGRES_DSN")),
			open: func(dsn string) gorm.Dialector {
				return postgres.New(postgres.Config{
					DSN:                  dsn,
					PreferSimpleProtocol: true,
				})
			},
		},
	} {
		t.Run(dialect.name, func(t *testing.T) {
			if dialect.dsn == "" {
				t.Skip("测试数据库 DSN 未配置")
			}
			if dialect.name != "sqlite" {
				assertScratchDatabaseDSN(t, dialect.dsn)
			}
			prefix := fmt.Sprintf("upstream_site_matrix_%d_", time.Now().UnixNano())
			db, err := gorm.Open(dialect.open(dialect.dsn), &gorm.Config{
				NamingStrategy: schema.NamingStrategy{TablePrefix: prefix},
			})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			markerTable := prefix + "test_guard"
			require.NoError(t, db.Table(markerTable).AutoMigrate(&upstreamSiteScratchMarker{}))
			require.NoError(t, db.Table(markerTable).Create(&upstreamSiteScratchMarker{
				Marker: "upstream-site-matrix",
			}).Error)
			var markerCount int64
			require.NoError(t, db.Table(markerTable).
				Where("marker = ?", "upstream-site-matrix").
				Count(&markerCount).Error)
			require.Equal(t, int64(1), markerCount)
			t.Cleanup(func() {
				_ = db.Migrator().DropTable(
					&model.UpstreamKeyAbility{},
					&model.UpstreamKey{},
					&model.PlatformSiteAccount{},
					&model.Ability{},
					&model.Channel{},
				)
				_ = db.Migrator().DropTable(markerTable)
			})
			require.NoError(t, db.AutoMigrate(
				&model.Channel{},
				&model.Ability{},
				&model.PlatformSiteAccount{},
				&model.UpstreamKey{},
				&model.UpstreamKeyAbility{},
			))
			versionQuery := "select version()"
			if dialect.name == "sqlite" {
				versionQuery = "select sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database: %s", version)

			previousDB := model.DB
			previousSecret := common.CryptoSecret
			model.DB = db
			common.CryptoSecret = "upstream-site-used-quota-matrix-secret"
			t.Cleanup(func() {
				model.DB = previousDB
				common.CryptoSecret = previousSecret
			})

			priority := int64(1)
			channel := &model.Channel{
				Name:         "matrix-site",
				Status:       common.ChannelStatusEnabled,
				UpstreamKind: model.UpstreamKindPlatformSite,
				Group:        "default",
				Priority:     &priority,
			}
			require.NoError(t, db.Create(channel).Error)
			account := &model.PlatformSiteAccount{
				ChannelID:       channel.Id,
				Platform:        model.PlatformNewAPI,
				BaseURL:         "https://upstream.example",
				ConversionRatio: 0.1,
			}
			require.NoError(t, db.Create(account).Error)

			require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
				Balance: 12,
				Keys: []UpstreamKeySnapshot{
					{
						ExternalID:   "key-a",
						Name:         "key-a",
						Secret:       "sk-a",
						Models:       []string{"gpt-4o"},
						ModelsSynced: true,
						UsedQuota:    6,
						UsedQuotaSet: true,
					},
					{
						ExternalID:   "key-b",
						Name:         "key-b",
						Secret:       "sk-b",
						Models:       []string{"gpt-4o-mini"},
						ModelsSynced: true,
						UsedQuota:    9,
						UsedQuotaSet: true,
					},
				},
			}))

			var savedAccount model.PlatformSiteAccount
			require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedAccount).Error)
			assert.Equal(t, int64(15), savedAccount.UsedQuota)
			var savedChannel model.Channel
			require.NoError(t, db.First(&savedChannel, channel.Id).Error)
			assert.Equal(t, int64(15), savedChannel.UsedQuota)

			require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
				Balance:      12,
				UsedQuota:    0,
				UsedQuotaSet: true,
				Keys: []UpstreamKeySnapshot{{
					ExternalID:   "key-a",
					Name:         "key-a",
					Secret:       "sk-a",
					Models:       []string{"gpt-4o"},
					ModelsSynced: true,
					UsedQuota:    6,
					UsedQuotaSet: true,
				}},
			}))

			require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedAccount).Error)
			assert.Zero(t, savedAccount.UsedQuota)
			require.NoError(t, db.First(&savedChannel, channel.Id).Error)
			assert.Zero(t, savedChannel.UsedQuota)
		})
	}
}

type upstreamSiteScratchMarker struct {
	ID     uint   `gorm:"primaryKey"`
	Marker string `gorm:"type:varchar(64);not null;uniqueIndex"`
}

func assertScratchDatabaseDSN(t *testing.T, dsn string) {
	t.Helper()
	databaseName := strings.ToLower(upstreamSiteDSNDatabaseName(dsn))
	if databaseName == "nexustok" {
		t.Fatalf("拒绝使用运行库 DSN：数据库名称为 nexustok")
	}
	if !strings.Contains(databaseName, "test") && !strings.Contains(databaseName, "scratch") {
		t.Fatalf("拒绝使用未标记的测试 DSN：数据库标识必须包含 test 或 scratch")
	}
}

func upstreamSiteDSNDatabaseName(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if strings.Contains(dsn, "://") {
		if parsed, err := url.Parse(dsn); err == nil {
			if database := parsed.Query().Get("dbname"); database != "" {
				return database
			}
			if database := parsed.Query().Get("database"); database != "" {
				return database
			}
			return strings.Trim(parsed.Path, "/")
		}
	}
	if separator := strings.LastIndex(dsn, ")/"); separator >= 0 {
		database := dsn[separator+2:]
		if query := strings.IndexByte(database, '?'); query >= 0 {
			database = database[:query]
		}
		return database
	}
	for _, field := range strings.FieldsFunc(dsn, func(r rune) bool {
		return r == ' ' || r == ','
	}) {
		for _, key := range []string{"dbname=", "database="} {
			if database, ok := strings.CutPrefix(field, key); ok {
				return database
			}
		}
	}
	return dsn
}

func TestPersistPlatformSiteSnapshotPreservesManualRatioAndWeightOverrides(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-site-override-test-secret"
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.UpstreamKey{},
		&model.UpstreamKeyAbility{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	priority := int64(3)
	channel := &model.Channel{
		Id:           14,
		Name:         "override-site",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
		Priority:     &priority,
	}
	require.NoError(t, db.Create(channel).Error)
	account := &model.PlatformSiteAccount{
		ChannelID:       channel.Id,
		Platform:        model.PlatformNewAPI,
		BaseURL:         "https://upstream.example",
		ConversionRatio: 0.1,
	}
	require.NoError(t, db.Create(account).Error)

	ratioOverride := 0.42
	weightOverride := 1777
	secret, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{AccessToken: "sk-override"})
	require.NoError(t, err)
	key := &model.UpstreamKey{
		ChannelID:               channel.Id,
		ExternalID:              "override-key",
		SecretCiphertext:        secret,
		ConversionRatio:         ratioOverride,
		ConversionRatioOverride: &ratioOverride,
		Weight:                  1580,
		WeightOverride:          &weightOverride,
		Status:                  model.UpstreamKeyStatusEnabled,
	}
	require.NoError(t, db.Create(key).Error)

	sourceRatio := 0.7
	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance: 9,
		Keys: []UpstreamKeySnapshot{{
			ExternalID:               "override-key",
			Name:                     "override",
			Secret:                   "sk-override-next",
			SourceConversionRatio:    sourceRatio,
			SourceConversionRatioSet: true,
			Models:                   []string{"gpt-4o"},
			ModelsSynced:             true,
		}},
	}))

	var saved model.UpstreamKey
	require.NoError(t, db.First(&saved, key.ID).Error)
	require.NotNil(t, saved.SourceConversionRatio)
	assert.Equal(t, sourceRatio, *saved.SourceConversionRatio)
	require.NotNil(t, saved.ConversionRatioOverride)
	assert.Equal(t, ratioOverride, *saved.ConversionRatioOverride)
	assert.Equal(t, ratioOverride, saved.ConversionRatio)
	require.NotNil(t, saved.WeightOverride)
	assert.Equal(t, weightOverride, *saved.WeightOverride)
	assert.Equal(t, weightOverride, saved.EffectiveWeight())
}

func TestPlatformSiteRequestRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat("x", upstreamSiteResponseLimit+1)))
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	_, err = platformSiteRequest(context.Background(), session, http.MethodGet, "/", url.Values{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "响应体超过限制")
}
