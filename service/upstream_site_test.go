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
	"github.com/c1cadaBob/NexusTok/model"
	"github.com/c1cadaBob/NexusTok/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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

func TestPlatformSitePasswordAuthenticationDoesNotDriftToTokenRefresh(t *testing.T) {
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
					t.Fatalf("密码认证不应尝试刷新令牌")
				case testCase.loginPath:
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
				AuthType:     model.UpstreamAuthPassword,
				Username:     "operator",
				Password:     "synthetic-password",
				AccessToken:  "stale-access",
				RefreshToken: "stale-refresh",
			}
			session, err := testCase.newAdapter(server.Client()).Authenticate(context.Background(), server.URL, credential)
			require.NoError(t, err)
			assert.Nil(t, session.CredentialUpdate)
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
	assert.Nil(t, session.CredentialUpdate)
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
		AuthType:     model.UpstreamAuthAccessToken,
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
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
	assert.Equal(t, int64(1), snapshot.UsedQuota)
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
			_, _ = writer.Write([]byte(`{"code":0,"data":{"total_actual_cost":2}}`))
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
	assert.Equal(t, int64(2), snapshot.UsedQuota)
	assert.True(t, snapshot.UsedQuotaSet)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "default", snapshot.Keys[0].Group)
	assert.Equal(t, 0.25, snapshot.Keys[0].SourceConversionRatio)
	assert.Equal(t, []string{"gpt-4o", "gemini-2.5-pro"}, snapshot.Keys[0].Models)
	assert.True(t, snapshot.Keys[0].ModelsSynced)
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
	// TEST_UPSTREAM_SITE_MYSQL_DSN and TEST_UPSTREAM_SITE_POSTGRES_DSN must
	// point to isolated scratch databases because this test drops its tables.
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
			db, err := gorm.Open(dialect.open(dialect.dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			if dialect.name != "sqlite" {
				require.NoError(t, db.Exec("DROP TABLE IF EXISTS upstream_key_abilities").Error)
				require.NoError(t, db.Exec("DROP TABLE IF EXISTS upstream_keys").Error)
				require.NoError(t, db.Exec("DROP TABLE IF EXISTS platform_site_accounts").Error)
				require.NoError(t, db.Exec("DROP TABLE IF EXISTS abilities").Error)
				require.NoError(t, db.Exec("DROP TABLE IF EXISTS channels").Error)
			}
			t.Cleanup(func() {
				if dialect.name != "sqlite" {
					require.NoError(t, db.Exec("DROP TABLE IF EXISTS upstream_key_abilities").Error)
					require.NoError(t, db.Exec("DROP TABLE IF EXISTS upstream_keys").Error)
					require.NoError(t, db.Exec("DROP TABLE IF EXISTS platform_site_accounts").Error)
					require.NoError(t, db.Exec("DROP TABLE IF EXISTS abilities").Error)
					require.NoError(t, db.Exec("DROP TABLE IF EXISTS channels").Error)
				}
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
