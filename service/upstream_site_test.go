package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
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
			baseURL:    strings.TrimSpace(os.Getenv("NEXUSTOK_REAL_NEWAPI_BASE_URL")),
			username:   strings.TrimSpace(os.Getenv("NEXUSTOK_REAL_NEWAPI_USERNAME")),
			password:   os.Getenv("NEXUSTOK_REAL_NEWAPI_PASSWORD"),
			wantModels: true,
		},
		{
			name:       "Sub2API",
			adapter:    NewSub2APIAdapter(nil),
			baseURL:    strings.TrimSpace(os.Getenv("NEXUSTOK_REAL_SUB2API_BASE_URL")),
			username:   strings.TrimSpace(os.Getenv("NEXUSTOK_REAL_SUB2API_USERNAME")),
			password:   os.Getenv("NEXUSTOK_REAL_SUB2API_PASSWORD"),
			wantModels: true,
		},
	}

	enabled := false
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.baseURL == "" || testCase.username == "" || testCase.password == "" {
				t.Skip("未配置该平台真实只读验证环境变量")
			}
			enabled = true
			session, err := testCase.adapter.Authenticate(
				context.Background(),
				testCase.baseURL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: testCase.username,
					Password: testCase.password,
				},
			)
			if session != nil {
				t.Cleanup(func() {
					if cleanupErr := testCase.adapter.Cleanup(context.Background(), session); cleanupErr != nil {
						t.Logf(
							"真实平台临时会话清理失败: %s",
							SafePlatformSiteSessionCleanupError(cleanupErr),
						)
					}
				})
			}
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
	if !enabled {
		t.Skip("未配置真实平台只读验证环境变量")
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

func TestPlatformSiteChannelProxyPreservesCookieJarAndSessionPolicy(t *testing.T) {
	proxyRequests := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		proxyRequests++
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/first" {
			http.SetCookie(writer, &http.Cookie{
				Name:  "new_api_refresh",
				Value: "proxy-refresh",
				Path:  "/",
			})
		}
		if request.URL.Path == "/second" {
			assert.Equal(t, "new_api_refresh=proxy-refresh", request.Header.Get("Cookie"))
		}
		_, _ = writer.Write([]byte(`{"success":true}`))
	}))
	defer proxy.Close()

	channelID := 9801
	setting := fmt.Sprintf(`{"proxy":%q}`, proxy.URL)
	channel := &model.Channel{
		Id:      channelID,
		Name:    "platform-site-proxy",
		Setting: &setting,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	t.Cleanup(func() {
		model.DB.Where("id = ?", channelID).Delete(&model.Channel{})
	})

	client, err := platformSiteHTTPClientForChannel(channelID)
	require.NoError(t, err)
	require.NotNil(t, client)

	session, err := newPlatformSiteSession("http://127.0.0.1:1", nil)
	require.NoError(t, err)
	attachPlatformSiteHTTPClient(session, client)
	require.NotNil(t, session.Client.Jar)
	assert.Equal(t, upstreamSiteRequestTimeout, session.Client.Timeout)
	assert.NotNil(t, session.Client.CheckRedirect)

	_, err = platformSiteRequest(context.Background(), session, http.MethodGet, "/first", nil, nil)
	require.NoError(t, err)
	_, err = platformSiteRequest(context.Background(), session, http.MethodGet, "/second", nil, nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, proxyRequests, 2)
}

func TestSyncPlatformSiteUsesChannelProxyAndClassifiesNewAPICredentials(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.PlatformSiteAccount{}, &model.Ability{}))

	proxyRequests := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		proxyRequests++
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/user/login" {
			_, _ = writer.Write([]byte(
				`{"success":false,"message":"Username or password error, or user has been banned"}`,
			))
			return
		}
		http.NotFound(writer, request)
	}))
	defer proxy.Close()

	channelID := 9802
	setting := fmt.Sprintf(`{"proxy":%q}`, proxy.URL)
	channel := &model.Channel{
		Id:           channelID,
		Name:         "platform-site-sync-proxy",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
		Setting:      &setting,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	t.Cleanup(func() {
		model.DB.Where("channel_id = ?", channelID).Delete(&model.PlatformSiteAccount{})
		model.DB.Where("id = ?", channelID).Delete(&model.Channel{})
	})

	credentialCiphertext, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
		Password: "proxy-password",
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.PlatformSiteAccount{
		ChannelID:            channelID,
		Platform:             model.PlatformNewAPI,
		BaseURL:              "http://127.0.0.1:1",
		AuthType:             model.UpstreamAuthPassword,
		CredentialCiphertext: credentialCiphertext,
		CredentialKeyVersion: "v1",
		SyncStatus:           model.UpstreamSiteSyncIdle,
	}).Error)

	err = SyncUpstreamSite(context.Background(), channelID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteCredentials)
	assert.NotErrorIs(t, err, ErrPlatformSiteTransport)
	assert.GreaterOrEqual(t, proxyRequests, 1)
	assert.Contains(t, SafePlatformSiteError(err), "账号或密码错误")
	assert.NotContains(t, SafePlatformSiteError(err), "proxy-password")
}

func TestPlatformSiteAuthFlowUsesChannelProxyForLoginAndTwoFA(t *testing.T) {
	proxyRequests := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		proxyRequests++
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{
				Name:  "new_api_refresh",
				Value: "flow-refresh",
				Path:  "/api/user/auth",
			})
			_, _ = writer.Write([]byte(
				`{"success":true,"data":{"require_2fa":true,"flow_token":"flow-token"}}`,
			))
		case "/api/user/login/2fa":
			assert.Equal(t, "new_api_refresh=flow-refresh", request.Header.Get("Cookie"))
			_, _ = writer.Write([]byte(fmt.Sprintf(
				`{"success":true,"data":{"access_token":"flow-access","token_type":"Bearer","access_expires_at":%d,"session":{"sid":"flow-session","current":true},"user":{"id":17}}}`,
				common.GetTimestamp()+3600,
			)))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer proxy.Close()

	channelID := 9803
	setting := fmt.Sprintf(`{"proxy":%q}`, proxy.URL)
	channel := &model.Channel{Id: channelID, Name: "platform-site-auth-proxy", Setting: &setting}
	require.NoError(t, model.DB.Create(channel).Error)
	t.Cleanup(func() {
		model.DB.Where("id = ?", channelID).Delete(&model.Channel{})
	})

	started, err := StartPlatformSiteAuthFlow(context.Background(), 9803, PlatformSiteAuthFlowStartRequest{
		Platform:  model.PlatformNewAPI,
		BaseURL:   "http://127.0.0.1:1",
		AuthType:  model.UpstreamAuthPassword,
		Username:  "operator",
		Password:  "flow-password",
		ChannelID: channelID,
	})
	require.NoError(t, err)
	assert.Equal(t, PlatformSiteAuthFlowStatusTwoFactorRequired, started.Status)
	t.Cleanup(func() {
		_ = DeletePlatformSiteAuthFlow(9803, started.FlowID)
	})

	verified, err := VerifyPlatformSiteAuthFlow(
		context.Background(),
		9803,
		started.FlowID,
		PlatformSiteAuthFlowVerifyRequest{Code: "123456"},
	)
	require.NoError(t, err)
	assert.Equal(t, PlatformSiteAuthFlowStatusAuthenticated, verified.Status)
	assert.Equal(t, "17", verified.UserID)
	assert.GreaterOrEqual(t, proxyRequests, 2)
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

	require.NoError(t, ConsumePlatformSiteCapture(7, start.CaptureID, 0, resolved.ClaimToken))
	_, err = ResolvePlatformSiteCapture(
		7,
		start.CaptureID,
		0,
		model.PlatformSub2API,
		model.UpstreamAuthAccessToken,
	)
	require.Error(t, err)
}

func TestPlatformSiteCaptureScriptUsesTargetScopedConnectAndCookieVerification(t *testing.T) {
	start, err := StartPlatformSiteCaptureSession(8, PlatformSiteCaptureStartRequest{
		Platform: model.PlatformNewAPI,
		BaseURL:  "http://127.0.0.1:8080",
		AuthType: PlatformSiteCaptureAuthAuto,
	}, "https://nexus.example.com")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = platformSiteCaptureCache.DeleteMany([]string{start.CaptureID})
	})

	record, found, err := platformSiteCaptureCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)

	script, err := RenderPlatformSiteCaptureUserscript(
		start.CaptureID,
		record.InstallToken,
		"https://nexus.example.com",
	)
	require.NoError(t, err)
	assert.Contains(t, script, "// @connect      nexus.example.com")
	assert.Contains(t, script, "// @connect      127.0.0.1")
	assert.Contains(t, script, "// @connect      api.127.0.0.1")
	assert.Contains(t, script, "headers.Cookie = cookie")
	assert.Contains(t, script, "{ headers, cookie }")
	assert.NotContains(t, script, "// @connect      *")
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
			request.CaptureSource = "capture_helper"
			request.HelperVersion = platformSiteCaptureHelperVersion
			request.Diagnostics = &PlatformSiteCaptureDiagnostics{}
			switch testCase.wantAuth {
			case model.UpstreamAuthAdminKey:
				request.Diagnostics.AdminKeyVerified = true
			default:
				request.Diagnostics.AuthUserVerified = true
			}
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
			require.NoError(t, ReleasePlatformSiteCaptureClaim(start.CaptureID, resolved.ClaimToken))
			resolved, err = ResolvePlatformSiteCapture(
				17,
				start.CaptureID,
				0,
				model.PlatformNewAPI,
				testCase.wantAuth,
			)
			require.NoError(t, err)
			assert.Equal(t, testCase.wantAuth, resolved.Credential.AuthType)
			require.NoError(t, ReleasePlatformSiteCaptureClaim(start.CaptureID, resolved.ClaimToken))
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
		CaptureSource: "capture_helper",
		HelperVersion: platformSiteCaptureHelperVersion,
		Diagnostics:   &PlatformSiteCaptureDiagnostics{},
	})
	require.Error(t, err)

	status, err := GetPlatformSiteCaptureStatus(18, start.CaptureID, "http://127.0.0.1:3003")
	require.NoError(t, err)
	assert.Equal(t, platformSiteCaptureStatusFailed, status.Status)
	assert.Equal(t, "自动配置未采集到可用登录态", status.Message)
}

func TestPlatformSiteCaptureAutoRequiresVerifiedHelperSubmission(t *testing.T) {
	start, err := StartPlatformSiteCaptureSession(19, PlatformSiteCaptureStartRequest{
		Platform: model.PlatformNewAPI,
		BaseURL:  "http://127.0.0.1:8183",
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
		AccessToken:   "access-token",
	})
	require.ErrorContains(t, err, "Capture Helper")

	_, err = CompletePlatformSiteCaptureSession(start.CaptureID, PlatformSiteCaptureCompleteRequest{
		CaptureSecret: record.Secret,
		Platform:      model.PlatformNewAPI,
		AuthType:      PlatformSiteCaptureAuthAuto,
		Origin:        record.Origin,
		CaptureSource: "capture_helper",
		HelperVersion: platformSiteCaptureHelperVersion,
		AccessToken:   "access-token",
		Diagnostics: &PlatformSiteCaptureDiagnostics{
			AuthUserVerified: true,
		},
	})
	require.NoError(t, err)
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
	require.NoError(t, ReleasePlatformSiteCaptureClaim(start.CaptureID, resolved.ClaimToken))

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
	require.Contains(t, script, "guessNewAPIUserID")
	require.Contains(t, script, "expandedAPIPaths")
	require.Contains(t, script, "discoverAPIPaths")
	require.Contains(t, script, "__AUTH_USER__")
	require.Contains(t, script, "New-Api-User")
	require.Contains(t, script, "@grant        GM_cookie")
	require.Contains(t, script, "headers.Authorization = 'Bearer '")
	require.Contains(t, script, "mergeCookieHeaders")
	require.Contains(t, script, "management_base_url")
	require.Contains(t, script, "delete safePayload.auth_user")
	require.Contains(t, script, "source !== 'refresh_token'")
	require.Contains(t, script, "valueFromRecord")
	require.Contains(t, script, "runCapture")
	require.Contains(t, script, "scheduleRetry")
	require.Contains(t, script, "DOMContentLoaded")
	require.Contains(t, script, "typeof payload === 'string'")
	require.Contains(t, script, "data && typeof data === 'object'")
	require.Contains(t, script, "responseSucceeded(result)")
	require.Contains(t, script, "sub2api-auth-coordination")
	require.Contains(t, script, "sub2api_auth_client_id")
	require.Contains(t, script, "captureStopped")
	require.Contains(t, script, "isPermanentCaptureError")
	require.Contains(t, script, "clearStoredHandoff")
	require.Contains(t, script, "正在等待上游登录")
	require.NotContains(t, script, "removeStoredHandoff")
	require.NotContains(t, script, "error: safeMessage")
	require.Contains(t, script, "GM_cookie.list({ url: cookieURL")
	require.Contains(t, script, "credentials: 'include'")
	require.NotContains(t, script, record.Secret)
	require.NotContains(t, script, "result.auth_user = validatedUser")

	helper, err := RenderPlatformSiteCaptureHelper("https://nexus.example.com")
	require.NoError(t, err)
	require.Contains(t, helper, "@match        http://*/*")
	require.Contains(t, helper, "@match        https://*/*")
	require.NotContains(t, helper, record.Secret)
}

func TestPlatformSiteCaptureHelperGeneratedScriptIsValidJavaScript(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("未安装 Node.js，跳过 Capture Helper JavaScript 语法检查")
	}

	helper, err := RenderPlatformSiteCaptureHelper("https://nexus.example.com")
	require.NoError(t, err)
	scriptPath := t.TempDir() + "/capture-helper.user.js"
	require.NoError(t, os.WriteFile(scriptPath, []byte(helper), 0o600))

	output, err := exec.Command(nodePath, "--check", scriptPath).CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestPlatformSiteCaptureBridgeUsesSignedInstallAndPostMessage(t *testing.T) {
	start, err := StartPlatformSiteCaptureSession(20, PlatformSiteCaptureStartRequest{
		Platform: model.PlatformNewAPI,
		BaseURL:  "http://127.0.0.1:8184",
		AuthType: PlatformSiteCaptureAuthAuto,
	}, "https://nexus.example.com")
	require.NoError(t, err)
	require.Contains(t, start.BridgeURL, "/bridge.js?install_token=")

	record, found, err := platformSiteCaptureCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, record.InstallToken)
	require.NotContains(t, start.BridgeURL, record.Secret)

	script, err := RenderPlatformSiteCaptureBridge(
		start.CaptureID,
		record.InstallToken,
		"https://nexus.example.com",
	)
	require.NoError(t, err)
	require.Contains(t, script, "nexustok-upstream-capture-bridge-result")
	require.Contains(t, script, "nexustok-upstream-capture-bridge-ack")
	require.Contains(t, script, "nexustok-upstream-capture-bridge-request")
	require.Contains(t, script, "nexustok-upstream-capture-bridge-handoff")
	require.Contains(t, script, "requestBridgeHandoff")
	require.Contains(t, script, "handoffFromURL")
	require.Contains(t, script, "window.opener.postMessage")
	require.Contains(t, script, "capture_source: 'capture_bridge'")
	require.Contains(t, script, `"transport":"bridge"`)
	require.NotContains(t, script, record.Secret)

	nodePath, err := exec.LookPath("node")
	if err == nil {
		scriptPath := t.TempDir() + "/capture-bridge.js"
		require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o600))
		output, checkErr := exec.Command(nodePath, "--check", scriptPath).CombinedOutput()
		require.NoError(t, checkErr, string(output))
	}

	status, err := CompletePlatformSiteCaptureSession(start.CaptureID, PlatformSiteCaptureCompleteRequest{
		CaptureSecret: record.Secret,
		CaptureSource: "capture_bridge",
		HelperVersion: platformSiteCaptureHelperVersion,
		Platform:      model.PlatformNewAPI,
		AuthType:      PlatformSiteCaptureAuthAuto,
		Origin:        record.Origin,
		AccessToken:   "bridge-access-token",
		Diagnostics: &PlatformSiteCaptureDiagnostics{
			AuthUserVerified: true,
			Source:           "capture_bridge",
		},
	})
	require.NoError(t, err)
	require.Equal(t, platformSiteCaptureStatusComplete, status.Status)
	require.NotNil(t, status.Summary)
	assert.Equal(t, "capture_bridge", status.Summary.CaptureSource)
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
	groupFallbackRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/user/login":
			_, hasTurnstile := request.URL.Query()["turnstile"]
			assert.True(t, hasTurnstile)
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			var loginPayload map[string]any
			require.NoError(t, common.Unmarshal(body, &loginPayload))
			assert.Equal(t, map[string]any{
				"username": "operator",
				"password": "synthetic-password",
			}, loginPayload)
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"success":true,"data":{"token":"newapi-session"}}`))
		case request.URL.Path == "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":10}}`))
		case request.URL.Path == "/api/user/self":
			assert.Equal(t, "Bearer newapi-session", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":12.5,"used_quota":3}}`))
		case request.URL.Path == "/api/user/self/groups":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"default":{"ratio":0.7,"desc":"默认组"}}}`))
		case request.URL.Path == "/api/ratio_config":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"group_ratio":{"default":0.7}}}`))
		case request.URL.Path == "/api/user/groups":
			groupFallbackRequests++
			t.Fatalf("第一个分组路由成功后不应继续请求兼容路由")
		case request.URL.Path == "/api/user/models":
			_, _ = writer.Write([]byte(`{"success":true,"data":["gpt-4o","claude-3-7-sonnet"]}`))
		case request.URL.Path == "/api/token/":
			assert.Equal(t, "1", request.URL.Query().Get("p"))
			assert.Equal(t, "100", request.URL.Query().Get("page_size"))
			assert.Empty(t, request.URL.Query().Get("page"))
			assert.Empty(t, request.URL.Query().Get("size"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"page":1,"page_size":100,"total":1,"items":[{"id":7,"name":"primary","key":"sk-newapi","group":"default","quota":8,"expired_time":"4102444800","model_limits":"gpt-4o,claude-3-7-sonnet"}]}}`))
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
	assert.Zero(t, groupFallbackRequests)
}

func decryptNewAPIPasswordFixture(
	t *testing.T,
	ciphertext string,
	privateKey *rsa.PrivateKey,
	keyID string,
) string {
	t.Helper()
	if !strings.HasPrefix(ciphertext, "v2.") {
		decoded, err := base64.StdEncoding.DecodeString(ciphertext)
		require.NoError(t, err)
		plaintext, err := rsa.DecryptOAEP(
			sha256.New(),
			rand.Reader,
			privateKey,
			decoded,
			nil,
		)
		require.NoError(t, err)
		return string(plaintext)
	}

	parts := strings.Split(ciphertext, ".")
	require.Len(t, parts, 4)
	wrappedKey, err := base64.StdEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	nonce, err := base64.StdEncoding.DecodeString(parts[2])
	require.NoError(t, err)
	encrypted, err := base64.StdEncoding.DecodeString(parts[3])
	require.NoError(t, err)
	secret, err := rsa.DecryptOAEP(
		sha256.New(),
		rand.Reader,
		privateKey,
		wrappedKey,
		[]byte(newAPIPasswordV2Label),
	)
	require.NoError(t, err)
	block, err := aes.NewCipher(secret)
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	plaintext, err := gcm.Open(
		nil,
		nonce,
		encrypted,
		[]byte(newAPIPasswordV2Label+":"+keyID),
	)
	require.NoError(t, err)
	return string(plaintext)
}

func TestNewAPIAdapterPasswordUsesEncryptedLoginWhenEnabled(t *testing.T) {
	passwords := []struct {
		name     string
		password string
	}{
		{name: "short RSA password", password: "synthetic-password"},
		{name: "long AES envelope password", password: strings.Repeat("long-password-", 32)},
	}

	for _, testCase := range passwords {
		t.Run(testCase.name, func(t *testing.T) {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			require.NoError(t, err)
			publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
			require.NoError(t, err)
			publicKeyPEM := pem.EncodeToMemory(&pem.Block{
				Type:  "PUBLIC KEY",
				Bytes: publicKeyDER,
			})
			const keyID = "synthetic-kid"
			loginRequests := 0

			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				request *http.Request,
			) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case newAPIPasswordEncryptionPath:
					_, _ = writer.Write([]byte(fmt.Sprintf(
						`{"success":true,"data":{"enabled":true,"kid":%q,"public_key":%q}}`,
						keyID,
						string(publicKeyPEM),
					)))
				case "/api/user/login":
					loginRequests++
					body, readErr := io.ReadAll(request.Body)
					require.NoError(t, readErr)
					var loginPayload map[string]string
					require.NoError(t, common.Unmarshal(body, &loginPayload))
					assert.Equal(t, "operator", loginPayload["username"])
					assert.Empty(t, loginPayload["password"])
					assert.Equal(t, keyID, loginPayload["encryption_key_id"])
					assert.NotContains(t, string(body), testCase.password)
					assert.Equal(
						t,
						testCase.password,
						decryptNewAPIPasswordFixture(
							t,
							loginPayload["password_encrypted"],
							privateKey,
							keyID,
						),
					)
					_, _ = writer.Write([]byte(
						`{"success":true,"data":{"token":"encrypted-login-access","user":{"id":17}}}`,
					))
				case "/api/user/self":
					assert.Equal(t, "Bearer encrypted-login-access", request.Header.Get("Authorization"))
					_, _ = writer.Write([]byte(
						`{"success":true,"data":{"id":17,"username":"operator"}}`,
					))
				default:
					http.NotFound(writer, request)
				}
			}))
			t.Cleanup(server.Close)

			_, err = NewNewAPIAdapter(server.Client()).Authenticate(
				context.Background(),
				server.URL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: "operator",
					Password: testCase.password,
				},
			)
			require.NoError(t, err)
			assert.Equal(t, 1, loginRequests)
		})
	}
}

func TestNewAPIAdapterPasswordFallsBackToPlaintextOnlyWhenEncryptionRouteMissing(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(fmt.Sprintf("status-%d", status), func(t *testing.T) {
			loginRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				request *http.Request,
			) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case newAPIPasswordEncryptionPath:
					writer.WriteHeader(status)
					_, _ = writer.Write([]byte(`{"message":"route missing"}`))
				case "/api/user/login":
					loginRequests++
					body, readErr := io.ReadAll(request.Body)
					require.NoError(t, readErr)
					var loginPayload map[string]string
					require.NoError(t, common.Unmarshal(body, &loginPayload))
					assert.Equal(t, "synthetic-password", loginPayload["password"])
					assert.Empty(t, loginPayload["password_encrypted"])
					assert.Empty(t, loginPayload["encryption_key_id"])
					_, _ = writer.Write([]byte(
						`{"success":true,"data":{"token":"plaintext-login-access","user":{"id":17}}}`,
					))
				case "/api/user/self":
					assert.Equal(t, "Bearer plaintext-login-access", request.Header.Get("Authorization"))
					_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17}}`))
				default:
					http.NotFound(writer, request)
				}
			}))
			t.Cleanup(server.Close)

			_, err := NewNewAPIAdapter(server.Client()).Authenticate(
				context.Background(),
				server.URL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: "operator",
					Password: "synthetic-password",
				},
			)
			require.NoError(t, err)
			assert.Equal(t, 1, loginRequests)
		})
	}
}

func TestNewAPIAdapterPasswordDoesNotFallbackWhenEncryptionConfigFails(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"message":"permission denied"}`,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			body:       `{"message":"temporary failure"}`,
		},
		{
			name:       "invalid enabled configuration",
			statusCode: http.StatusOK,
			body:       `{"success":true,"data":{"enabled":true}}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			loginRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				request *http.Request,
			) {
				writer.Header().Set("Content-Type", "application/json")
				if request.URL.Path == newAPIPasswordEncryptionPath {
					writer.WriteHeader(testCase.statusCode)
					_, _ = writer.Write([]byte(testCase.body))
					return
				}
				if request.URL.Path == "/api/user/login" {
					loginRequests++
					t.Fatalf("密码加密配置失败时不应回退明文登录")
				}
				http.NotFound(writer, request)
			}))
			t.Cleanup(server.Close)

			_, err := NewNewAPIAdapter(server.Client()).Authenticate(
				context.Background(),
				server.URL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: "operator",
					Password: "synthetic-password",
				},
			)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPlatformSiteAuth)
			assert.Zero(t, loginRequests)
		})
	}
}

func TestNewAPITokenPaginationUsesLegacyOneBasedParameters(t *testing.T) {
	requestedPages := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "/api/token/", request.URL.Path)
		requestedPages = append(requestedPages, request.URL.Query().Get("p"))
		assert.Equal(t, "100", request.URL.Query().Get("page_size"))
		assert.Empty(t, request.URL.Query().Get("size"))
		switch request.URL.Query().Get("p") {
		case "1":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"page":1,"page_size":1,"total":2,"items":[{"id":7,"key":"sk-first"}]}}`))
		case "2":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"page":2,"page_size":1,"total":2,"items":[{"id":8,"key":"sk-second"}]}}`))
		default:
			t.Fatalf("旧版 NewAPI 分页不应请求 p=%s", request.URL.Query().Get("p"))
		}
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	tokens, err := fetchNewAPITokens(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, []string{"1", "2"}, requestedPages)
	require.Len(t, tokens, 2)
	assert.Equal(t, "7", firstString(tokens[0], "id"))
	assert.Equal(t, "8", firstString(tokens[1], "id"))
}

func TestNewAPITokenPaginationDoesNotUseZeroBasedPrimaryRequest(t *testing.T) {
	requestedPages := make([]string, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "/api/token/", request.URL.Path)
		requestedPages = append(requestedPages, request.URL.Query().Get("p"))
		assert.Equal(t, "1", request.URL.Query().Get("p"))
		assert.Equal(t, "100", request.URL.Query().Get("page_size"))
		_, _ = writer.Write([]byte(`{"success":true,"data":{"page":1,"page_size":100,"total":1,"items":[{"id":17,"key":"sk-one"}]}}`))
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	tokens, err := fetchNewAPITokens(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, []string{"1"}, requestedPages)
	require.Len(t, tokens, 1)
	assert.Equal(t, "17", firstString(tokens[0], "id"))
}

func TestNewAPITokensUseIndependentThousandPageLimit(t *testing.T) {
	requestedPages := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "/api/token/", request.URL.Path)
		page := request.URL.Query().Get("p")
		if page == "1001" {
			t.Fatalf("NewAPI 密钥分页不应请求第 1001 页")
		}
		requestedPages++
		_, _ = fmt.Fprintf(
			writer,
			`{"success":true,"data":{"page":%s,"page_size":1,"total":1000,"items":[{"id":"key-%s","key":"sk-%s","models":["gpt-4o"]}]}}`,
			page,
			page,
			page,
		)
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	tokens, err := fetchNewAPITokens(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, 1000, requestedPages)
	require.Len(t, tokens, 1000)
	assert.Equal(t, "key-1000", firstString(tokens[len(tokens)-1], "id"))
}

func TestNewAPILoginTriesCompatibleIdentityBodiesOnlyForCredentialErrors(t *testing.T) {
	loginBodies := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			loginBodies = append(loginBodies, string(body))
			if len(loginBodies) < 3 {
				_, _ = writer.Write([]byte(`{"success":false,"message":"invalid username or password"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":{"token":"newapi-session"}}`))
		case "/api/user/self":
			assert.Equal(t, "Bearer newapi-session", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	require.Len(t, loginBodies, 3)
	assert.Contains(t, loginBodies[0], `"username":"operator@example.com"`)
	assert.NotContains(t, loginBodies[0], `"email"`)
	assert.Contains(t, loginBodies[1], `"email":"operator@example.com"`)
	assert.NotContains(t, loginBodies[1], `"username"`)
	assert.Contains(t, loginBodies[2], `"username":"operator@example.com"`)
	assert.Contains(t, loginBodies[2], `"email":"operator@example.com"`)
}

func TestNewAPIStatusFailureUsesDefaultQuotaAndStillSyncsTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"token":"newapi-session"}}`))
		case "/api/status":
			writer.WriteHeader(http.StatusBadGateway)
			_, _ = writer.Write([]byte(`{"success":false,"message":"status unavailable"}`))
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":5000000,"used_quota":2000000}}`))
		case "/api/token/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"page":1,"page_size":100,"total":1,"items":[{"id":"key-1","key":"sk-newapi","models":["gpt-4o"],"quota":4}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	snapshot, err := NewNewAPIAdapter(server.Client()).FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, 10.0, snapshot.Balance)
	assert.Equal(t, int64(2000000), snapshot.UsedQuota)
	assert.Equal(t, server.URL, snapshot.ManagementBaseURL)
	assert.Equal(t, server.URL, snapshot.RelayBaseURL)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "sk-newapi", snapshot.Keys[0].Secret)
	assert.Equal(t, []string{"gpt-4o"}, snapshot.Keys[0].Models)
}

func TestNewAPIAdapterModelsFallbackOnlyOnMissingRoute(t *testing.T) {
	t.Run("兼容模型路由", func(t *testing.T) {
		requestedPaths := make([]string, 0, 3)
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requestedPaths = append(requestedPaths, request.URL.Path)
			switch request.URL.Path {
			case "/api/user/models", "/api/user/available_models":
				http.NotFound(writer, request)
			case "/api/user/available_model/":
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(`{"success":true,"data":["gpt-4o-mini"]}`))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		session, err := newPlatformSiteSession(server.URL, nil)
		require.NoError(t, err)
		session.Client = server.Client()
		models, err := fetchNewAPIModels(context.Background(), session)
		require.NoError(t, err)
		assert.Equal(t, []string{"gpt-4o-mini"}, models)
		assert.Equal(t, []string{
			"/api/user/models",
			"/api/user/available_models",
			"/api/user/available_model/",
		}, requestedPaths)
	})

	t.Run("权限错误不继续尝试兼容路由", func(t *testing.T) {
		compatibleRequests := 0
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/api/user/models":
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusForbidden)
				_, _ = writer.Write([]byte(`{"success":false,"code":403,"message":"permission denied"}`))
			case "/api/user/available_models", "/api/user/available_model/":
				compatibleRequests++
				t.Fatalf("权限错误时不应继续尝试模型兼容路由")
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		session, err := newPlatformSiteSession(server.URL, nil)
		require.NoError(t, err)
		session.Client = server.Client()
		_, err = fetchNewAPIModels(context.Background(), session)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPlatformSitePermission)
		assert.Zero(t, compatibleRequests)
	})
}

func TestNewAPIAdapterLoadsAPIyiSelectableGroupsAfterMissingDefaultRoutes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/user/self/groups", "/api/user/groups":
			http.NotFound(writer, request)
		case "/api/groupPro/selectable":
			assert.Equal(t, "0", request.URL.Query().Get("p"))
			assert.Equal(t, "1000", request.URL.Query().Get("pageSize"))
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"success":true,"data":[{"name":"default","display_name":"Default","convert_ratio":0.5}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	rates, groups, loaded, endpoint, err := fetchNewAPIGroupResources(context.Background(), session)
	require.NoError(t, err)
	require.True(t, loaded)
	assert.Equal(t, "/api/groupPro/selectable", endpoint)
	assert.Equal(t, 0.5, rates["default"])
	require.Len(t, groups, 1)
	assert.Equal(t, "default", groups[0].ExternalID)
	assert.Equal(t, "Default", groups[0].Name)
	assert.Equal(t, 0.5, groups[0].Ratio)
}

func TestNewAPIAdapterPasswordDoesNotReusePersistedSession(t *testing.T) {
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			loginRequests++
			assert.Empty(t, request.Header.Get("Authorization"))
			assert.NotContains(t, request.Header.Get("Cookie"), "saved-refresh")
			assert.NotEqual(t, "saved-session", request.Header.Get("X-Auth-Session"))
			_, _ = writer.Write([]byte(
				`{"success":true,"data":{"token":"login-access","user":{"id":17}}}`,
			))
		case "/api/user/self":
			assert.Equal(t, "Bearer login-access", request.Header.Get("Authorization"))
			assert.NotContains(t, request.Header.Get("Cookie"), "saved-refresh")
			assert.NotEqual(t, "saved-session", request.Header.Get("X-Auth-Session"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType:       model.UpstreamAuthPassword,
			Username:       "operator",
			Password:       "synthetic-password",
			UserID:         "17",
			AccessToken:    "saved-access",
			TokenType:      "Bearer",
			TokenExpiresAt: common.GetTimestamp() + 3600,
			SessionID:      "saved-session",
			SessionCurrent: true,
			Cookie:         "new_api_refresh=saved-refresh",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, loginRequests)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, "17", session.CredentialUpdate.UserID)
	assert.Equal(t, "operator", session.CredentialUpdate.Username)
	assert.Equal(t, "synthetic-password", session.CredentialUpdate.Password)
	assert.Empty(t, session.CredentialUpdate.AccessToken)
	assert.Empty(t, session.CredentialUpdate.RefreshToken)
	assert.Empty(t, session.CredentialUpdate.SessionID)
	assert.Empty(t, session.CredentialUpdate.Cookie)
}

func TestNewAPIAdapterAccessTokenUsesUnexpiredDashboardSessionWithoutRefresh(t *testing.T) {
	refreshRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/auth/refresh":
			refreshRequests++
			t.Fatalf("未过期的 Dashboard Access Token 不应无条件刷新")
		case "/api/user/self":
			assert.Equal(t, "Bearer saved-access", request.Header.Get("Authorization"))
			assert.Equal(t, "saved-session", request.Header.Get("X-Auth-Session"))
			assert.Equal(t, "new_api_refresh=saved-refresh", request.Header.Get("Cookie"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType:       model.UpstreamAuthAccessToken,
			AccessToken:    "saved-access",
			TokenExpiresAt: common.GetTimestamp() + 3600,
			SessionID:      "saved-session",
			SessionCurrent: true,
			Cookie:         "new_api_refresh=saved-refresh",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 0, refreshRequests)
}

func TestNewAPIAdapterAccessTokenPersistsResponseCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path != "/api/user/self" {
			http.NotFound(writer, request)
			return
		}
		assert.Equal(t, "Bearer access-token", request.Header.Get("Authorization"))
		assert.Equal(t, "session=old", request.Header.Get("Cookie"))
		http.SetCookie(writer, &http.Cookie{
			Name:  "new_api_refresh",
			Value: "captured-refresh",
			Path:  "/api/user/auth",
		})
		_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":1}}`))
	}))
	defer server.Close()

	session, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType:    model.UpstreamAuthAccessToken,
			AccessToken: "access-token",
			Cookie:      "session=old",
		},
	)
	require.NoError(t, err)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, "17", session.CredentialUpdate.UserID)
	assert.Contains(t, session.CredentialUpdate.Cookie, "session=old")
	assert.Contains(t, session.CredentialUpdate.Cookie, "new_api_refresh=captured-refresh")
}

func TestNewAPIAdapterPasswordAlwaysLogsInWithPassword(t *testing.T) {
	refreshRequests := 0
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/auth/refresh":
			refreshRequests++
			t.Fatalf("密码认证不应调用 Dashboard Refresh")
		case "/api/user/login":
			loginRequests++
			assert.Empty(t, request.Header.Get("Authorization"))
			assert.NotContains(t, request.Header.Get("Cookie"), "old-refresh")
			assert.NotEqual(t, "old-session", request.Header.Get("X-Auth-Session"))
			_, _ = writer.Write([]byte(
				`{"success":true,"data":{"token":"login-access","user":{"id":17}}}`,
			))
		case "/api/user/self":
			assert.Equal(t, "Bearer login-access", request.Header.Get("Authorization"))
			assert.NotContains(t, request.Header.Get("Cookie"), "old-refresh")
			assert.NotEqual(t, "old-session", request.Header.Get("X-Auth-Session"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	credential := model.PlatformSiteCredential{
		AuthType:       model.UpstreamAuthPassword,
		Username:       "operator",
		Password:       "synthetic-password",
		UserID:         "17",
		AccessToken:    "expired-access",
		TokenType:      "Bearer",
		TokenExpiresAt: common.GetTimestamp() - 1,
		SessionID:      "old-session",
		SessionCurrent: true,
		Cookie:         "new_api_refresh=old-refresh",
	}
	session, err := NewNewAPIAdapter(server.Client()).Authenticate(context.Background(), server.URL, credential)
	require.NoError(t, err)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, 0, refreshRequests)
	assert.Equal(t, 1, loginRequests)
	assert.Equal(t, model.UpstreamAuthPassword, session.CredentialUpdate.AuthType)
	assert.Equal(t, "operator", session.CredentialUpdate.Username)
	assert.Equal(t, "synthetic-password", session.CredentialUpdate.Password)
	assert.Equal(t, "17", session.CredentialUpdate.UserID)
	assert.Empty(t, session.CredentialUpdate.AccessToken)
	assert.Empty(t, session.CredentialUpdate.RefreshToken)
	assert.Empty(t, session.CredentialUpdate.SessionID)
	assert.Empty(t, session.CredentialUpdate.Cookie)
}

func TestNewAPIRefreshCookieMergesJarAndExplicitHeaderWithoutDuplicates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/user/self" {
			assert.Equal(t, "new_api_refresh=jar-refresh; session=dashboard", request.Header.Get("Cookie"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17}}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	parsedURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	session.Client.Jar.SetCookies(parsedURL, []*http.Cookie{
		{Name: "new_api_refresh", Value: "jar-refresh", Path: "/"},
		{Name: "session", Value: "dashboard", Path: "/"},
	})
	credential := model.PlatformSiteCredential{Cookie: "new_api_refresh=old-refresh"}
	captureNewAPIRefreshCookie(session, &credential)
	syncNewAPISessionHeaders(session, credential)

	_, err = fetchNewAPICurrentUser(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, "new_api_refresh=jar-refresh; session=dashboard", credential.Cookie)
}

func TestPlatformSiteRequestCarriesSetCookieAcrossRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/start" {
			assert.Equal(t, "redirect-session=old", request.Header.Get("Cookie"))
			http.SetCookie(writer, &http.Cookie{
				Name:  "redirect-session",
				Value: "captured",
				Path:  "/",
			})
			http.Redirect(writer, request, "/final", http.StatusFound)
			return
		}
		if request.URL.Path == "/final" {
			assert.Equal(t, "redirect-session=captured", request.Header.Get("Cookie"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"ok":true}}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Headers.Set("Cookie", "redirect-session=old")
	_, err = platformSiteRequest(
		context.Background(),
		session,
		http.MethodGet,
		"/start",
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.Contains(t, platformSiteJarCookieHeader(session), "redirect-session=captured")
}

func TestNewAPIAdapterPasswordDoesNotRefreshBeforeLogin(t *testing.T) {
	refreshRequests := 0
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/auth/refresh":
			refreshRequests++
			t.Fatalf("密码认证不应请求历史 Refresh")
		case "/api/user/login":
			loginRequests++
			assert.Empty(t, request.Header.Get("Authorization"))
			assert.NotContains(t, request.Header.Get("Cookie"), "old-refresh")
			assert.NotEqual(t, "old-session", request.Header.Get("X-Auth-Session"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"token":"login-access","user":{"id":17}}}`))
		case "/api/user/self":
			assert.Equal(t, "Bearer login-access", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := NewNewAPIAdapter(server.Client()).Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType:       model.UpstreamAuthPassword,
		Username:       "operator",
		Password:       "synthetic-password",
		AccessToken:    "expired-access",
		TokenExpiresAt: common.GetTimestamp() - 1,
		SessionID:      "old-session",
		Cookie:         "new_api_refresh=old-refresh",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, refreshRequests)
	assert.Equal(t, 1, loginRequests)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, model.UpstreamAuthPassword, session.CredentialUpdate.AuthType)
	assert.Empty(t, session.CredentialUpdate.AccessToken)
	assert.Empty(t, session.CredentialUpdate.RefreshToken)
	assert.Empty(t, session.CredentialUpdate.SessionID)
	assert.Empty(t, session.CredentialUpdate.Cookie)
}

func TestNewAPIAdapterPasswordLoginIgnoresHistoricalRefreshMaterial(t *testing.T) {
	refreshRequests := 0
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/auth/refresh":
			refreshRequests++
			t.Fatalf("密码认证不应请求历史 Refresh")
		case "/api/user/login":
			loginRequests++
			assert.Empty(t, request.Header.Get("Authorization"))
			assert.Empty(t, request.Header.Get("X-Auth-Session"))
			assert.NotContains(t, request.Header.Get("Cookie"), "old-refresh")
			_, _ = writer.Write([]byte(`{"success":true,"data":{"token":"login-access","user":{"id":17}}}`))
		case "/api/user/self":
			assert.Equal(t, "Bearer login-access", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := NewNewAPIAdapter(server.Client()).Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType:       model.UpstreamAuthPassword,
		Username:       "operator",
		Password:       "synthetic-password",
		AccessToken:    "expired-access",
		TokenExpiresAt: common.GetTimestamp() - 1,
		RefreshToken:   "old-refresh-token",
		SessionID:      "old-session",
		AdminKey:       "old-admin-key",
		Cookie:         "new_api_refresh=old-refresh",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, refreshRequests)
	assert.Equal(t, 1, loginRequests)
	require.NotNil(t, session.CredentialUpdate)
	assert.Empty(t, session.CredentialUpdate.AccessToken)
	assert.Empty(t, session.CredentialUpdate.RefreshToken)
	assert.Empty(t, session.CredentialUpdate.SessionID)
	assert.Empty(t, session.CredentialUpdate.Cookie)
	assert.Empty(t, session.CredentialUpdate.AdminKey)
}

func TestTemporaryPasswordSessionMaterialsRequireActualMaterial(t *testing.T) {
	session, err := newPlatformSiteSession("http://127.0.0.1", nil)
	require.NoError(t, err)

	setTemporaryPasswordSessionMaterials(session, model.PlatformSiteCredential{})
	assert.False(t, session.PasswordSession)
	assert.Empty(t, session.TemporaryToken)
	assert.Empty(t, session.TemporaryRefresh)
	assert.Empty(t, session.TemporarySession)
	assert.Empty(t, session.TemporaryCookie)

	setTemporaryPasswordSessionMaterials(session, model.PlatformSiteCredential{
		AccessToken: "temporary-access",
	})
	assert.True(t, session.PasswordSession)
	assert.Equal(t, "temporary-access", session.TemporaryToken)

	clearPlatformSitePasswordSession(session)
	assert.False(t, session.PasswordSession)
	assert.Empty(t, session.TemporaryToken)
}

func TestPlatformSiteAuthFlowCleansCookieAfterPasswordLoginFailure(t *testing.T) {
	logoutRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{
				Name:  "new_api_refresh",
				Value: "failed-login-refresh",
				Path:  "/",
			})
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte(
				`{"success":false,"message":"Username or password error"}`,
			))
		case "/api/user/auth/logout":
			logoutRequests++
			assert.Equal(t, "new_api_refresh=failed-login-refresh", request.Header.Get("Cookie"))
			_, _ = writer.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := StartPlatformSiteAuthFlow(context.Background(), 9901, PlatformSiteAuthFlowStartRequest{
		Platform: model.PlatformNewAPI,
		BaseURL:  server.URL,
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
		Password: "synthetic-password",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteCredentials)
	assert.Equal(t, 1, logoutRequests)
}

func TestNewAPIAdapterCleanupLogsOutAndDeletesExactSession(t *testing.T) {
	logoutRequests := 0
	deleteRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{
				Name:  "new_api_refresh",
				Value: "temporary-refresh",
				Path:  "/",
			})
			_, _ = writer.Write([]byte(
				`{"success":true,"data":{"token":"temporary-access","session_id":"sid-123","user":{"id":7}}}`,
			))
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":7,"username":"operator"}}`))
		case "/api/user/auth/logout":
			logoutRequests++
			assert.Equal(t, http.MethodPost, request.Method)
			assert.Equal(t, "http://"+request.Host, request.Header.Get("Origin"))
			assert.Equal(t, "Bearer temporary-access", request.Header.Get("Authorization"))
			assert.Equal(t, "sid-123", request.Header.Get("X-Auth-Session"))
			assert.Contains(t, request.Header.Get("Cookie"), "new_api_refresh=temporary-refresh")
			_, _ = writer.Write([]byte(`{"success":true}`))
		case "/api/user/sessions/sid-123":
			deleteRequests++
			assert.Equal(t, http.MethodDelete, request.Method)
			assert.Equal(t, "Bearer temporary-access", request.Header.Get("Authorization"))
			assert.Equal(t, "sid-123", request.Header.Get("X-Auth-Session"))
			_, _ = writer.Write([]byte(`{"success":true}`))
		case "/api/user/sessions/revoke-others":
			t.Fatalf("密码同步不得撤销其他设备会话")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	require.True(t, session.PasswordSession)
	require.Equal(t, "temporary-access", session.TemporaryToken)
	require.Equal(t, "sid-123", session.TemporarySession)

	require.NoError(t, NewNewAPIAdapter(server.Client()).Cleanup(context.Background(), session))
	assert.Equal(t, 1, logoutRequests)
	assert.Equal(t, 1, deleteRequests)
	assert.Empty(t, session.TemporaryToken)
	assert.Empty(t, session.TemporaryRefresh)
	assert.Empty(t, session.TemporarySession)
	assert.Empty(t, session.TemporaryCookie)
	assert.Empty(t, session.Headers.Get("Authorization"))
	assert.Empty(t, session.Headers.Get("Cookie"))
	assert.Empty(t, session.Headers.Get("X-Auth-Session"))
}

func TestNewAPIAdapterCleanupTreatsLogoutAndSessionDeleteAsIdempotent(t *testing.T) {
	tests := []struct {
		name         string
		logoutStatus int
		logoutBody   string
		deleteStatus int
	}{
		{
			name:         "session mismatch",
			logoutStatus: http.StatusConflict,
			logoutBody:   `{"success":false,"code":"AUTH_SESSION_MISMATCH"}`,
			deleteStatus: http.StatusOK,
		},
		{
			name:         "logout unauthorized",
			logoutStatus: http.StatusUnauthorized,
			deleteStatus: http.StatusOK,
		},
		{
			name:         "logout forbidden",
			logoutStatus: http.StatusForbidden,
			deleteStatus: http.StatusOK,
		},
		{
			name:         "logout not found",
			logoutStatus: http.StatusNotFound,
			deleteStatus: http.StatusOK,
		},
		{
			name:         "logout method missing",
			logoutStatus: http.StatusMethodNotAllowed,
			deleteStatus: http.StatusOK,
		},
		{
			name:         "session delete unauthorized",
			logoutStatus: http.StatusOK,
			deleteStatus: http.StatusUnauthorized,
		},
		{
			name:         "session delete forbidden",
			logoutStatus: http.StatusOK,
			deleteStatus: http.StatusForbidden,
		},
		{
			name:         "session delete not found",
			logoutStatus: http.StatusOK,
			deleteStatus: http.StatusNotFound,
		},
		{
			name:         "session delete method missing",
			logoutStatus: http.StatusOK,
			deleteStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			deleteRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/user/auth/logout":
					writer.WriteHeader(testCase.logoutStatus)
					if testCase.logoutBody != "" {
						_, _ = writer.Write([]byte(testCase.logoutBody))
					}
				case "/api/user/sessions/sid-123":
					deleteRequests++
					writer.WriteHeader(testCase.deleteStatus)
					if testCase.deleteStatus == http.StatusOK {
						_, _ = writer.Write([]byte(`{"success":true}`))
					}
				case "/api/user/sessions/revoke-others":
					t.Fatalf("不得调用 revoke-others")
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			session, err := newPlatformSiteSession(server.URL, nil)
			require.NoError(t, err)
			session.Platform = model.PlatformNewAPI
			session.PasswordSession = true
			session.TemporaryToken = "temporary-access"
			session.TemporarySession = "sid-123"
			session.TemporaryCookie = "new_api_refresh=temporary-refresh"

			require.NoError(t, NewNewAPIAdapter(server.Client()).Cleanup(context.Background(), session))
			assert.Equal(t, 1, deleteRequests)
			assert.Empty(t, session.TemporaryToken)
			assert.Empty(t, session.TemporarySession)
		})
	}
}

func TestSub2APIAdapterCleanupUsesOnlyRefreshToken(t *testing.T) {
	logoutRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/settings/public":
			http.NotFound(writer, request)
		case "/api/v1/auth/login":
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"access_token":"temporary-access","refresh_token":"temporary-refresh","expires_in":3600,"user":{"id":9}}}`,
			))
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":9,"username":"operator"}}`))
		case "/api/v1/auth/logout":
			logoutRequests++
			assert.Equal(t, http.MethodPost, request.Method)
			assert.Empty(t, request.Header.Get("Authorization"))
			assert.Empty(t, request.Header.Get("Cookie"))
			body, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			var payload map[string]string
			require.NoError(t, common.Unmarshal(body, &payload))
			assert.Equal(t, map[string]string{"refresh_token": "temporary-refresh"}, payload)
			_, _ = writer.Write([]byte(`{"code":0}`))
		case "/api/v1/auth/revoke-all-sessions":
			t.Fatalf("密码同步不得撤销全部会话")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := NewSub2APIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	require.True(t, session.PasswordSession)
	require.Equal(t, "temporary-refresh", session.TemporaryRefresh)

	require.NoError(t, NewSub2APIAdapter(server.Client()).Cleanup(context.Background(), session))
	assert.Equal(t, 1, logoutRequests)
	assert.Empty(t, session.TemporaryToken)
	assert.Empty(t, session.TemporaryRefresh)
	assert.Empty(t, session.TemporarySession)
	assert.Empty(t, session.TemporaryCookie)
}

func TestSub2APIAdapterCleanupWithoutRefreshTokenClearsTemporaryMaterials(t *testing.T) {
	logoutRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		logoutRequests++
		t.Fatalf("缺少 Refresh Token 时不应请求注销接口")
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Platform = model.PlatformSub2API
	session.PasswordSession = true
	session.TemporaryToken = "temporary-access"
	session.TemporarySession = "sid-123"
	session.TemporaryCookie = "session=temporary"

	err = NewSub2APIAdapter(server.Client()).Cleanup(context.Background(), session)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteSessionCleanup)
	assert.Equal(t, 0, logoutRequests)
	assert.Empty(t, session.TemporaryToken)
	assert.Empty(t, session.TemporaryRefresh)
	assert.Empty(t, session.TemporarySession)
	assert.Empty(t, session.TemporaryCookie)
}

func TestPlatformSiteCleanupDoesNotRequestWithoutTemporarySession(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		requests++
		t.Fatalf("未建立临时会话时不应发送注销请求")
	}))
	t.Cleanup(server.Close)

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Platform = model.PlatformNewAPI
	session.PasswordSession = true

	err = NewNewAPIAdapter(server.Client()).Cleanup(context.Background(), session)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteSessionCleanup)
	assert.Zero(t, requests)
	assert.False(t, session.PasswordSession)
	assert.Empty(t, session.TemporaryToken)
	assert.Empty(t, session.TemporaryRefresh)
	assert.Empty(t, session.TemporarySession)
	assert.Empty(t, session.TemporaryCookie)
}

func TestSyncPlatformSiteMapsSessionLimitWithoutPasswordRetry(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.CryptoSecret = "upstream-site-session-limit-test-secret"
	common.MemoryCacheEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.PlatformSiteAccount{}))
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

	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/auth/refresh":
			t.Fatalf("账号密码同步不应请求历史 Refresh")
		case "/api/user/login":
			loginRequests++
			_, _ = writer.Write([]byte(`{"success":false,"code":"AUTH_SESSION_LIMIT","message":"too many active sessions"}`))
		case "/api/user/self":
			t.Fatalf("登录被会话上限拒绝时不应读取当前用户")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	channel := &model.Channel{
		Id:           904,
		Name:         "session-limit",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
	}
	require.NoError(t, db.Create(channel).Error)
	ciphertext, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{
		AuthType:       model.UpstreamAuthPassword,
		Username:       "operator",
		Password:       "synthetic-password",
		AccessToken:    "expired-access",
		TokenExpiresAt: common.GetTimestamp() - 1,
		SessionID:      "old-session",
		Cookie:         "new_api_refresh=old-refresh",
	})
	require.NoError(t, err)
	account := &model.PlatformSiteAccount{
		ChannelID:            channel.Id,
		Platform:             model.PlatformNewAPI,
		BaseURL:              server.URL,
		AuthType:             model.UpstreamAuthPassword,
		CredentialCiphertext: ciphertext,
		CredentialKeyVersion: "v1",
		SyncStatus:           model.UpstreamSiteSyncIdle,
	}
	require.NoError(t, db.Create(account).Error)

	err = SyncUpstreamSite(context.Background(), channel.Id)
	require.Error(t, err)
	assert.Equal(t, 1, loginRequests)

	var saved model.PlatformSiteAccount
	require.NoError(t, db.First(&saved, account.ID).Error)
	assert.Equal(t, model.PlatformSiteAuthStatusSessionLimit, saved.AuthStatus)
	assert.Contains(t, saved.AuthStatusReason, "会话")
	assert.NotContains(t, saved.AuthStatusReason, "synthetic-password")
	assert.Contains(t, SafePlatformSiteError(err), "清理旧会话")
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
				assert.Contains(t, string(body), `"username":"operator@example.com"`)
				assert.NotContains(t, string(body), `"email"`)
				return platformSiteJSONResponse(http.StatusOK, `{"success":true,"data":{"token":"newapi-session","user":{"uid":888}}}`), nil
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
	assert.Equal(t, 1, loginAttempts)

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
			_, _ = writer.Write([]byte(`{"success":true,"data":{"token":"newapi-session","user":{"id":88}}}`))
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

func TestNewAPITokenBatchRevealDoesNotFallbackAfterNonRouteError(t *testing.T) {
	singleRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/token/batch/keys":
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"success":false,"message":"verification required"}`))
		case "/api/token/7/key":
			singleRequests++
			t.Fatalf("批量 Key 返回非 404/405 时不应调用单条回退")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	revealed, failures := fetchNewAPITokenKeys(context.Background(), session, []map[string]any{{
		"id":  7,
		"key": "sk-****",
	}})
	assert.Empty(t, revealed)
	require.Error(t, failures["7"])
	assert.ErrorIs(t, failures["7"], ErrPlatformSiteSecurity)
	assert.Equal(t, 0, singleRequests)
}

func TestNewAPITokenBatchRevealFallsBackOnlyForMissingIDs(t *testing.T) {
	singleRequests := make([]string, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/token/batch/keys":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"keys":{"7":"fixture-batch-key"}}}`))
		case "/api/token/8/key":
			singleRequests = append(singleRequests, request.Method)
			require.Equal(t, http.MethodPost, request.Method)
			_, _ = writer.Write([]byte(`{"success":true,"data":{"key":"fixture-single-key"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	revealed, failures := fetchNewAPITokenKeys(context.Background(), session, []map[string]any{
		{"id": 7, "key": "sk-****"},
		{"id": 8, "key": "sk-****"},
	})
	require.Empty(t, failures)
	assert.Equal(t, map[string]string{
		"7": "fixture-batch-key",
		"8": "fixture-single-key",
	}, revealed)
	assert.Equal(t, []string{http.MethodPost}, singleRequests)
}

func TestNewAPITokenKeyUsesGETOnlyAfterPOSTRouteMissing(t *testing.T) {
	t.Run("404 允许 GET 兼容", func(t *testing.T) {
		methods := make([]string, 0, 2)
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			methods = append(methods, request.Method)
			if request.Method == http.MethodPost {
				http.NotFound(writer, request)
				return
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":{"key":"fixture-get-key"}}`))
		}))
		defer server.Close()

		session, err := newPlatformSiteSession(server.URL, nil)
		require.NoError(t, err)
		session.Client = server.Client()
		key, err := fetchNewAPITokenKey(context.Background(), session, "7")
		require.NoError(t, err)
		assert.Equal(t, "fixture-get-key", key)
		assert.Equal(t, []string{http.MethodPost, http.MethodGet}, methods)
	})

	t.Run("403 不允许 GET 兼容", func(t *testing.T) {
		methods := make([]string, 0, 1)
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			methods = append(methods, request.Method)
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"success":false,"message":"verification required"}`))
		}))
		defer server.Close()

		session, err := newPlatformSiteSession(server.URL, nil)
		require.NoError(t, err)
		session.Client = server.Client()
		_, err = fetchNewAPITokenKey(context.Background(), session, "7")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPlatformSiteSecurity)
		assert.Equal(t, []string{http.MethodPost}, methods)
	})
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

func TestPlatformSitePasswordAuthenticationPersistsOnlyPasswordState(t *testing.T) {
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
			loginBody:   `{"success":true,"data":{"token":"session","refresh_token":"rotated"}}`,
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
					t.Fatalf("密码认证不应尝试刷新令牌")
				case testCase.loginPath:
					assert.Empty(t, request.Header.Get("Authorization"))
					assert.Empty(t, request.Header.Get("Cookie"))
					assert.Empty(t, request.Header.Get("X-Auth-Session"))
					_, _ = writer.Write([]byte(testCase.loginBody))
				case testCase.selfPath:
					assert.Equal(t, "Bearer session", request.Header.Get("Authorization"))
					_, _ = writer.Write([]byte(testCase.selfBody))
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
				SessionID:      "stale-session",
				Cookie:         "new_api_refresh=stale-refresh",
			}
			session, err := testCase.newAdapter(server.Client()).Authenticate(context.Background(), server.URL, credential)
			require.NoError(t, err)
			require.NotNil(t, session.CredentialUpdate)
			assert.Equal(t, model.UpstreamAuthPassword, session.CredentialUpdate.AuthType)
			assert.Equal(t, "operator@example.com", session.CredentialUpdate.Username)
			assert.Equal(t, "synthetic-password", session.CredentialUpdate.Password)
			assert.Empty(t, session.CredentialUpdate.AccessToken)
			assert.Empty(t, session.CredentialUpdate.RefreshToken)
			assert.Empty(t, session.CredentialUpdate.TokenExpiresAt)
			assert.Empty(t, session.CredentialUpdate.SessionID)
			assert.Empty(t, session.CredentialUpdate.Cookie)
		})
	}
}

func TestPlatformSitePasswordAuthenticationPersistsCurrentUsername(t *testing.T) {
	tests := []struct {
		name      string
		adapter   func(*http.Client) PlatformSiteAdapter
		loginPath string
		selfPath  string
		selfBody  string
		wantID    string
		wantName  string
		username  string
	}{
		{
			name:      "NewAPI username",
			adapter:   func(client *http.Client) PlatformSiteAdapter { return NewNewAPIAdapter(client) },
			loginPath: "/api/user/login",
			selfPath:  "/api/user/self",
			selfBody:  `{"success":true,"data":{"id":17,"username":"newapi-user","email":"newapi@example.com"}}`,
			wantID:    "17",
			wantName:  "newapi-user",
		},
		{
			name:      "Sub2API user_name",
			adapter:   func(client *http.Client) PlatformSiteAdapter { return NewSub2APIAdapter(client) },
			loginPath: "/api/v1/auth/login",
			selfPath:  "/api/v1/auth/me",
			selfBody:  `{"code":0,"data":{"id":23,"user_name":"sub2-user","email":"sub2@example.com"}}`,
			wantID:    "23",
			wantName:  "sub2-user",
		},
		{
			name:      "Sub2API email keeps login identity",
			adapter:   func(client *http.Client) PlatformSiteAdapter { return NewSub2APIAdapter(client) },
			loginPath: "/api/v1/auth/login",
			selfPath:  "/api/v1/auth/me",
			selfBody:  `{"code":0,"data":{"id":24,"username":"sub2-display-name","email":"operator@example.com"}}`,
			wantID:    "24",
			wantName:  "operator@example.com",
			username:  "operator@example.com",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				request *http.Request,
			) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/api/v1/settings/public":
					http.NotFound(writer, request)
				case testCase.loginPath:
					if testCase.loginPath == "/api/user/login" {
						_, _ = writer.Write([]byte(
							`{"success":true,"data":{"token":"temporary-access"}}`,
						))
					} else {
						_, _ = writer.Write([]byte(
							`{"code":0,"data":{"access_token":"temporary-access","refresh_token":"temporary-refresh"}}`,
						))
					}
				case testCase.selfPath:
					_, _ = writer.Write([]byte(testCase.selfBody))
				default:
					http.NotFound(writer, request)
				}
			}))
			t.Cleanup(server.Close)

			session, err := testCase.adapter(server.Client()).Authenticate(
				context.Background(),
				server.URL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: firstNonEmptyString(testCase.username, "operator"),
					Password: "synthetic-password",
				},
			)
			require.NoError(t, err)
			require.NotNil(t, session.CredentialUpdate)
			assert.Equal(t, testCase.wantID, session.CredentialUpdate.UserID)
			assert.Equal(t, testCase.wantName, session.CredentialUpdate.Username)
			assert.NotZero(t, session.CredentialUpdate.LastAuthAt)
			assert.Empty(t, session.CredentialUpdate.AccessToken)
			assert.Empty(t, session.CredentialUpdate.RefreshToken)
			assert.Empty(t, session.CredentialUpdate.SessionID)
			assert.Empty(t, session.CredentialUpdate.Cookie)
		})
	}
}

func TestSub2APIAdapterParsesRealLoginEnvelopeWithoutCredentialDrift(t *testing.T) {
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
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
		AuthType:     model.UpstreamAuthPassword,
		Username:     "operator@example.com",
		Password:     "synthetic-password",
		AccessToken:  "stale-access-token",
		RefreshToken: "stale-refresh-token",
	}
	session, err := NewSub2APIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		credential,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, loginRequests)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, model.UpstreamAuthPassword, session.CredentialUpdate.AuthType)
	assert.Equal(t, "operator@example.com", session.CredentialUpdate.Username)
	assert.Equal(t, "synthetic-password", session.CredentialUpdate.Password)
	assert.Empty(t, session.CredentialUpdate.AccessToken)
	assert.Empty(t, session.CredentialUpdate.RefreshToken)
	assert.Empty(t, session.CredentialUpdate.SessionID)
	assert.Empty(t, session.CredentialUpdate.Cookie)
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
				AuthType:     model.UpstreamAuthAccessToken,
				Username:     "operator",
				Password:     "synthetic-password",
				AccessToken:  "stale-access",
				RefreshToken: "stale-refresh",
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPlatformSiteAuth)
		})
	}
}

func TestNewAPIAdapterDoesNotSendLegacyRefreshWithoutDashboardCookie(t *testing.T) {
	refreshRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/auth/refresh":
			refreshRequests++
			t.Fatalf("缺少 new_api_refresh Cookie 时不应调用 Dashboard Refresh")
		case "/api/user/self":
			assert.Equal(t, "Bearer legacy-access", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType:       model.UpstreamAuthAccessToken,
			AccessToken:    "legacy-access",
			TokenExpiresAt: common.GetTimestamp() + 3600,
			RefreshToken:   "legacy-refresh",
			SessionID:      "legacy-session",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 0, refreshRequests)
}

func TestNewAPIAdapterReportsUncertainLegacyRefreshWithoutDashboardCookie(t *testing.T) {
	refreshRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/user/auth/refresh" {
			refreshRequests++
			t.Fatalf("缺少 new_api_refresh Cookie 时不应调用 Dashboard Refresh")
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	_, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType:     model.UpstreamAuthAccessToken,
			AccessToken:  "expired-access",
			RefreshToken: "legacy-refresh",
			SessionID:    "legacy-session",
		},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteRefreshUncertain)
	assert.Equal(t, 0, refreshRequests)
}

func TestRefreshPlatformSiteSessionRequiresDashboardRefreshCookie(t *testing.T) {
	refreshRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		refreshRequests++
		http.Error(writer, `{"success":false}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	err = refreshPlatformSiteSession(
		context.Background(),
		session,
		"/api/user/auth/refresh",
		&model.PlatformSiteCredential{
			AccessToken:  "expired-access",
			SessionID:    "legacy-session",
			RefreshToken: "legacy-refresh",
		},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteRefreshUncertain)
	assert.ErrorIs(t, err, ErrPlatformSiteAuth)
	assert.Equal(t, 0, refreshRequests)
}

func TestNewAPIAdapterRefreshesRotatingDashboardSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/auth/refresh":
			assert.Equal(t, "Bearer old-access", request.Header.Get("Authorization"))
			assert.Equal(t, "old-session", request.Header.Get("X-Auth-Session"))
			assert.Equal(t, "new_api_refresh=old-refresh", request.Header.Get("Cookie"))
			assert.Equal(t, "http://"+request.Host, request.Header.Get("Origin"))
			assert.Equal(t, "http://"+request.Host+"/login", request.Header.Get("Referer"))
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Empty(t, string(body))
			http.SetCookie(writer, &http.Cookie{
				Name:  "new_api_refresh",
				Value: "new-refresh",
				Path:  "/api/user/auth",
			})
			_, _ = writer.Write([]byte(fmt.Sprintf(
				`{"success":true,"data":{"access_token":"new-access","token_type":"Bearer","access_expires_at":%d,"session":{"sid":"new-session","current":true},"user":{"id":17}}}`,
				common.GetTimestamp()+3600,
			)))
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
		AuthType:     model.UpstreamAuthAccessToken,
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		SessionID:    "old-session",
		Cookie:       "new_api_refresh=old-refresh",
	})
	require.NoError(t, err)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, "new-access", session.CredentialUpdate.AccessToken)
	assert.Equal(t, "new-session", session.CredentialUpdate.SessionID)
	assert.Contains(t, session.CredentialUpdate.Cookie, "new_api_refresh=new-refresh")
	assert.Greater(t, session.CredentialUpdate.TokenExpiresAt, common.GetTimestamp())
}

func TestNewAPIAdapterRefreshWithoutRotatedCookieIsUncertain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/auth/refresh":
			assert.Equal(t, "new_api_refresh=old-refresh", request.Header.Get("Cookie"))
			_, _ = writer.Write([]byte(fmt.Sprintf(
				`{"success":true,"data":{"access_token":"new-access","token_type":"Bearer","access_expires_at":%d,"session":{"sid":"new-session","current":true},"user":{"id":17}}}`,
				common.GetTimestamp()+3600,
			)))
		case "/api/user/self":
			t.Fatalf("Refresh Cookie 未轮换时不应继续读取当前用户")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType:       model.UpstreamAuthAccessToken,
			AccessToken:    "old-access",
			TokenExpiresAt: common.GetTimestamp() - 1,
			SessionID:      "old-session",
			Cookie:         "new_api_refresh=old-refresh",
		},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteRefreshUncertain)
	assert.NotContains(t, err.Error(), "old-refresh")
	assert.NotContains(t, err.Error(), "new-access")
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

func TestPlatformSiteAuthFlowBindsIdentityAndConsumesAfterNewAPITwoFA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{
				Name:  "new_api_refresh",
				Value: "challenge-refresh",
				Path:  "/api/user/auth",
			})
			_, _ = writer.Write([]byte(`{"success":true,"data":{"require_2fa":true,"flow_token":"upstream-flow"}}`))
		case "/api/user/login/2fa":
			assert.Equal(t, "new_api_refresh=challenge-refresh", request.Header.Get("Cookie"))
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"code":"123456"`)
			assert.Contains(t, string(body), `"flow_token":"upstream-flow"`)
			_, _ = writer.Write([]byte(fmt.Sprintf(
				`{"success":true,"data":{"access_token":"newapi-access","token_type":"Bearer","access_expires_at":%d,"session":{"sid":"newapi-session","current":true},"user":{"id":17}}}`,
				common.GetTimestamp()+3600,
			)))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	started, err := StartPlatformSiteAuthFlow(context.Background(), 41, PlatformSiteAuthFlowStartRequest{
		Platform:  model.PlatformNewAPI,
		BaseURL:   server.URL,
		AuthType:  model.UpstreamAuthPassword,
		Username:  "operator",
		Password:  "synthetic-password",
		ChannelID: 7,
	})
	require.NoError(t, err)
	assert.Equal(t, PlatformSiteAuthFlowStatusTwoFactorRequired, started.Status)
	assert.NotEmpty(t, started.FlowID)
	assert.NotContains(t, started.FlowID, "upstream-flow")
	assert.NotContains(t, fmt.Sprint(started), "synthetic-password")

	_, err = VerifyPlatformSiteAuthFlow(
		context.Background(),
		99,
		started.FlowID,
		PlatformSiteAuthFlowVerifyRequest{Code: "123456"},
	)
	assert.ErrorIs(t, err, ErrPlatformSiteAuthFlowInvalid)

	_, err = VerifyPlatformSiteAuthFlow(
		context.Background(),
		41,
		started.FlowID,
		PlatformSiteAuthFlowVerifyRequest{Code: "bad"},
	)
	assert.ErrorIs(t, err, ErrPlatformSiteAuthFlowCodeInvalid)

	verified, err := VerifyPlatformSiteAuthFlow(
		context.Background(),
		41,
		started.FlowID,
		PlatformSiteAuthFlowVerifyRequest{Code: "123456"},
	)
	require.NoError(t, err)
	assert.Equal(t, PlatformSiteAuthFlowStatusAuthenticated, verified.Status)
	assert.Equal(t, model.UpstreamAuthPassword, verified.AuthType)
	assert.Equal(t, "17", verified.UserID)
	assert.NotContains(t, fmt.Sprint(verified), "newapi-access")
	assert.NotContains(t, fmt.Sprint(verified), "newapi-refresh")

	_, err = ResolvePlatformSiteAuthFlow(
		41,
		started.FlowID,
		7,
		model.PlatformSub2API,
		server.URL,
	)
	assert.ErrorIs(t, err, ErrPlatformSiteAuthFlowInvalid)

	resolution, err := ResolvePlatformSiteAuthFlow(
		41,
		started.FlowID,
		7,
		model.PlatformNewAPI,
		server.URL,
	)
	require.NoError(t, err)
	assert.Empty(t, resolution.Credential.AccessToken)
	assert.Empty(t, resolution.Credential.RefreshToken)
	assert.Empty(t, resolution.Credential.SessionID)
	assert.Empty(t, resolution.Credential.Cookie)
	assert.Equal(t, "17", resolution.Credential.UserID)
	assert.Equal(t, model.UpstreamAuthPassword, resolution.Credential.AuthType)

	require.NoError(t, ConsumePlatformSiteAuthFlow(41, started.FlowID, 7))
	_, err = ResolvePlatformSiteAuthFlow(
		41,
		started.FlowID,
		7,
		model.PlatformNewAPI,
		server.URL,
	)
	assert.Error(t, err)
}

func TestPlatformSiteAuthFlowPersistsRotatedCookieAfterNewAPI2FAError(t *testing.T) {
	twoFARequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{
				Name:  "new_api_refresh",
				Value: "challenge-refresh",
				Path:  "/api/user/auth",
			})
			_, _ = writer.Write([]byte(`{"success":true,"data":{"require_2fa":true,"flow_token":"flow-token"}}`))
		case "/api/user/login/2fa":
			twoFARequests++
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"flow_token":"flow-token"`)
			switch twoFARequests {
			case 1:
				assert.Equal(t, "new_api_refresh=challenge-refresh", request.Header.Get("Cookie"))
				http.SetCookie(writer, &http.Cookie{
					Name:  "new_api_refresh",
					Value: "rotated-refresh",
					Path:  "/api/user/auth",
				})
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = writer.Write([]byte(`{"success":false,"code":401,"message":"invalid verification code"}`))
			case 2:
				assert.Equal(t, "new_api_refresh=rotated-refresh", request.Header.Get("Cookie"))
				http.SetCookie(writer, &http.Cookie{
					Name:  "new_api_refresh",
					Value: "final-refresh",
					Path:  "/api/user/auth",
				})
				_, _ = writer.Write([]byte(fmt.Sprintf(
					`{"success":true,"data":{"access_token":"twofa-access","token_type":"Bearer","access_expires_at":%d,"session":{"sid":"twofa-session","current":true},"user":{"id":42}}}`,
					common.GetTimestamp()+3600,
				)))
			default:
				t.Fatalf("不应重复提交超过一次二次验证")
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	started, err := StartPlatformSiteAuthFlow(context.Background(), 42, PlatformSiteAuthFlowStartRequest{
		Platform: model.PlatformNewAPI,
		BaseURL:  server.URL,
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
		Password: "synthetic-password",
	})
	require.NoError(t, err)
	assert.Equal(t, PlatformSiteAuthFlowStatusTwoFactorRequired, started.Status)

	_, err = VerifyPlatformSiteAuthFlow(
		context.Background(),
		42,
		started.FlowID,
		PlatformSiteAuthFlowVerifyRequest{Code: "111111"},
	)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "rotated-refresh")
	assert.NotContains(t, err.Error(), "flow-token")

	verified, err := VerifyPlatformSiteAuthFlow(
		context.Background(),
		42,
		started.FlowID,
		PlatformSiteAuthFlowVerifyRequest{Code: "222222"},
	)
	require.NoError(t, err)
	assert.Equal(t, PlatformSiteAuthFlowStatusAuthenticated, verified.Status)
	assert.Equal(t, model.UpstreamAuthPassword, verified.AuthType)
	assert.Equal(t, "42", verified.UserID)
	assert.Equal(t, 2, twoFARequests)

	resolution, err := ResolvePlatformSiteAuthFlow(
		42,
		started.FlowID,
		0,
		model.PlatformNewAPI,
		server.URL,
	)
	require.NoError(t, err)
	assert.Empty(t, resolution.Credential.AccessToken)
	assert.Empty(t, resolution.Credential.RefreshToken)
	assert.Empty(t, resolution.Credential.SessionID)
	assert.Empty(t, resolution.Credential.Cookie)
	assert.Equal(t, model.UpstreamAuthPassword, resolution.Credential.AuthType)
	assert.Equal(t, "synthetic-password", resolution.Credential.Password)
}

func TestNewAPIAuthFlowRejectsIncompleteModernBundle(t *testing.T) {
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/api/user/login" {
			loginRequests++
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(
				`{"success":true,"data":{"access_token":"partial-access","token_type":"Bearer"}}`,
			))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	_, err := StartPlatformSiteAuthFlow(context.Background(), 41, PlatformSiteAuthFlowStartRequest{
		Platform: model.PlatformNewAPI,
		BaseURL:  server.URL,
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
		Password: "synthetic-password",
	})
	require.Error(t, err)
	assert.Equal(t, 1, loginRequests)
	assert.ErrorIs(t, err, ErrPlatformSiteAuthBundle)
	assert.NotContains(t, err.Error(), "partial-access")
	assert.NotContains(t, err.Error(), "synthetic-password")
}

func TestNewAPIAuthFlowLegacyLoginPreservesPasswordCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost && request.URL.Path == "/api/user/login" {
			_, _ = writer.Write([]byte(
				`{"success":true,"data":{"token":"legacy-access","user":{"id":17}}}`,
			))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	started, err := StartPlatformSiteAuthFlow(context.Background(), 41, PlatformSiteAuthFlowStartRequest{
		Platform:  model.PlatformNewAPI,
		BaseURL:   server.URL,
		AuthType:  model.UpstreamAuthPassword,
		Username:  "operator",
		Password:  "synthetic-password",
		ChannelID: 7,
	})
	require.NoError(t, err)
	assert.Equal(t, PlatformSiteAuthFlowStatusAuthenticated, started.Status)
	assert.Equal(t, model.UpstreamAuthPassword, started.AuthType)
	assert.Equal(t, "17", started.UserID)

	resolution, err := ResolvePlatformSiteAuthFlow(
		41,
		started.FlowID,
		7,
		model.PlatformNewAPI,
		server.URL,
	)
	require.NoError(t, err)
	assert.Equal(t, model.UpstreamAuthPassword, resolution.Credential.AuthType)
	assert.Equal(t, "operator", resolution.Credential.Username)
	assert.Equal(t, "synthetic-password", resolution.Credential.Password)
	assert.Empty(t, resolution.Credential.AccessToken)
	assert.Empty(t, resolution.Credential.RefreshToken)
	assert.Empty(t, resolution.Credential.SessionID)
	assert.Empty(t, resolution.Credential.Cookie)
}

func TestPlatformSiteAuthFlowRejectsEmptyCurrentUserResponse(t *testing.T) {
	logoutRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			_, _ = writer.Write([]byte(
				`{"success":true,"data":{"token":"temporary-access","user":{"id":17}}}`,
			))
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{}}`))
		case "/api/user/auth/logout":
			logoutRequests++
			_, _ = writer.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := StartPlatformSiteAuthFlow(context.Background(), 9902, PlatformSiteAuthFlowStartRequest{
		Platform: model.PlatformNewAPI,
		BaseURL:  server.URL,
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
		Password: "synthetic-password",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteAuth)
	assert.Equal(t, 1, logoutRequests)
}

func TestSub2APIAuthFlowTwoFAUsesBrowserHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"requires_2fa":true,"temp_token":"sub2-temp"}}`))
		case "/api/v1/auth/login/2fa":
			assert.Equal(t, "http://"+request.Host, request.Header.Get("Origin"))
			assert.Equal(t, "http://"+request.Host+"/login", request.Header.Get("Referer"))
			assert.Equal(t, "NexusTok-UpstreamSite/1.0", request.Header.Get("User-Agent"))
			assert.Equal(t, "XMLHttpRequest", request.Header.Get("X-Requested-With"))
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"temp_token":"sub2-temp"`)
			assert.Contains(t, string(body), `"totp_code":"654321"`)
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"sub2-access","refresh_token":"sub2-refresh","expires_in":3600,"user":{"id":23}}}`))
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":23,"user_name":"sub-operator"}}`))
		case "/api/v1/auth/logout":
			_, _ = writer.Write([]byte(`{"code":0}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	started, err := StartPlatformSiteAuthFlow(context.Background(), 41, PlatformSiteAuthFlowStartRequest{
		Platform: model.PlatformSub2API,
		BaseURL:  server.URL,
		AuthType: model.UpstreamAuthPassword,
		Username: "operator@example.com",
		Password: "synthetic-password",
	})
	require.NoError(t, err)
	assert.Equal(t, PlatformSiteAuthFlowStatusTwoFactorRequired, started.Status)

	verified, err := VerifyPlatformSiteAuthFlow(
		context.Background(),
		41,
		started.FlowID,
		PlatformSiteAuthFlowVerifyRequest{Code: "654321"},
	)
	require.NoError(t, err)
	assert.Equal(t, "23", verified.UserID)
	assert.Equal(t, "sub-operator", verified.Username)
	assert.Equal(t, model.UpstreamAuthPassword, verified.AuthType)
	resolution, err := ResolvePlatformSiteAuthFlow(
		41,
		started.FlowID,
		0,
		model.PlatformSub2API,
		server.URL,
	)
	require.NoError(t, err)
	assert.Empty(t, resolution.Credential.AccessToken)
	assert.Empty(t, resolution.Credential.RefreshToken)
	assert.Empty(t, resolution.Credential.SessionID)
	assert.Empty(t, resolution.Credential.Cookie)
	assert.Equal(t, "sub-operator", resolution.Credential.Username)
}

func TestSub2APIAuthFlowResolutionNormalizesManagementBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost && request.URL.Path == "/api/v1/auth/login" {
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"access_token":"sub2-access","user":{"id":43}}}`,
			))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	started, err := StartPlatformSiteAuthFlow(context.Background(), 43, PlatformSiteAuthFlowStartRequest{
		Platform: model.PlatformSub2API,
		BaseURL:  server.URL + "/v1",
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
		Password: "synthetic-password",
	})
	require.NoError(t, err)
	assert.Equal(t, server.URL, started.BaseURL)

	resolution, err := ResolvePlatformSiteAuthFlow(
		43,
		started.FlowID,
		0,
		model.PlatformSub2API,
		server.URL+"/v1",
	)
	require.NoError(t, err)
	assert.Empty(t, resolution.Credential.AccessToken)
	assert.Empty(t, resolution.Credential.RefreshToken)
	assert.Empty(t, resolution.Credential.SessionID)
	assert.Empty(t, resolution.Credential.Cookie)
}

func TestSub2APIAuthFlowTwoFAUsesLatestLoginAgreementRevision(t *testing.T) {
	settingsRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/settings/public":
			settingsRequests++
			_, _ = fmt.Fprintf(
				writer,
				`{"code":0,"data":{"login_agreement_enabled":true,"login_agreement_revision":"terms-rev-latest"}}`,
			)
		case "/api/v1/auth/login":
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			var loginPayload map[string]any
			require.NoError(t, common.Unmarshal(body, &loginPayload))
			_, hasAgreedRevision := loginPayload["agreed_revision"]
			assert.False(t, hasAgreedRevision)
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"requires_2fa":true,"temp_token":"sub2-temp"}}`,
			))
		case "/api/v1/auth/login/2fa":
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			var verifyPayload map[string]any
			require.NoError(t, common.Unmarshal(body, &verifyPayload))
			assert.Equal(t, "terms-rev-latest", verifyPayload["agreed_revision"])
			assert.Equal(t, "sub2-temp", verifyPayload["temp_token"])
			assert.Equal(t, "654321", verifyPayload["totp_code"])
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"access_token":"sub2-access","refresh_token":"sub2-refresh","expires_in":3600,"user":{"id":23}}}`,
			))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	started, err := StartPlatformSiteAuthFlow(context.Background(), 41, PlatformSiteAuthFlowStartRequest{
		Platform: model.PlatformSub2API,
		BaseURL:  server.URL,
		AuthType: model.UpstreamAuthPassword,
		Username: "operator@example.com",
		Password: "synthetic-password",
	})
	require.NoError(t, err)
	assert.Equal(t, PlatformSiteAuthFlowStatusTwoFactorRequired, started.Status)
	t.Cleanup(func() {
		_ = DeletePlatformSiteAuthFlow(41, started.FlowID)
	})

	verified, err := VerifyPlatformSiteAuthFlow(
		context.Background(),
		41,
		started.FlowID,
		PlatformSiteAuthFlowVerifyRequest{Code: "654321"},
	)
	require.NoError(t, err)
	assert.Equal(t, PlatformSiteAuthFlowStatusAuthenticated, verified.Status)
	assert.Equal(t, 1, settingsRequests)
}

func TestSub2APIAuthFlowTwoFAAgreementRejectionPersistsDedicatedStatus(t *testing.T) {
	settingsRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/settings/public":
			settingsRequests++
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"login_agreement_enabled":true,"login_agreement_revision":"terms-rev"}}`,
			))
		case "/api/v1/auth/login":
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"requires_2fa":true,"temp_token":"sub2-temp"}}`,
			))
		case "/api/v1/auth/login/2fa":
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(
				`{"code":400,"message":"agreement required","detail":"upstream-secret-response"}`,
			))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	started, err := StartPlatformSiteAuthFlow(context.Background(), 42, PlatformSiteAuthFlowStartRequest{
		Platform: model.PlatformSub2API,
		BaseURL:  server.URL,
		AuthType: model.UpstreamAuthPassword,
		Username: "operator@example.com",
		Password: "synthetic-password",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = DeletePlatformSiteAuthFlow(42, started.FlowID)
	})

	_, err = VerifyPlatformSiteAuthFlow(
		context.Background(),
		42,
		started.FlowID,
		PlatformSiteAuthFlowVerifyRequest{Code: "654321"},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSub2APILoginAgreement)
	assert.NotContains(t, err.Error(), "upstream-secret-response")
	assert.NotContains(t, err.Error(), "synthetic-password")
	assert.Equal(t, 1, settingsRequests)

	record, err := getPlatformSiteAuthFlowRecord(started.FlowID)
	require.NoError(t, err)
	assert.Equal(t, PlatformSiteAuthFlowStatusLoginAgreementRequired, record.Status)
	assert.Zero(t, record.CodeAttempts)
}

func TestNewAPIAdminSnapshotMergesAccountAndChannelModelsAndCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":500000}}`))
		case "/api/user/models":
			_, _ = writer.Write([]byte(`{"success":true,"data":["account-model"]}`))
		case "/api/user/self/groups":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"default":{"ratio":1}}}`))
		case "/api/pricing":
			_, _ = writer.Write([]byte(`{"success":true,"data":[],"group_ratio":{"default":1},"supported_endpoint":{"openai":{"path":"/v1/chat/completions","method":"POST"}}}`))
		case "/api/channel/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":9,"models":["channel-list-model"]}]}}`))
		case "/api/channel/9":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"models":["channel-detail-model"]}}`))
		case "/api/channel/fetch_models/9":
			_, _ = writer.Write([]byte(`{"success":true,"data":["channel-fetch-model"]}`))
		case "/api/token/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthAdminKey,
			AdminKey: "admin-secret",
		},
	)
	require.NoError(t, err)

	snapshot, err := NewNewAPIAdapter(server.Client()).FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"account-model",
		"channel-list-model",
		"channel-detail-model",
		"channel-fetch-model",
	}, snapshot.Models)
	assert.Contains(t, snapshot.Endpoint.Capabilities, PlatformSiteEndpointCapabilitySnapshot{
		Protocol:   "admin",
		HTTPMethod: http.MethodGet,
		Path:       "/api/channel/fetch_models/{id}",
		Supported:  true,
		SourceData: "route",
	})
}

func TestNewAPIAdminResourceFailureIsolatedFromAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":500000}}`))
		case "/api/user/models", "/api/user/self/groups", "/api/pricing":
			http.NotFound(writer, request)
		case "/api/channel/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":9}]}}`))
		case "/api/channel/9":
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"success":false,"code":403,"message":"permission denied"}`))
		case "/api/channel/fetch_models/9":
			http.NotFound(writer, request)
		case "/api/token/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthAdminKey,
			AdminKey: "admin-secret",
		},
	)
	require.NoError(t, err)

	snapshot, err := NewNewAPIAdapter(server.Client()).FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.NotNil(t, snapshot.Identity)
	assert.Equal(t, float64(1), snapshot.Balance)
	assert.True(t, platformSiteSnapshotHasBlockingResourceFailure(nil, snapshot))

	var modelResource *PlatformSiteResourceSyncSnapshot
	for index := range snapshot.ResourceSyncs {
		if snapshot.ResourceSyncs[index].ResourceType == model.PlatformSiteResourceModels {
			modelResource = &snapshot.ResourceSyncs[index]
		}
	}
	require.NotNil(t, modelResource)
	assert.Equal(t, model.PlatformSiteResourceStatusFailed, modelResource.Status)
	assert.Contains(t, modelResource.FailureReason, "权限")
}

func TestPlatformSiteSnapshotCoreResourcesDefineBlockingFailure(t *testing.T) {
	snapshot := PlatformSiteSnapshot{
		ResourceSyncs: []PlatformSiteResourceSyncSnapshot{
			{
				ResourceType: model.PlatformSiteResourceEndpoints,
				Status:       model.PlatformSiteResourceStatusFailed,
			},
			{
				ResourceType:                 model.PlatformSiteResourceKeys,
				Status:                       model.PlatformSiteResourceStatusSecureVerificationRequired,
				RequiresSecurityVerification: true,
			},
		},
		Keys: []UpstreamKeySnapshot{
			{
				ExternalID:   "healthy",
				Secret:       "sk-healthy",
				Models:       []string{"gpt-4o"},
				ModelsSynced: true,
			},
			{
				ExternalID: "unavailable",
				SyncError:  upstreamKeySyncErrorSecretUnavailable,
			},
		},
	}
	assert.False(t, platformSiteSnapshotHasBlockingResourceFailure(nil, snapshot))

	snapshot.Keys[0].SyncError = upstreamKeySyncErrorModelsUnavailable
	assert.True(t, platformSiteSnapshotHasBlockingResourceFailure(nil, snapshot))
}

func TestPlatformSiteSnapshotModelProbeFailureUsesExistingSnapshot(t *testing.T) {
	snapshot := PlatformSiteSnapshot{
		Keys: []UpstreamKeySnapshot{
			{
				ExternalID:   "key-with-secret",
				Secret:       "sk-current",
				SyncError:    upstreamKeySyncErrorModelsUnavailable,
				ModelsSynced: false,
			},
		},
	}
	account := &model.PlatformSiteAccount{
		LastSyncAt: 1_700_000_000,
		SyncStatus: model.UpstreamSiteSyncSuccess,
	}

	assert.False(t, platformSiteSnapshotHasBlockingResourceFailure(account, snapshot))

	account.LastSyncAt = 0
	assert.True(t, platformSiteSnapshotHasBlockingResourceFailure(account, snapshot))
}

func TestNewAPIResourceFailuresKeepIdentityAndClassifyOptionalResources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/status":
			writer.WriteHeader(http.StatusBadGateway)
			_, _ = writer.Write([]byte(`{"success":false,"code":"UPSTREAM_STATUS_UNAVAILABLE","message":"status unavailable"}`))
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":500000}}`))
		case "/api/user/models":
			_, _ = writer.Write([]byte(`{"success":false,"message":"model catalog unavailable"}`))
		case "/api/token/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType:    model.UpstreamAuthAccessToken,
			AccessToken: "access-token",
		},
	)
	require.NoError(t, err)

	snapshot, err := NewNewAPIAdapter(server.Client()).FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	require.NotNil(t, snapshot.Identity)
	assert.Equal(t, "17", snapshot.Identity.PlatformUserID)
	assert.Equal(t, float64(1), snapshot.Balance)

	resources := make(map[string]PlatformSiteResourceSyncSnapshot)
	for _, resource := range snapshot.ResourceSyncs {
		resources[resource.ResourceType] = resource
	}
	assert.Equal(t, model.PlatformSiteResourceStatusFailed, resources[model.PlatformSiteResourceUsage].Status)
	assert.Equal(t, model.PlatformSiteResourceStatusFailed, resources[model.PlatformSiteResourceModels].Status)
	assert.Contains(t, resources[model.PlatformSiteResourceModels].FailureReason, "资源")
	assert.NotContains(t, resources[model.PlatformSiteResourceModels].FailureReason, "账号或密码")
}

func TestNormalizePlatformSiteModelResourceStatusPrefersFreshSources(t *testing.T) {
	t.Run("permission failure with fresh source becomes partial", func(t *testing.T) {
		snapshot := PlatformSiteSnapshot{ResourceSyncs: []PlatformSiteResourceSyncSnapshot{
			{
				ResourceType:   model.PlatformSiteResourceModels,
				Status:         model.PlatformSiteResourceStatusSuccess,
				SourceEndpoint: "/v1/models",
			},
			{
				ResourceType:   model.PlatformSiteResourceModels,
				Status:         model.PlatformSiteResourceStatusFailed,
				SourceEndpoint: "/api/user/models",
				FailureReason:  "上游平台资源权限不足",
			},
		}}
		normalizePlatformSiteResourceSyncs(&snapshot)
		require.Len(t, snapshot.ResourceSyncs, 1)
		assert.Equal(t, model.PlatformSiteResourceStatusPartial, snapshot.ResourceSyncs[0].Status)
	})

	t.Run("route missing does not hide fresh source", func(t *testing.T) {
		snapshot := PlatformSiteSnapshot{ResourceSyncs: []PlatformSiteResourceSyncSnapshot{
			{
				ResourceType:   model.PlatformSiteResourceModels,
				Status:         model.PlatformSiteResourceStatusStale,
				SourceEndpoint: "/api/user/models",
			},
			{
				ResourceType:   model.PlatformSiteResourceModels,
				Status:         model.PlatformSiteResourceStatusSuccess,
				SourceEndpoint: "/v1/models",
			},
		}}
		normalizePlatformSiteResourceSyncs(&snapshot)
		require.Len(t, snapshot.ResourceSyncs, 1)
		assert.Equal(t, model.PlatformSiteResourceStatusSuccess, snapshot.ResourceSyncs[0].Status)
	})
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
		AuthType:     model.UpstreamAuthAccessToken,
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
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
		case "/api/v1/groups/available":
			_, _ = writer.Write([]byte(`{"data":[{"id":"default","name":"default","rate_multiplier":0.5}]}`))
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

func TestSub2APIAdapterAllowsNegativeBalanceAndStructuredQuotaAliases(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"sub2api-session"}}`))
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":7,"balance":-1.25,"quota":{"window":"monthly"},"used_quota":{"total":3}}}`))
		case "/api/v1/user/profile":
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"code":404}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"total_actual_cost":2}}`))
		case "/api/v1/groups/available", "/api/v1/groups/rates":
			_, _ = writer.Write([]byte(`{"code":0,"data":[]}`))
		case "/api/v1/keys":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[{"id":"key-1","name":"primary","key":"sk-one","models":["gpt-4o"],"quota":{"limit":10},"quota_used":1}],"total":1,"page_size":100}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator@example.com",
		Password: "synthetic-password",
	})
	require.NoError(t, err)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.True(t, snapshot.BalanceSet)
	assert.Equal(t, -1.25, snapshot.Balance)
	assert.True(t, snapshot.UsedQuotaSet)
	assert.Equal(t, int64(1000000), snapshot.UsedQuota)
	require.Len(t, snapshot.Keys, 1)
	assert.Nil(t, snapshot.Keys[0].RemainQuota)
	assert.Equal(t, int64(500000), snapshot.Keys[0].UsedQuota)
	assert.True(t, snapshot.Keys[0].UsedQuotaSet)
}

func TestSub2APIAdapterRejectsNegativeUsedQuota(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"sub2api-session"}}`))
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":1,"used_quota":-1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator@example.com",
		Password: "synthetic-password",
	})
	require.NoError(t, err)

	_, err = adapter.FetchSnapshot(context.Background(), session)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteResponse)
	assert.Contains(t, err.Error(), "Sub2API 当前用户用量字段")
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
				assert.Contains(t, string(body), `"email":"operator@example.com"`)
				assert.NotContains(t, string(body), `"username"`)
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
	assert.Equal(t, 1, loginAttempts)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	require.Len(t, snapshot.Keys, 1)
	assert.Nil(t, snapshot.Keys[0].RemainQuota)
	assert.Equal(t, []string{"gpt-5.5"}, snapshot.Keys[0].Models)
	assert.True(t, snapshot.Keys[0].ModelsSynced)
	assert.Equal(t, 1, modelRequests)
}

func TestSub2APIAdapterDoesNotUseGroupModelsWhenRelayRejectsKey(t *testing.T) {
	relayModelRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"sub2api-session"}}`))
		case "/api/v1/auth/me":
			assert.Equal(t, "Bearer sub2api-session", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":7,"balance":1}}`))
		case "/api/v1/user/profile", "/api/v1/usage/dashboard/stats", "/api/v1/usage/stats":
			http.NotFound(writer, request)
		case "/api/v1/groups/available", "/api/v1/groups/rates":
			_, _ = writer.Write([]byte(`{"code":0,"data":[]}`))
		case "/api/v1/keys":
			assert.Equal(t, "Bearer sub2api-session", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[{"id":"key-1","name":"primary","key":"sk-upstream","group":{"id":20,"name":"Grok-heavy"}}],"total":1,"page_size":100}}`))
		case "/v1/models":
			relayModelRequests++
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"error":{"code":"ACCESS_DENIED"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator@example.com",
		Password: "synthetic-password",
	})
	require.NoError(t, err)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	require.Len(t, snapshot.Keys, 1)
	assert.Empty(t, snapshot.Keys[0].Models)
	assert.False(t, snapshot.Keys[0].ModelsSynced)
	assert.Equal(t, upstreamKeySyncErrorModelsUnavailable, snapshot.Keys[0].SyncError)
	assert.Equal(t, 1, relayModelRequests)
	assert.Empty(t, snapshot.Models)
}

func TestSub2APILoginTriesEmailThenUsernameBody(t *testing.T) {
	loginBodies := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login":
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			loginBodies = append(loginBodies, string(body))
			if len(loginBodies) == 1 {
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = writer.Write([]byte(`{"code":401,"message":"invalid credentials"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"sub2api-session"}}`))
		case "/api/v1/auth/me":
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
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	require.Len(t, loginBodies, 2)
	assert.Contains(t, loginBodies[0], `"email":"operator@example.com"`)
	assert.NotContains(t, loginBodies[0], `"username"`)
	assert.Contains(t, loginBodies[1], `"username":"operator@example.com"`)
	assert.NotContains(t, loginBodies[1], `"email"`)
	assert.Equal(t, "Bearer sub2api-session", session.Headers.Get("Authorization"))
}

func TestSub2APILoginDoesNotRetryBusinessCredentialErrorWithoutHTTP401(t *testing.T) {
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/api/v1/auth/login" {
			loginRequests++
			_, _ = writer.Write([]byte(`{"code":401,"message":"invalid credentials"}`))
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == "/auth/login" {
			t.Fatalf("HTTP 200 业务凭据错误不应回退旧登录路由")
		}
		http.NotFound(writer, request)
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
	assert.Equal(t, 1, loginRequests)
	assert.ErrorIs(t, err, ErrPlatformSiteCredentials)
}

func TestSub2APILoginDoesNotFallbackRouteAfterPrimaryCredentialError(t *testing.T) {
	primaryRequests := 0
	legacyRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/auth/login":
			primaryRequests++
			if primaryRequests == 1 {
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = writer.Write([]byte(`{"code":401,"message":"invalid credentials"}`))
				return
			}
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"code":404,"message":"route not found"}`))
		case "/auth/login":
			legacyRequests++
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"legacy-session"}}`))
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
	assert.Equal(t, 2, primaryRequests)
	assert.Zero(t, legacyRequests)
}

func TestSub2APILoginStopsOnSecurityVerification(t *testing.T) {
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Path == "/api/v1/settings/public" {
			http.NotFound(writer, request)
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == "/api/v1/auth/login" {
			loginRequests++
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"code":403,"message":"verification required"}`))
			return
		}
		if request.URL.Path == "" || request.URL.Path == "/" {
			http.NotFound(writer, request)
			return
		}
		t.Fatalf("安全验证失败时不应继续请求 %s", request.URL.Path)
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
	assert.Equal(t, 1, loginRequests)
	assert.ErrorIs(t, err, ErrPlatformSiteSecurity)
	assert.NotContains(t, SafePlatformSiteError(err), "synthetic-password")
}

func TestSub2APIUsageReadsDashboardBeforeStatsFallback(t *testing.T) {
	requests := make([]string, 0, 16)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		requests = append(requests, request.URL.Path)
		switch request.URL.Path {
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":1}}`))
		case "/api/v1/user/profile":
			http.NotFound(writer, request)
		case "/api/v1/groups/available":
			_, _ = writer.Write([]byte(`{"code":0,"data":[]}`))
		case "/api/v1/groups/rates":
			_, _ = writer.Write([]byte(`{"code":0,"data":{}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = writer.Write([]byte(`{"code":0,"data":{}}`))
		case "/api/v1/usage/stats":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"total_actual_cost":3}}`))
		case "/api/v1/keys":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[{"id":42,"key":"sk-sub2api","models":["gpt-4o"]}],"total":1,"page_size":100}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType:    model.UpstreamAuthAccessToken,
		AccessToken: "session-token",
	})
	require.NoError(t, err)
	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, int64(1500000), snapshot.UsedQuota)
	assert.True(t, snapshot.UsedQuotaSet)

	lastIndex := func(path string) int {
		for index := len(requests) - 1; index >= 0; index-- {
			if requests[index] == path {
				return index
			}
		}
		return -1
	}
	assert.Less(t, lastIndex("/api/v1/groups/available"), lastIndex("/api/v1/groups/rates"))
	assert.Less(t, lastIndex("/api/v1/groups/rates"), lastIndex("/api/v1/usage/dashboard/stats"))
	assert.Less(t, lastIndex("/api/v1/usage/dashboard/stats"), lastIndex("/api/v1/usage/stats"))
	assert.Less(t, lastIndex("/api/v1/usage/stats"), lastIndex("/api/v1/keys"))
}

func TestSub2APIGroupsFailureStopsKeySynchronizationAndKeepsSnapshotData(t *testing.T) {
	keyRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":7,"balance":1}}`))
		case "/api/v1/user/profile":
			http.NotFound(writer, request)
		case "/api/v1/groups/available":
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"code":403,"message":"permission denied"}`))
		case "/api/v1/keys":
			keyRequests++
			t.Fatalf("分组核心资源失败后不应读取密钥")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType:    model.UpstreamAuthAccessToken,
		AccessToken: "session-token",
	})
	require.NoError(t, err)
	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteResource)
	assert.NotErrorIs(t, err, ErrPlatformSiteCredentials)
	assert.Equal(t, 0, keyRequests)
	assert.NotNil(t, snapshot.Identity)
	assert.False(t, snapshot.KeysComplete)
	require.Len(t, snapshot.ResourceSyncs, 2)
	assert.Equal(t, model.PlatformSiteResourceStatusFailed, snapshot.ResourceSyncs[1].Status)
}

func TestSub2APIKeyPaginationUsesThousandPageLimitAndListModels(t *testing.T) {
	requestedPages := 0
	modelRequests := 0
	detailRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/keys":
			page := request.URL.Query().Get("page")
			if page == "1001" {
				t.Fatalf("Sub2API 普通密钥分页不应请求第 1001 页")
			}
			requestedPages++
			assert.Equal(t, "100", request.URL.Query().Get("page_size"))
			_, _ = fmt.Fprintf(
				writer,
				`{"code":0,"data":{"items":[{"id":"key-%s","key":"sk-%s","model_limits":"gpt-4o"}],"total":1000,"page_size":1}}`,
				page,
				page,
			)
		case "/v1/models":
			modelRequests++
		default:
			if strings.HasPrefix(request.URL.Path, "/api/v1/keys/") {
				detailRequests++
			}
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	keys, err := fetchSub2APIKeys(context.Background(), session, nil)
	require.NoError(t, err)
	assert.Equal(t, 1000, requestedPages)
	assert.Equal(t, 0, detailRequests)
	assert.Equal(t, 0, modelRequests)
	require.Len(t, keys, 1000)
	assert.Equal(t, "key-1000", keys[len(keys)-1].ExternalID)
	assert.Equal(t, "sk-1000", keys[len(keys)-1].Secret)
	assert.Equal(t, []string{"gpt-4o"}, keys[len(keys)-1].Models)
}

func TestSub2APIAdminKeyPaginationUsesThousandPageLimitAndSkipsDataForCompleteKeys(t *testing.T) {
	requestedPages := 0
	dataRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/accounts":
			page := request.URL.Query().Get("page")
			if page == "1001" {
				t.Fatalf("Sub2API 管理密钥分页不应请求第 1001 页")
			}
			requestedPages++
			assert.Equal(t, "100", request.URL.Query().Get("page_size"))
			assert.Equal(t, "apikey", request.URL.Query().Get("type"))
			_, _ = fmt.Fprintf(
				writer,
				`{"code":0,"data":{"accounts":[{"id":"account-%s","name":"managed","key":"sk-%s","models":["gpt-4o"]}],"total":1000,"page_size":1}}`,
				page,
				page,
			)
		case "/api/v1/admin/accounts/data":
			dataRequests++
			t.Fatalf("列表已返回完整 Admin Key 时不应请求 accounts/data")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	session.Headers.Set("x-api-key", "admin-secret")
	keys, err := fetchSub2APIAdminKeys(context.Background(), session, nil)
	require.NoError(t, err)
	assert.Equal(t, 1000, requestedPages)
	assert.Equal(t, 0, dataRequests)
	require.Len(t, keys, 1000)
	assert.Equal(t, "account-1000", keys[len(keys)-1].ExternalID)
	assert.Equal(t, "sk-1000", keys[len(keys)-1].Secret)
}

func TestSub2APIKeyPaginationFailureReturnsPartialSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":1}}`))
		case "/api/v1/user/profile":
			http.NotFound(writer, request)
		case "/api/v1/groups/available":
			_, _ = writer.Write([]byte(`{"code":0,"data":[]}`))
		case "/api/v1/groups/rates":
			_, _ = writer.Write([]byte(`{"code":0,"data":{}}`))
		case "/api/v1/usage/dashboard/stats", "/api/v1/usage/stats":
			http.NotFound(writer, request)
		case "/api/v1/keys":
			if request.URL.Query().Get("page") == "2" {
				writer.WriteHeader(http.StatusBadGateway)
				_, _ = writer.Write([]byte(`{"code":502,"message":"temporary upstream failure"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[{"id":"key-1","key":"sk-one","models":["gpt-4o"]}],"total":2,"page_size":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType:    model.UpstreamAuthAccessToken,
		AccessToken: "session-token",
	})
	require.NoError(t, err)
	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteResource)
	assert.False(t, snapshot.KeysComplete)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "key-1", snapshot.Keys[0].ExternalID)
	assert.NotEmpty(t, snapshot.ResourceSyncs)
}

func TestSub2APIAdminKeyStepUpRemainsIndependentFromAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":1}}`))
		case "/api/v1/user/profile":
			http.NotFound(writer, request)
		case "/api/v1/groups/available":
			_, _ = writer.Write([]byte(`{"code":0,"data":[]}`))
		case "/api/v1/groups/rates":
			_, _ = writer.Write([]byte(`{"code":0,"data":{}}`))
		case "/api/v1/usage/dashboard/stats", "/api/v1/usage/stats":
			http.NotFound(writer, request)
		case "/api/v1/admin/accounts":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"accounts":[{"id":"account-1","name":"managed"}],"total":1,"page_size":100}}`))
		case "/api/v1/admin/accounts/data":
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"code":403,"message":"verification required"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthAdminKey,
		AdminKey: "admin-secret",
		UserID:   "",
	})
	require.NoError(t, err)
	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteResource)
	assert.ErrorIs(t, err, ErrPlatformSiteSecurity)
	assert.Equal(t, model.PlatformSiteAuthStatusSecureVerificationRequired, snapshot.AuthStatus)
	var keyResource *PlatformSiteResourceSyncSnapshot
	for index := range snapshot.ResourceSyncs {
		if snapshot.ResourceSyncs[index].ResourceType == model.PlatformSiteResourceKeys {
			keyResource = &snapshot.ResourceSyncs[index]
			break
		}
	}
	require.NotNil(t, keyResource)
	assert.Equal(t, model.PlatformSiteResourceStatusSecureVerificationRequired, keyResource.Status)
	assert.True(t, keyResource.RequiresSecurityVerification)
}

func TestSub2APIAdapterResolvesRelativeRelayURLFromPageConfig(t *testing.T) {
	client := &http.Client{
		Transport: platformSiteRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch {
			case request.Method == http.MethodGet && (request.URL.Path == "" || request.URL.Path == "/"):
				response := &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader(`<script>window.__APP_CONFIG__={"api_base_url":"/v1"}</script>`)),
				}
				response.Request = request.Clone(request.Context())
				response.Request.URL, _ = url.Parse("https://example.com/zh/home")
				return response, nil
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
		"https://management.example.com/",
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/v1", session.ModelBaseURL)
}

func TestSub2APIAdapterAcceptsRelayURLDeclaredByRedirectedPage(t *testing.T) {
	modelRequests := 0
	client := &http.Client{
		Transport: platformSiteRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch {
			case request.URL.Host == "hhw1231.com" &&
				request.Method == http.MethodGet &&
				(request.URL.Path == "" || request.URL.Path == "/"):
				response := &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
					Body: io.NopCloser(strings.NewReader(
						`<script>window.__APP_CONFIG__={"api_base_url":"https://127.0.0.1:9443/v1"}</script>`,
					)),
				}
				response.Request = request.Clone(request.Context())
				response.Request.URL, _ = url.Parse("https://127.0.0.1:9443/zh/home")
				return response, nil
			case request.URL.Host == "127.0.0.1:9443" &&
				request.Method == http.MethodPost &&
				request.URL.Path == "/api/v1/auth/login":
				body, readErr := io.ReadAll(request.Body)
				require.NoError(t, readErr)
				var loginPayload map[string]any
				require.NoError(t, common.Unmarshal(body, &loginPayload))
				assert.Equal(t, "operator@example.com", loginPayload["email"])
				_, hasUsername := loginPayload["username"]
				assert.False(t, hasUsername)
				return platformSiteJSONResponse(
					http.StatusOK,
					`{"code":0,"data":{"access_token":"sub2api-session"}}`,
				), nil
			case request.URL.Host == "127.0.0.1:9443" &&
				request.Method == http.MethodGet &&
				request.URL.Path == "/api/v1/auth/me":
				return platformSiteJSONResponse(
					http.StatusOK,
					`{"code":0,"data":{"id":9,"username":"operator","balance":3}}`,
				), nil
			case request.URL.Host == "127.0.0.1:9443" &&
				request.Method == http.MethodGet &&
				request.URL.Path == "/api/v1/user/profile":
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
			case request.URL.Host == "127.0.0.1:9443" &&
				request.Method == http.MethodGet &&
				request.URL.Path == "/api/v1/groups/available":
				return platformSiteJSONResponse(http.StatusOK, `{"code":0,"data":[]}`), nil
			case request.URL.Host == "127.0.0.1:9443" &&
				request.Method == http.MethodGet &&
				request.URL.Path == "/api/v1/groups/rates":
				return platformSiteJSONResponse(http.StatusOK, `{"code":0,"data":{}}`), nil
			case request.URL.Host == "127.0.0.1:9443" &&
				request.Method == http.MethodGet &&
				(request.URL.Path == "/api/v1/usage/dashboard/stats" ||
					request.URL.Path == "/api/v1/usage/stats"):
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
			case request.URL.Host == "127.0.0.1:9443" &&
				request.Method == http.MethodGet &&
				request.URL.Path == "/api/v1/keys":
				return platformSiteJSONResponse(
					http.StatusOK,
					`{"code":0,"data":{"items":[{"id":"key-1","name":"primary","key":"sk-upstream"}],"total":1,"page_size":100}}`,
				), nil
			case request.URL.Host == "127.0.0.1:9443" &&
				request.Method == http.MethodGet &&
				request.URL.Path == "/v1/models":
				modelRequests++
				assert.Equal(t, "Bearer sk-upstream", request.Header.Get("Authorization"))
				assert.Equal(t, "sk-upstream", request.Header.Get("x-api-key"))
				assert.Empty(t, request.Header.Get("Cookie"))
				assert.Empty(t, request.Header.Get("Origin"))
				assert.Empty(t, request.Header.Get("Referer"))
				assert.Empty(t, request.Header.Get("X-Requested-With"))
				assert.Empty(t, request.Header.Get("X-Auth-Session"))
				return platformSiteJSONResponse(
					http.StatusOK,
					`{"data":[{"id":"gpt-5.5"}]}`,
				), nil
			default:
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
			}
		}),
	}

	adapter := NewSub2APIAdapter(client)
	session, err := adapter.Authenticate(
		context.Background(),
		"https://hhw1231.com",
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "https://127.0.0.1:9443/v1", session.ModelBaseURL)
	assert.Equal(t, "https://hhw1231.com", session.ManagementBaseURL)
	assert.Equal(t, "https://127.0.0.1:9443", session.BaseURL)
	session.Headers.Set("Cookie", "management=session")
	session.Headers.Set("Origin", "https://hhw1231.com")
	session.Headers.Set("Referer", "https://hhw1231.com/login")
	session.Headers.Set("X-Requested-With", "XMLHttpRequest")
	session.Headers.Set("X-Auth-Session", "management-session")
	jar, jarErr := cookiejar.New(nil)
	require.NoError(t, jarErr)
	relayURL, parseErr := url.Parse(session.ModelBaseURL)
	require.NoError(t, parseErr)
	jar.SetCookies(relayURL, []*http.Cookie{{
		Name:  "management_cookie",
		Value: "session",
	}})
	session.Client.Jar = jar

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	require.Len(t, snapshot.Keys, 1)
	assert.True(t, snapshot.Keys[0].ModelsSynced)
	assert.Equal(t, []string{"gpt-5.5"}, snapshot.Keys[0].Models)
	assert.Equal(t, 1, modelRequests)
}

func TestSub2APIAdapterRejectsUnverifiedRelayURLFromRedirectedPage(t *testing.T) {
	client := &http.Client{
		Transport: platformSiteRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch {
			case request.Method == http.MethodGet &&
				request.URL.Host == "hhw1231.com" &&
				(request.URL.Path == "" || request.URL.Path == "/"):
				response := &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body: io.NopCloser(strings.NewReader(
						`<script>window.__APP_CONFIG__={"api_base_url":"https://192.0.2.1/v1"}</script>`,
					)),
				}
				response.Request = request.Clone(request.Context())
				response.Request.URL, _ = url.Parse("https://example.com/zh/home")
				return response, nil
			case request.Method == http.MethodPost &&
				request.URL.Host == "hhw1231.com" &&
				request.URL.Path == "/api/v1/auth/login":
				return platformSiteJSONResponse(
					http.StatusOK,
					`{"code":0,"data":{"access_token":"sub2api-session"}}`,
				), nil
			case request.Method == http.MethodGet &&
				request.URL.Host == "hhw1231.com" &&
				request.URL.Path == "/api/v1/auth/me":
				return platformSiteJSONResponse(
					http.StatusOK,
					`{"code":0,"data":{"id":9,"username":"operator"}}`,
				), nil
			default:
				return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
			}
		}),
	}

	session, err := NewSub2APIAdapter(client).Authenticate(
		context.Background(),
		"https://hhw1231.com",
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	assert.Empty(t, session.ModelBaseURL)
	assert.Equal(t, "https://hhw1231.com", session.ManagementBaseURL)
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

func TestSub2APIDirectAPIOriginRelationRequiresMatchingSchemeAndPort(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  bool
	}{
		{
			name:  "same scheme and default port",
			left:  "https://aiapipay.com",
			right: "https://api.aiapipay.com/v1",
			want:  true,
		},
		{
			name:  "scheme differs",
			left:  "https://aiapipay.com",
			right: "http://api.aiapipay.com/v1",
		},
		{
			name:  "port differs",
			left:  "https://aiapipay.com:8443",
			right: "https://api.aiapipay.com/v1",
		},
		{
			name:  "unrelated subdomain",
			left:  "https://aiapipay.com",
			right: "https://relay.aiapipay.com/v1",
		},
		{
			name:  "different registrable domain",
			left:  "https://aiapipay.com",
			right: "https://api.other.example/v1",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, sub2APIDirectAPIOriginRelation(
				testCase.left,
				testCase.right,
			))
		})
	}
}

func TestSub2APILoginUsesStrictEmailBodyWithoutPublicSettings(t *testing.T) {
	settingsRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/settings/public":
			settingsRequests++
			t.Fatalf("普通登录成功时不应读取公开条款设置")
		case "/api/v1/auth/login":
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			var loginPayload map[string]any
			require.NoError(t, common.Unmarshal(body, &loginPayload))
			assert.Equal(t, map[string]any{
				"email":    "operator@example.com",
				"password": "synthetic-password",
			}, loginPayload)
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"sub2api-session"}}`))
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":9,"username":"operator"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

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
	assert.Equal(t, "Bearer sub2api-session", session.Headers.Get("Authorization"))
	assert.Zero(t, settingsRequests)
}

func TestSub2APILoginRouteFallbackOnlyOnMissingRoute(t *testing.T) {
	tests := []struct {
		name              string
		primaryStatus     int
		wantLegacyRoute   bool
		wantLoginRequests int
	}{
		{
			name:              "not found",
			primaryStatus:     http.StatusNotFound,
			wantLegacyRoute:   true,
			wantLoginRequests: 1,
		},
		{
			name:              "method not allowed",
			primaryStatus:     http.StatusMethodNotAllowed,
			wantLegacyRoute:   true,
			wantLoginRequests: 1,
		},
		{
			name:              "invalid request",
			primaryStatus:     http.StatusBadRequest,
			wantLegacyRoute:   false,
			wantLoginRequests: 1,
		},
		{
			name:              "unauthorized",
			primaryStatus:     http.StatusUnauthorized,
			wantLegacyRoute:   false,
			wantLoginRequests: 2,
		},
		{
			name:              "forbidden",
			primaryStatus:     http.StatusForbidden,
			wantLegacyRoute:   false,
			wantLoginRequests: 1,
		},
		{
			name:              "rate limited",
			primaryStatus:     http.StatusTooManyRequests,
			wantLegacyRoute:   false,
			wantLoginRequests: 1,
		},
		{
			name:              "server error",
			primaryStatus:     http.StatusInternalServerError,
			wantLegacyRoute:   false,
			wantLoginRequests: 1,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			primaryRequests := 0
			legacyRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				request *http.Request,
			) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/api/v1/auth/login":
					primaryRequests++
					writer.WriteHeader(testCase.primaryStatus)
					_, _ = writer.Write([]byte(fmt.Sprintf(
						`{"code":%d,"reason":"INVALID_REQUEST"}`,
						testCase.primaryStatus,
					)))
				case "/auth/login":
					legacyRequests++
					_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"legacy-session"}}`))
				case "/api/v1/auth/me":
					_, _ = writer.Write([]byte(`{"code":0,"data":{"id":9,"username":"operator"}}`))
				default:
					http.NotFound(writer, request)
				}
			}))

			adapter := NewSub2APIAdapter(server.Client())
			session, err := adapter.Authenticate(
				context.Background(),
				server.URL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: "operator@example.com",
					Password: "synthetic-password",
				},
			)
			if testCase.wantLegacyRoute {
				require.NoError(t, err)
				require.NotNil(t, session)
				assert.Equal(t, "Bearer legacy-session", session.Headers.Get("Authorization"))
			} else {
				require.Error(t, err)
			}
			assert.Equal(t, testCase.wantLoginRequests, primaryRequests)
			assert.Equal(t, testCase.wantLegacyRoute, legacyRequests == 1)
			server.Close()
		})
	}
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

func TestSub2APILoginClassifiesHTMLTurnstileAsInteractiveVerification(t *testing.T) {
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/api/v1/auth/login" {
			loginRequests++
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(
				`<html><title>Turnstile challenge</title><body>verify you are human</body></html>`,
			))
			return
		}
		http.NotFound(writer, request)
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
	assert.Equal(t, 1, loginRequests)
	assert.ErrorIs(t, err, ErrSub2APILoginInteractive)
	assert.ErrorIs(t, err, ErrPlatformSiteSecurity)
	assert.Equal(t, platformSiteErrorCategoryInteractive, platformSiteErrorCategoryOf(err))
	message := SafePlatformSiteError(err)
	assert.Contains(t, message, "交互验证")
	assert.Contains(t, message, "HTTP 400")
	assert.NotContains(t, message, "Turnstile challenge")
	assert.NotContains(t, message, "synthetic-password")
	assert.NotContains(t, message, "Access Token")
	assert.NotContains(t, message, "Refresh Token")
	assert.NotContains(t, message, "Cookie")
}

func TestPlatformSiteRequestClassifiesHTMLForbiddenAsWAF(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(fmt.Sprintf("HTTP-%d", status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "text/html; charset=utf-8")
				writer.WriteHeader(status)
				_, _ = writer.Write([]byte("<html><body>request blocked</body></html>"))
			}))
			t.Cleanup(server.Close)

			session, err := newPlatformSiteSession(server.URL, nil)
			require.NoError(t, err)

			_, err = platformSiteRequest(
				context.Background(),
				session,
				http.MethodGet,
				"/api/user/self",
				nil,
				nil,
			)
			require.Error(t, err)
			assert.Equal(t, platformSiteErrorCategoryWAF, platformSiteErrorCategoryOf(err))
			assert.ErrorIs(t, err, ErrPlatformSiteSecurity)
			assert.NotContains(t, SafePlatformSiteError(err), "账号或密码错误")
			assert.NotContains(t, SafePlatformSiteError(err), "响应格式错误")
		})
	}
}

func TestSub2APILoginUsesUsernameBody(t *testing.T) {
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/api/v1/auth/login" {
			loginRequests++
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"username":"operator"`)
			assert.NotContains(t, string(body), `"email"`)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"username-session"}}`))
			return
		}
		if request.Method == http.MethodGet && request.URL.Path == "/api/v1/auth/me" {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":1}}`))
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(server.Close)

	session, err := NewSub2APIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, loginRequests)
	require.NotNil(t, session)
	assert.Equal(t, "Bearer username-session", session.Headers.Get("Authorization"))
}

func TestSub2APILoginAgreementRevisionIsSentOnlyAfterExplicitRequirement(t *testing.T) {
	loginBodies := make([]map[string]any, 0, 2)
	settingsRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/settings/public":
			settingsRequests++
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"login_agreement_enabled":true,"login_agreement_revision":"terms-rev-2026-10-01"}}`,
			))
		case "/api/v1/auth/login":
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			var loginPayload map[string]any
			require.NoError(t, common.Unmarshal(body, &loginPayload))
			loginBodies = append(loginBodies, loginPayload)
			if len(loginBodies) == 1 {
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = writer.Write([]byte(`{"code":400,"message":"agreement required"}`))
				return
			}
			assert.Equal(t, "operator@example.com", loginPayload["email"])
			assert.Equal(t, "terms-rev-2026-10-01", loginPayload["agreed_revision"])
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"sub2api-session"}}`))
		case "/api/v1/auth/me":
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
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Len(t, loginBodies, 2)
	assert.Equal(t, 1, settingsRequests)
	assert.Equal(t, "operator@example.com", loginBodies[0]["email"])
	assert.Equal(t, "synthetic-password", loginBodies[0]["password"])
	_, firstHasUsername := loginBodies[0]["username"]
	assert.False(t, firstHasUsername)
	_, firstHasAgreement := loginBodies[0]["agreed_revision"]
	assert.False(t, firstHasAgreement)
	assert.Equal(t, "operator@example.com", loginBodies[1]["email"])
	assert.Equal(t, "synthetic-password", loginBodies[1]["password"])
	assert.Equal(t, "terms-rev-2026-10-01", loginBodies[1]["agreed_revision"])
	_, secondHasUsername := loginBodies[1]["username"]
	assert.False(t, secondHasUsername)
	_, secondHasNotInCNConfirmed := loginBodies[1]["not_in_cn_confirmed"]
	assert.False(t, secondHasNotInCNConfirmed)
}

func TestSub2APILoginAgreementRevisionFailureFallsBackToLegacyLogin(t *testing.T) {
	testCases := []struct {
		name           string
		settingsStatus int
		settingsBody   string
		networkError   bool
	}{
		{
			name:           "disabled",
			settingsStatus: http.StatusOK,
			settingsBody:   `{"code":0,"data":{"login_agreement_enabled":false,"login_agreement_revision":"terms-rev"}}`,
		},
		{
			name:           "revision missing",
			settingsStatus: http.StatusOK,
			settingsBody:   `{"code":0,"data":{"login_agreement_enabled":true}}`,
		},
		{
			name:           "revision empty",
			settingsStatus: http.StatusOK,
			settingsBody:   `{"code":0,"data":{"login_agreement_enabled":true,"login_agreement_revision":"  "}}`,
		},
		{
			name:           "not found",
			settingsStatus: http.StatusNotFound,
			settingsBody:   `{"code":404,"message":"route not found"}`,
		},
		{
			name:           "invalid response",
			settingsStatus: http.StatusOK,
			settingsBody:   `not-json`,
		},
		{
			name:         "network error",
			networkError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			loginRequests := 0
			handleRequest := func(request *http.Request) (*http.Response, error) {
				switch request.URL.Path {
				case "/":
					return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
				case "/api/v1/settings/public":
					if testCase.networkError {
						return nil, errors.New("settings network unavailable")
					}
					return platformSiteJSONResponse(
						testCase.settingsStatus,
						testCase.settingsBody,
					), nil
				case "/api/v1/auth/login":
					loginRequests++
					body, readErr := io.ReadAll(request.Body)
					require.NoError(t, readErr)
					var loginPayload map[string]any
					require.NoError(t, common.Unmarshal(body, &loginPayload))
					_, hasAgreedRevision := loginPayload["agreed_revision"]
					assert.False(t, hasAgreedRevision)
					return platformSiteJSONResponse(
						http.StatusOK,
						`{"code":0,"data":{"access_token":"legacy-session"}}`,
					), nil
				case "/api/v1/auth/me":
					return platformSiteJSONResponse(
						http.StatusOK,
						`{"code":0,"data":{"balance":1}}`,
					), nil
				default:
					return platformSiteJSONResponse(http.StatusNotFound, `{"code":404}`), nil
				}
			}

			var (
				client  *http.Client
				baseURL string
				server  *httptest.Server
			)
			if testCase.networkError {
				client = &http.Client{Transport: platformSiteRoundTripFunc(handleRequest)}
				baseURL = "https://example.com"
			} else {
				server = httptest.NewServer(http.HandlerFunc(func(
					writer http.ResponseWriter,
					request *http.Request,
				) {
					response, responseErr := handleRequest(request)
					require.NoError(t, responseErr)
					writer.Header().Set("Content-Type", "application/json")
					writer.WriteHeader(response.StatusCode)
					data, readErr := io.ReadAll(response.Body)
					require.NoError(t, readErr)
					_, _ = writer.Write(data)
					_ = response.Body.Close()
				}))
				t.Cleanup(server.Close)
				client = server.Client()
				baseURL = server.URL
			}

			_, err := NewSub2APIAdapter(client).Authenticate(
				context.Background(),
				baseURL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: "operator@example.com",
					Password: "synthetic-password",
				},
			)
			require.NoError(t, err)
			assert.Equal(t, 1, loginRequests)
		})
	}
}

func TestSub2APILoginAgreementStopsCredentialFallbackAndIsNotCredentialError(t *testing.T) {
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/settings/public":
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"login_agreement_enabled":true,"login_agreement_revision":"terms-rev"}}`,
			))
		case "/api/v1/auth/login":
			loginRequests++
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(
				`{"code":400,"message":"agreement required","detail":"upstream-secret-response"}`,
			))
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
	assert.Equal(t, 2, loginRequests)
	assert.ErrorIs(t, err, ErrSub2APILoginAgreement)
	assert.NotErrorIs(t, err, ErrPlatformSiteCredentials)
	assert.NotErrorIs(t, err, ErrPlatformSiteSecurity)
	message := SafePlatformSiteError(err)
	assert.Contains(t, message, "已尝试提交当前服务条款版本")
	assert.NotContains(t, message, "synthetic-password")
	assert.NotContains(t, message, "upstream-secret-response")
	assert.NotContains(t, message, "Cookie")
	assert.NotContains(t, message, "Access Token")
	assert.NotContains(t, message, "Refresh Token")
}

func TestSub2APILoginAgreementMarkerPrecedesSecurityVerification(t *testing.T) {
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/settings/public":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"login_agreement_enabled":true,"login_agreement_revision":"terms-rev"}}`,
			))
		case "/api/v1/auth/login":
			loginRequests++
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = writer.Write([]byte(
				`<html><body>service terms agreement required</body></html>`,
			))
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
	assert.Equal(t, 2, loginRequests)
	assert.ErrorIs(t, err, ErrSub2APILoginAgreement)
	assert.NotErrorIs(t, err, ErrPlatformSiteSecurity)
}

func TestNewAPILoginDoesNotReadSub2APIPublicSettingsOrSendAgreementRevision(t *testing.T) {
	settingsRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/settings/public":
			settingsRequests++
			t.Fatalf("New API 登录不应请求 Sub2API 公开设置")
		case "/api/user/login":
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			var loginPayload map[string]any
			require.NoError(t, common.Unmarshal(body, &loginPayload))
			_, hasAgreedRevision := loginPayload["agreed_revision"]
			assert.False(t, hasAgreedRevision)
			_, _ = writer.Write([]byte(
				`{"success":true,"data":{"token":"new-api-session"}}`,
			))
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := NewNewAPIAdapter(server.Client()).Authenticate(
		context.Background(),
		server.URL,
		model.PlatformSiteCredential{
			AuthType: model.UpstreamAuthPassword,
			Username: "operator@example.com",
			Password: "synthetic-password",
		},
	)
	require.NoError(t, err)
	assert.Zero(t, settingsRequests)
}

func TestNewAPILoginDistinguishesHTTP200InteractiveAndCredentialFailures(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantSecurity   bool
		wantCredential bool
		wantMessage    string
	}{
		{
			name:         "turnstile",
			body:         `{"success":false,"message":"turnstile verification required"}`,
			wantSecurity: true,
			wantMessage:  "安全验证",
		},
		{
			name:           "credentials",
			body:           `{"success":false,"message":"invalid username or password"}`,
			wantCredential: true,
			wantMessage:    "账号或密码错误",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			loginRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet && request.URL.Path == newAPIPasswordEncryptionPath {
					http.NotFound(writer, request)
					return
				}
				if request.Method == http.MethodPost && request.URL.Path == "/api/user/login" {
					loginRequests++
					writer.Header().Set("Content-Type", "application/json")
					_, _ = writer.Write([]byte(testCase.body))
					return
				}
				t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
			}))
			defer server.Close()

			_, err := NewNewAPIAdapter(server.Client()).Authenticate(
				context.Background(),
				server.URL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: "operator",
					Password: "synthetic-password",
				},
			)
			require.Error(t, err)
			assert.Equal(t, 1, loginRequests)
			assert.Equal(t, testCase.wantSecurity, errors.Is(err, ErrPlatformSiteSecurity))
			assert.Equal(t, testCase.wantCredential, errors.Is(err, ErrPlatformSiteCredentials))
			message := SafePlatformSiteError(err)
			assert.Contains(t, message, testCase.wantMessage)
			assert.NotContains(t, message, "synthetic-password")
		})
	}
}

func TestPlatformSiteCompatibleCurrentUserRoutesOnlyFallbackOn404Or405(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(fmt.Sprintf("fallback-on-%d", status), func(t *testing.T) {
			fallbackRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/api/user/login":
					_, _ = writer.Write([]byte(`{"success":true,"data":{"token":"session"}}`))
				case "/api/user/self":
					writer.WriteHeader(status)
					_, _ = writer.Write([]byte(fmt.Sprintf(`{"code":%d}`, status)))
				case "/api/user/me":
					fallbackRequests++
					_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":1}}`))
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			_, err := NewNewAPIAdapter(server.Client()).Authenticate(
				context.Background(),
				server.URL,
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: "operator",
					Password: "synthetic-password",
				},
			)
			require.NoError(t, err)
			assert.Equal(t, 1, fallbackRequests)
		})
	}

	t.Run("no-fallback-on-401", func(t *testing.T) {
		fallbackRequests := 0
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			switch request.URL.Path {
			case "/api/user/login":
				_, _ = writer.Write([]byte(`{"success":true,"data":{"token":"session"}}`))
			case "/api/user/self":
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = writer.Write([]byte(`{"code":401,"message":"invalid credentials"}`))
			case "/api/user/me":
				fallbackRequests++
				t.Fatalf("401 后不应继续尝试兼容用户接口")
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		_, err := NewNewAPIAdapter(server.Client()).Authenticate(
			context.Background(),
			server.URL,
			model.PlatformSiteCredential{
				AuthType: model.UpstreamAuthPassword,
				Username: "operator",
				Password: "synthetic-password",
			},
		)
		require.Error(t, err)
		assert.Zero(t, fallbackRequests)
		assert.ErrorIs(t, err, ErrPlatformSiteCredentials)
	})
}

func TestPlatformSiteRequestWrapsNetworkTimeoutWithoutSensitiveDetails(t *testing.T) {
	session, err := newPlatformSiteSession("https://upstream.example", nil)
	require.NoError(t, err)
	session.Client = &http.Client{
		Transport: platformSiteRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}),
	}

	_, err = platformSiteRequest(
		context.Background(),
		session,
		http.MethodGet,
		"/api/user/self",
		nil,
		nil,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteTransport)
	message := SafePlatformSiteError(err)
	assert.Contains(t, message, "保留最近成功快照")
	assert.NotContains(t, message, "Cookie")
	assert.NotContains(t, message, "synthetic-password")
}

func TestSub2APIAdapterResolvesMaskedKeyFromKeyDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":1}}`))
		case "/api/v1/groups/available":
			_, _ = writer.Write([]byte(`{"code":0,"data":[]}`))
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
	assert.Equal(t, model.PlatformSiteAuthStatusSecureVerificationRequired, snapshot.AuthStatus)
	var keyResource *PlatformSiteResourceSyncSnapshot
	for index := range snapshot.ResourceSyncs {
		if snapshot.ResourceSyncs[index].ResourceType == model.PlatformSiteResourceKeys {
			keyResource = &snapshot.ResourceSyncs[index]
			break
		}
	}
	require.NotNil(t, keyResource)
	assert.Equal(t, model.PlatformSiteResourceStatusSecureVerificationRequired, keyResource.Status)
	assert.True(t, keyResource.RequiresSecurityVerification)
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
		&model.PlatformSiteIdentity{},
		&model.PlatformSiteGroup{},
		&model.PlatformSiteEndpoint{},
		&model.PlatformSiteEndpointCapability{},
		&model.PlatformSiteResourceSync{},
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
	assert.Equal(t, model.UpstreamKeyStatusEnabled, savedOld.Status)
	assert.Empty(t, savedOld.DisabledReason)
	assert.Equal(t, "gpt-4o", savedOld.Models)
	oldCredential, err := model.DecryptPlatformSiteCredential(savedOld.SecretCiphertext)
	require.NoError(t, err)
	assert.Equal(t, "sk-old", oldCredential.AccessToken)
	var oldAbilityCount int64
	require.NoError(t, db.Model(&model.UpstreamKeyAbility{}).
		Where("upstream_key_id = ?", savedOld.ID).
		Count(&oldAbilityCount).Error)
	assert.Equal(t, int64(1), oldAbilityCount)

	var newKey model.UpstreamKey
	assert.ErrorIs(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "new-key").First(&newKey).Error, gorm.ErrRecordNotFound)
	var newAbilityCount int64
	require.NoError(t, db.Model(&model.UpstreamKeyAbility{}).
		Where("upstream_key_id = ?", 0).
		Count(&newAbilityCount).Error)
	assert.Zero(t, newAbilityCount)

	var keyResourceSync model.PlatformSiteResourceSync
	require.NoError(t, db.Where("channel_id = ? AND resource_type = ?", channel.Id, model.PlatformSiteResourceKeys).First(&keyResourceSync).Error)
	assert.Equal(t, model.PlatformSiteResourceStatusPartial, keyResourceSync.Status)
	assert.True(t, keyResourceSync.Partial)
	assert.True(t, keyResourceSync.UsingSnapshot)

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

	account.Balance = savedAccount.Balance
	account.UsedQuota = savedAccount.UsedQuota
	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		KeysComplete: false,
	}))
	require.NoError(t, db.First(&savedAccount, account.ID).Error)
	assert.Equal(t, 7.0, savedAccount.Balance)
	assert.Equal(t, int64(123456), savedAccount.UsedQuota)
	require.NoError(t, db.First(&savedChannel, channel.Id).Error)
	assert.Equal(t, 7.0, savedChannel.Balance)
	assert.Equal(t, int64(123456), savedChannel.UsedQuota)
}

func TestPersistPlatformSiteSnapshotSeparatesManagementAndRelayURLs(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.PlatformSiteIdentity{},
		&model.PlatformSiteGroup{},
		&model.PlatformSiteEndpoint{},
		&model.PlatformSiteEndpointCapability{},
		&model.PlatformSiteResourceSync{},
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

	require.NoError(t, db.Model(account).Update("platform", model.PlatformNewAPI).Error)
	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance:           9,
		UsedQuota:         34,
		UsedQuotaSet:      true,
		ManagementBaseURL: "https://new-management.example",
		RelayBaseURL:      "https://new-relay.example/v1",
	}))

	require.NoError(t, db.First(&savedAccount, account.ID).Error)
	assert.Equal(t, "https://new-management.example", savedAccount.BaseURL)
	assert.Equal(t, "https://new-relay.example/v1", savedAccount.RelayBaseURL)
	require.NoError(t, db.First(&savedChannel, channel.Id).Error)
	require.NotNil(t, savedChannel.BaseURL)
	assert.Equal(t, "https://new-relay.example", *savedChannel.BaseURL)
}

func TestRestoreSub2APIPasswordLoginIdentityUsesHistoricalEmail(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	missingTableCredential := model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
	}
	restoreSub2APIPasswordLoginIdentity(1, &missingTableCredential)
	assert.Equal(t, "operator", missingTableCredential.Username)

	require.NoError(t, db.AutoMigrate(&model.PlatformSiteIdentity{}))
	tests := []struct {
		name           string
		channelID      int
		historicalName string
		credentialName string
		wantCredential string
	}{
		{
			name:           "empty credential username",
			channelID:      101,
			historicalName: "display-name",
			wantCredential: "operator@example.com",
		},
		{
			name:           "historical username drift",
			channelID:      102,
			historicalName: "operator",
			credentialName: "operator",
			wantCredential: "operator@example.com",
		},
		{
			name:           "explicit username is preserved",
			channelID:      103,
			historicalName: "operator",
			credentialName: "manual-login",
			wantCredential: "manual-login",
		},
		{
			name:           "invalid historical email is ignored",
			channelID:      104,
			historicalName: "operator",
			credentialName: "operator",
			wantCredential: "operator",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			email := "operator@example.com"
			if testCase.name == "invalid historical email is ignored" {
				email = "not-an-email"
			}
			require.NoError(t, db.Create(&model.PlatformSiteIdentity{
				ChannelID: testCase.channelID,
				Username:  testCase.historicalName,
				Email:     email,
			}).Error)
			credential := model.PlatformSiteCredential{
				AuthType: model.UpstreamAuthPassword,
				Username: testCase.credentialName,
			}
			restoreSub2APIPasswordLoginIdentity(testCase.channelID, &credential)
			assert.Equal(t, testCase.wantCredential, credential.Username)
		})
	}
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

func TestSyncPlatformSitePersistsPasswordSessionBeforeResourceFailure(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.CryptoSecret = "upstream-site-password-session-test-secret"
	common.MemoryCacheEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.PlatformSiteIdentity{},
		&model.PlatformSiteGroup{},
		&model.PlatformSiteEndpoint{},
		&model.PlatformSiteEndpointCapability{},
		&model.PlatformSiteResourceSync{},
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

	loginRequests := 0
	logoutRequests := 0
	deleteSessionRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			loginRequests++
			http.SetCookie(writer, &http.Cookie{Name: "new_api_refresh", Value: "refresh-1", Path: "/"})
			_, _ = writer.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"access_token":"login-access","token_type":"Bearer","access_expires_at":%d,"session":{"sid":"login-session","current":true},"user":{"id":17}}}`, common.GetTimestamp()+3600)))
		case "/api/user/self":
			assert.Equal(t, "Bearer login-access", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":5000000,"used_quota":1000000}}`))
		case "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/token/":
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"success":false,"code":403,"message":"permission denied"}`))
		case "/api/user/auth/logout":
			logoutRequests++
			assert.Equal(t, http.MethodPost, request.Method)
			assert.Equal(t, "Bearer login-access", request.Header.Get("Authorization"))
			assert.Equal(t, "login-session", request.Header.Get("X-Auth-Session"))
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"success":false,"message":"logout failed"}`))
		case "/api/user/sessions/login-session":
			deleteSessionRequests++
			assert.Equal(t, http.MethodDelete, request.Method)
			assert.Equal(t, "Bearer login-access", request.Header.Get("Authorization"))
			assert.Equal(t, "login-session", request.Header.Get("X-Auth-Session"))
			_, _ = writer.Write([]byte(`{"success":true}`))
		case "/api/user/sessions/revoke-others":
			t.Fatalf("密码同步不得撤销其他设备会话")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	priority := int64(2)
	channel := &model.Channel{
		Id:           903,
		Name:         "password-session-persistence",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
		Priority:     &priority,
	}
	require.NoError(t, db.Create(channel).Error)
	ciphertext, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator",
		Password: "synthetic-password",
	})
	require.NoError(t, err)
	account := &model.PlatformSiteAccount{
		ChannelID:            channel.Id,
		Platform:             model.PlatformNewAPI,
		BaseURL:              server.URL,
		AuthType:             model.UpstreamAuthPassword,
		CredentialCiphertext: ciphertext,
		CredentialKeyVersion: "v1",
		ConversionRatio:      0.1,
		SyncStatus:           model.UpstreamSiteSyncIdle,
	}
	require.NoError(t, db.Create(account).Error)
	oldSecret, err := model.EncryptPlatformSiteCredential(
		model.PlatformSiteCredential{AccessToken: "sk-old"},
	)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.UpstreamKey{
		ChannelID:        channel.Id,
		ExternalID:       "old-key",
		Name:             "old",
		SecretCiphertext: oldSecret,
		Models:           "gpt-4o",
		ModelsSynced:     true,
		Status:           model.UpstreamKeyStatusEnabled,
	}).Error)

	err = SyncUpstreamSite(context.Background(), channel.Id)
	require.Error(t, err)
	assert.Equal(t, 1, loginRequests)

	var saved model.PlatformSiteAccount
	require.NoError(t, db.First(&saved, account.ID).Error)
	savedCredential, err := model.DecryptPlatformSiteCredential(saved.CredentialCiphertext)
	require.NoError(t, err)
	assert.Equal(t, model.UpstreamAuthPassword, savedCredential.AuthType)
	assert.Equal(t, "operator", savedCredential.Username)
	assert.Equal(t, "synthetic-password", savedCredential.Password)
	assert.Empty(t, savedCredential.AccessToken)
	assert.Empty(t, savedCredential.RefreshToken)
	assert.Empty(t, savedCredential.RefreshStatus)
	assert.False(t, savedCredential.ReauthRequired)
	assert.False(t, savedCredential.RefreshUncertain)
	assert.Equal(t, "17", savedCredential.UserID)
	assert.Empty(t, savedCredential.SessionID)
	assert.Empty(t, savedCredential.Cookie)
	assert.Equal(t, model.UpstreamSiteSyncFailed, saved.SyncStatus)
	assert.Equal(t, 1, logoutRequests)
	assert.Equal(t, 1, deleteSessionRequests)
	assert.Zero(t, saved.LastSyncAt)
	assert.Equal(t, model.PlatformSiteAuthStatusAuthenticated, saved.AuthStatus)
	assert.Equal(t, 10.0, saved.Balance)
	assert.Equal(t, int64(1000000), saved.UsedQuota)

	var savedIdentity model.PlatformSiteIdentity
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedIdentity).Error)
	assert.Equal(t, "17", savedIdentity.PlatformUserID)
	assert.Equal(t, 10.0, savedIdentity.Balance)
	assert.Equal(t, int64(1000000), savedIdentity.UsedQuota)

	var savedResource model.PlatformSiteResourceSync
	require.NoError(t, db.Where(
		"channel_id = ? AND resource_type = ?",
		channel.Id,
		model.PlatformSiteResourceKeys,
	).First(&savedResource).Error)
	assert.Equal(t, model.PlatformSiteResourceStatusFailed, savedResource.Status)
	assert.True(t, savedResource.UsingSnapshot)
	assert.NotContains(t, SafePlatformSiteError(err), "账号或密码")

	var savedKey model.UpstreamKey
	require.NoError(t, db.Where(
		"channel_id = ? AND external_id = ?",
		channel.Id,
		"old-key",
	).First(&savedKey).Error)
	oldCredential, err := model.DecryptPlatformSiteCredential(savedKey.SecretCiphertext)
	require.NoError(t, err)
	assert.Equal(t, "sk-old", oldCredential.AccessToken)
	assert.Equal(t, model.UpstreamKeyStatusEnabled, savedKey.Status)
}

func TestSyncPlatformSiteSub2APIPasswordResourceFailureCleansSession(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.CryptoSecret = "upstream-site-sub2-password-cleanup-test-secret"
	common.MemoryCacheEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.PlatformSiteIdentity{},
		&model.PlatformSiteGroup{},
		&model.PlatformSiteEndpoint{},
		&model.PlatformSiteEndpointCapability{},
		&model.PlatformSiteResourceSync{},
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

	logoutRequests := 0
	var logoutBody string
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/":
			http.NotFound(writer, request)
		case "/api/v1/settings/public":
			http.NotFound(writer, request)
		case "/api/v1/auth/login":
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"access_token":"sub2-temporary-access","refresh_token":"sub2-temporary-refresh","expires_in":3600,"user":{"id":29}}}`,
			))
		case "/api/v1/auth/me":
			assert.Equal(t, "Bearer sub2-temporary-access", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"id":29,"username":"sub2-user","balance":8}}`,
			))
		case "/api/v1/user/profile":
			http.NotFound(writer, request)
		case "/api/v1/groups/available":
			_, _ = writer.Write([]byte(`{"code":0,"data":[]}`))
		case "/api/v1/groups/rates":
			_, _ = writer.Write([]byte(`{"code":0,"data":{}}`))
		case "/api/v1/usage/dashboard/stats", "/api/v1/usage/stats":
			http.NotFound(writer, request)
		case "/api/v1/keys":
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"code":500,"message":"temporary key failure"}`))
		case "/api/v1/auth/logout":
			logoutRequests++
			assert.Empty(t, request.Header.Get("Authorization"))
			assert.Empty(t, request.Header.Get("Cookie"))
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			logoutBody = string(body)
			_, _ = writer.Write([]byte(`{"code":0}`))
		case "/api/v1/auth/revoke-all-sessions":
			t.Fatalf("密码同步不得撤销全部 Sub2API 会话")
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	channel := &model.Channel{
		Id:           906,
		Name:         "sub2-password-resource-failure",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
	}
	require.NoError(t, db.Create(channel).Error)
	credentialCiphertext, err := model.EncryptPlatformSiteCredential(
		model.PlatformSiteCredential{
			AuthType:     model.UpstreamAuthPassword,
			Username:     "operator",
			Password:     "synthetic-password",
			AccessToken:  "stale-access-token",
			RefreshToken: "stale-refresh-token",
			SessionID:    "stale-session",
			Cookie:       "session=stale",
		},
	)
	require.NoError(t, err)
	account := &model.PlatformSiteAccount{
		ChannelID:            channel.Id,
		Platform:             model.PlatformSub2API,
		BaseURL:              server.URL,
		AuthType:             model.UpstreamAuthPassword,
		CredentialCiphertext: credentialCiphertext,
		CredentialKeyVersion: "v1",
		SyncStatus:           model.UpstreamSiteSyncIdle,
	}
	require.NoError(t, db.Create(account).Error)

	err = SyncUpstreamSite(context.Background(), channel.Id)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteResource)
	assert.Equal(t, 1, logoutRequests)
	assert.JSONEq(t, `{"refresh_token":"sub2-temporary-refresh"}`, logoutBody)
	assert.NotContains(t, SafePlatformSiteError(err), "撤销全部")

	var saved model.PlatformSiteAccount
	require.NoError(t, db.First(&saved, account.ID).Error)
	savedCredential, decryptErr := model.DecryptPlatformSiteCredential(saved.CredentialCiphertext)
	require.NoError(t, decryptErr)
	assert.Equal(t, "sub2-user", savedCredential.Username)
	assert.Equal(t, "29", savedCredential.UserID)
	assert.Empty(t, savedCredential.AccessToken)
	assert.Empty(t, savedCredential.RefreshToken)
	assert.Empty(t, savedCredential.SessionID)
	assert.Empty(t, savedCredential.Cookie)
	assert.NotContains(t, saved.CredentialCiphertext, "sub2-temporary-access")
	assert.NotContains(t, saved.CredentialCiphertext, "sub2-temporary-refresh")
}

func TestSyncPlatformSiteSucceedsWithUsableKeyAndPartialResourceFailure(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.CryptoSecret = "upstream-site-partial-resource-success-test-secret"
	common.MemoryCacheEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.PlatformSiteIdentity{},
		&model.PlatformSiteGroup{},
		&model.PlatformSiteEndpoint{},
		&model.PlatformSiteEndpointCapability{},
		&model.PlatformSiteResourceSync{},
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

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":17,"quota":5000000,"used_quota":1000000}}`))
		case "/api/user/models":
			_, _ = writer.Write([]byte(`{"success":true,"data":["gpt-4o"]}`))
		case "/api/user/self/groups":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"default":{"ratio":1}}}`))
		case "/api/pricing":
			http.NotFound(writer, request)
		case "/api/token/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"page":1,"page_size":100,"total":2,"items":[{"id":1,"name":"healthy","key":"sk-healthy","group":"default","models":["gpt-4o"]},{"id":2,"name":"protected","key":"sk-****","group":"default","models":["gpt-4o"]}]}}`))
		case "/api/token/batch/keys":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"keys":{}}}`))
		case "/api/token/2/key":
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"success":false,"message":"verification required"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	channel := &model.Channel{
		Id:           905,
		Name:         "partial-resource-success",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
	}
	require.NoError(t, db.Create(channel).Error)
	ciphertext, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{
		AuthType:       model.UpstreamAuthAccessToken,
		AccessToken:    "session-token",
		TokenExpiresAt: common.GetTimestamp() + 3600,
	})
	require.NoError(t, err)
	account := &model.PlatformSiteAccount{
		ChannelID:            channel.Id,
		Platform:             model.PlatformNewAPI,
		BaseURL:              server.URL,
		AuthType:             model.UpstreamAuthAccessToken,
		CredentialCiphertext: ciphertext,
		CredentialKeyVersion: "v1",
		ConversionRatio:      0.1,
		SyncStatus:           model.UpstreamSiteSyncIdle,
	}
	require.NoError(t, db.Create(account).Error)

	require.NoError(t, SyncUpstreamSite(context.Background(), channel.Id))

	var saved model.PlatformSiteAccount
	require.NoError(t, db.First(&saved, account.ID).Error)
	assert.Equal(t, model.UpstreamSiteSyncSuccess, saved.SyncStatus)
	assert.NotZero(t, saved.LastSyncAt)
	assert.Equal(t, model.PlatformSiteAuthStatusSecureVerificationRequired, saved.AuthStatus)

	var savedKey model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "1").First(&savedKey).Error)
	assert.Equal(t, model.UpstreamKeyStatusEnabled, savedKey.Status)
	assert.True(t, savedKey.ModelsSynced)

	var protectedKey model.UpstreamKey
	assert.ErrorIs(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "2").First(&protectedKey).Error, gorm.ErrRecordNotFound)

	var keyResource model.PlatformSiteResourceSync
	require.NoError(t, db.Where(
		"channel_id = ? AND resource_type = ?",
		channel.Id,
		model.PlatformSiteResourceKeys,
	).First(&keyResource).Error)
	assert.Equal(t, model.PlatformSiteResourceStatusSecureVerificationRequired, keyResource.Status)
	assert.True(t, keyResource.UsingSnapshot)
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
		&model.PlatformSiteIdentity{},
		&model.PlatformSiteGroup{},
		&model.PlatformSiteEndpoint{},
		&model.PlatformSiteEndpointCapability{},
		&model.PlatformSiteResourceSync{},
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
	assert.Equal(t, model.PlatformSiteAuthStatusCredentialsInvalid, savedAccount.AuthStatus)
	assert.Contains(t, savedAccount.LastSyncError, "账号或密码错误")
	assert.NotContains(t, savedAccount.LastSyncError, "session-token")

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

func TestSyncPlatformSiteLoginAgreementFailurePreservesLastSuccessfulSnapshot(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.CryptoSecret = "upstream-site-login-agreement-test-secret"
	common.MemoryCacheEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.PlatformSiteIdentity{},
		&model.PlatformSiteGroup{},
		&model.PlatformSiteEndpoint{},
		&model.PlatformSiteEndpointCapability{},
		&model.PlatformSiteResourceSync{},
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

	loginRequests := 0
	settingsRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/settings/public":
			settingsRequests++
			if loginRequests == 0 {
				_, _ = writer.Write([]byte(
					`{"code":0,"data":{"login_agreement_enabled":false,"login_agreement_revision":""}}`,
				))
				return
			}
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"login_agreement_enabled":true,"login_agreement_revision":"terms-rev"}}`,
			))
		case "/api/v1/auth/login":
			loginRequests++
			if loginRequests > 1 {
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = writer.Write([]byte(
					`{"code":400,"message":"agreement required","detail":"upstream-secret-response"}`,
				))
				return
			}
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"access_token":"session-token"}}`,
			))
		case "/api/v1/auth/me":
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"id":7,"balance":2,"used_quota":3}}`,
			))
		case "/api/v1/user/profile":
			http.NotFound(writer, request)
		case "/api/v1/groups/available":
			_, _ = writer.Write([]byte(`{"code":0,"data":[]}`))
		case "/api/v1/groups/rates":
			_, _ = writer.Write([]byte(`{"code":0,"data":{}}`))
		case "/api/v1/usage/dashboard/stats", "/api/v1/usage/stats":
			http.NotFound(writer, request)
		case "/api/v1/keys":
			_, _ = writer.Write([]byte(
				`{"code":0,"data":{"items":[{"id":"key-1","name":"primary","key":"sk-stable","models":["gpt-4o"],"quota_used":4}],"total":1,"page_size":100}}`,
			))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	channel := &model.Channel{
		Name:         "sub2api-login-agreement",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
	}
	require.NoError(t, db.Create(channel).Error)
	credentialCiphertext, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: "operator@example.com",
		Password: "synthetic-password",
	})
	require.NoError(t, err)
	account := &model.PlatformSiteAccount{
		ChannelID:            channel.Id,
		Platform:             model.PlatformSub2API,
		BaseURL:              server.URL,
		AuthType:             model.UpstreamAuthPassword,
		CredentialCiphertext: credentialCiphertext,
		CredentialKeyVersion: "v1",
		ConversionRatio:      0.1,
		SyncStatus:           model.UpstreamSiteSyncIdle,
	}
	require.NoError(t, db.Create(account).Error)

	require.NoError(t, SyncUpstreamSite(context.Background(), channel.Id))
	var savedAccount model.PlatformSiteAccount
	require.NoError(t, db.First(&savedAccount, account.ID).Error)
	assert.Equal(t, model.UpstreamSiteSyncSuccess, savedAccount.SyncStatus)
	assert.Equal(t, model.PlatformSiteAuthStatusAuthenticated, savedAccount.AuthStatus)
	assert.NotZero(t, savedAccount.LastSyncAt)
	successfulSyncAt := savedAccount.LastSyncAt
	successfulBalance := savedAccount.Balance
	successfulUsedQuota := savedAccount.UsedQuota

	var savedKey model.UpstreamKey
	require.NoError(t, db.Where(
		"channel_id = ? AND external_id = ?",
		channel.Id,
		"key-1",
	).First(&savedKey).Error)
	oldKeyCiphertext := savedKey.SecretCiphertext
	oldKeyModels := savedKey.Models

	err = SyncUpstreamSite(context.Background(), channel.Id)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSub2APILoginAgreement)
	assert.Equal(t, 3, loginRequests)
	assert.Equal(t, 1, settingsRequests)

	require.NoError(t, db.First(&savedAccount, account.ID).Error)
	assert.Equal(t, model.UpstreamSiteSyncFailed, savedAccount.SyncStatus)
	assert.Equal(t, model.PlatformSiteAuthStatusLoginAgreementRequired, savedAccount.AuthStatus)
	assert.Equal(t, successfulSyncAt, savedAccount.LastSyncAt)
	assert.Equal(t, successfulBalance, savedAccount.Balance)
	assert.Equal(t, successfulUsedQuota, savedAccount.UsedQuota)
	assert.Equal(t, 1, savedAccount.ConsecutiveFailures)
	assert.Contains(t, savedAccount.AuthStatusReason, "已尝试提交当前服务条款版本")
	assert.NotContains(t, savedAccount.AuthStatusReason, "synthetic-password")
	assert.NotContains(t, savedAccount.AuthStatusReason, "upstream-secret-response")

	require.NoError(t, db.First(&savedKey, savedKey.ID).Error)
	assert.Equal(t, oldKeyCiphertext, savedKey.SecretCiphertext)
	assert.Equal(t, oldKeyModels, savedKey.Models)
}

func TestSyncPlatformSiteLoginAgreementStatusDatabaseMatrix(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		dbType    common.DatabaseType
		dialector func(string) gorm.Dialector
		dsn       func(*testing.T) string
	}{
		{
			name:   "sqlite",
			dbType: common.DatabaseTypeSQLite,
			dialector: func(dsn string) gorm.Dialector {
				return sqlite.Open(dsn)
			},
			dsn: func(t *testing.T) string {
				t.Helper()
				return fmt.Sprintf(
					"file:%s?mode=memory&cache=shared",
					strings.ReplaceAll(t.Name(), "/", "_"),
				)
			},
		},
		{
			name:      "mysql",
			env:       "TEST_UPSTREAM_SITE_MYSQL_DSN",
			dbType:    common.DatabaseTypeMySQL,
			dialector: func(dsn string) gorm.Dialector { return mysql.Open(dsn) },
		},
		{
			name:   "postgres",
			env:    "TEST_UPSTREAM_SITE_POSTGRES_DSN",
			dbType: common.DatabaseTypePostgreSQL,
			dialector: func(dsn string) gorm.Dialector {
				return postgres.New(postgres.Config{
					DSN:                  dsn,
					PreferSimpleProtocol: true,
				})
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			dsn := ""
			if testCase.dsn != nil {
				dsn = testCase.dsn(t)
			} else {
				dsn = strings.TrimSpace(os.Getenv(testCase.env))
				if dsn == "" {
					t.Skip(testCase.env + " 未配置")
				}
				assertScratchDatabaseDSN(t, dsn)
			}

			prefix := fmt.Sprintf("la_%d_", time.Now().UnixNano())
			db, err := gorm.Open(testCase.dialector(dsn), &gorm.Config{
				NamingStrategy: schema.NamingStrategy{TablePrefix: prefix},
			})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			t.Cleanup(func() {
				_ = db.Migrator().DropTable(&model.PlatformSiteAccount{}, &model.Channel{})
			})
			require.NoError(t, db.AutoMigrate(
				&model.Channel{},
				&model.PlatformSiteAccount{},
			))

			versionQuery := "select version()"
			if testCase.name == "sqlite" {
				versionQuery = "select sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database: %s", version)

			previousDB := model.DB
			previousMainType := common.MainDatabaseType()
			previousLogType := common.LogDatabaseType()
			previousSecret := common.CryptoSecret
			model.DB = db
			common.SetDatabaseTypes(testCase.dbType, testCase.dbType)
			common.CryptoSecret = "upstream-site-login-agreement-matrix-secret"
			t.Cleanup(func() {
				model.DB = previousDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
				common.CryptoSecret = previousSecret
			})

			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				request *http.Request,
			) {
				writer.Header().Set("Content-Type", "application/json")
				if request.Method == http.MethodGet &&
					request.URL.Path == "/api/v1/settings/public" {
					_, _ = writer.Write([]byte(
						`{"code":0,"data":{"login_agreement_enabled":true,"login_agreement_revision":"terms-rev"}}`,
					))
					return
				}
				if request.Method == http.MethodPost &&
					request.URL.Path == "/api/v1/auth/login" {
					writer.WriteHeader(http.StatusBadRequest)
					_, _ = writer.Write([]byte(
						`{"code":400,"message":"agreement required"}`,
					))
					return
				}
				http.NotFound(writer, request)
			}))
			t.Cleanup(server.Close)

			priority := int64(1)
			channel := &model.Channel{
				Name:         "login-agreement-matrix",
				Status:       common.ChannelStatusEnabled,
				UpstreamKind: model.UpstreamKindPlatformSite,
				Group:        "default",
				Priority:     &priority,
			}
			require.NoError(t, db.Create(channel).Error)
			credentialCiphertext, err := model.EncryptPlatformSiteCredential(
				model.PlatformSiteCredential{
					AuthType: model.UpstreamAuthPassword,
					Username: "operator@example.com",
					Password: "synthetic-password",
				},
			)
			require.NoError(t, err)
			account := &model.PlatformSiteAccount{
				ChannelID:            channel.Id,
				Platform:             model.PlatformSub2API,
				BaseURL:              server.URL,
				AuthType:             model.UpstreamAuthPassword,
				CredentialCiphertext: credentialCiphertext,
				CredentialKeyVersion: "v1",
				Balance:              8.5,
				UsedQuota:            21,
				LastSyncAt:           1_700_000_000,
				SyncStatus:           model.UpstreamSiteSyncSuccess,
				ConsecutiveFailures:  3,
			}
			require.NoError(t, db.Create(account).Error)

			err = SyncUpstreamSite(context.Background(), channel.Id)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrSub2APILoginAgreement)

			var saved model.PlatformSiteAccount
			require.NoError(t, db.First(&saved, account.ID).Error)
			assert.Equal(t, model.UpstreamSiteSyncFailed, saved.SyncStatus)
			assert.Equal(t, model.PlatformSiteAuthStatusLoginAgreementRequired, saved.AuthStatus)
			assert.Contains(t, saved.AuthStatusReason, "已尝试提交当前服务条款版本")
			assert.NotContains(t, saved.AuthStatusReason, "synthetic-password")
			assert.Equal(t, 4, saved.ConsecutiveFailures)
			assert.Equal(t, int64(1_700_000_000), saved.LastSyncAt)
			assert.Equal(t, 8.5, saved.Balance)
			assert.Equal(t, int64(21), saved.UsedQuota)
		})
	}
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
		Balance:      5,
		Models:       []string{"gpt-4o"},
		Keys:         nil,
		KeysComplete: true,
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
