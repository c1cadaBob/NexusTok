package upstreamaccount

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNewAPIPreviewFetchesKeysRatesAndBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":100}}`))
		case "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "authenticated-session", Path: "/"})
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default"}}`))
		case "/api/user/self":
			require.Equal(t, "7", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default","quota":250,"used_quota":50}}`))
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1,"desc":"Default"},"vip":{"ratio":0.5,"desc":"VIP"}}}`))
		case "/api/ratio_config":
			_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{"gpt-4o":2},"completion_ratio":{},"cache_ratio":{},"create_cache_ratio":{},"model_price":{}}}`))
		case "/api/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":11,"name":"key-a","key":"sk-***abcd","group":"vip","status":1,"model_limits":"gpt-4o","remain_quota":120,"used_quota":30}],"total":1}}`))
		case "/api/token/batch/keys":
			_, _ = w.Write([]byte(`{"success":true,"data":{"11":"sk-newapi-full-key"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformNewAPI,
		BaseURL:  server.URL,
		Username: "alice",
		Password: "secret",
	}})

	require.NoError(t, err)
	require.NotEmpty(t, result.PreviewID)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Equal(t, "11", result.Snapshot.Keys[0].SyncID)
	require.Empty(t, result.Snapshot.Keys[0].Key)
	require.Equal(t, "sk-new...-key", result.Snapshot.Keys[0].MaskedKey)
	require.Equal(t, "vip", result.Snapshot.Keys[0].GroupName)
	require.Equal(t, float64(2.5), *result.Snapshot.Balance.BalanceUSD)
	require.Equal(t, float64(0.5), *result.Snapshot.Balance.UsedUSD)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, "sk-newapi-full-key", record.Snapshot.Keys[0].Key)
	require.Nil(t, result.Snapshot.StoredCredential)
	require.NotNil(t, record.Snapshot.StoredCredential)
	require.NotContains(t, record.Snapshot.StoredCredential.Password, "secret")
	decryptedPassword, err := common.DecryptSensitiveString(record.Snapshot.StoredCredential.Password)
	require.NoError(t, err)
	require.Equal(t, "secret", decryptedPassword)
	require.NotEmpty(t, record.Snapshot.StoredCredential.Session)
	require.NotContains(t, record.Snapshot.StoredCredential.Session, "authenticated-session")
	authSession, err := decryptAuthenticatedSession(record.Snapshot.StoredCredential.Session)
	require.NoError(t, err)
	require.NotNil(t, authSession)
	require.Equal(t, PlatformNewAPI, authSession.Platform)
	require.Equal(t, "7", authSession.NewAPI.UserID)
	require.NotEmpty(t, authSession.NewAPI.Cookies)
}

func TestNewAPIPreviewAcceptsNestedUserAndAccessTokenLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":100}}`))
		case "/api/user/login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"access_token":"nested-access-token","token_type":"Bearer","user":{"id":5,"display_name":"c1cada","group":"default"}}}`))
		case "/api/user/self":
			require.Equal(t, "Bearer nested-access-token", r.Header.Get("Authorization"))
			require.Equal(t, "5", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":5,"display_name":"c1cada","group":"default","quota":300,"used_quota":50}}`))
		case "/api/user/self/groups":
			require.Equal(t, "Bearer nested-access-token", r.Header.Get("Authorization"))
			require.Equal(t, "5", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1,"desc":"Default"}}}`))
		case "/api/ratio_config":
			_, _ = w.Write([]byte(`{"success":false,"message":"hidden"}`))
		case "/api/token/":
			require.Equal(t, "Bearer nested-access-token", r.Header.Get("Authorization"))
			require.Equal(t, "5", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":272,"name":"key-a","key":"sk-***abcd","group":"default","status":1,"model_limits":"gpt-4o","remain_quota":120,"used_quota":30}],"total":1}}`))
		case "/api/token/batch/keys":
			require.Equal(t, "Bearer nested-access-token", r.Header.Get("Authorization"))
			require.Equal(t, "5", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"keys":{"272":"sk-nested-full-key"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformNewAPI,
		BaseURL:  server.URL,
		Username: "c1cada",
		Password: "secret",
	}})

	require.NoError(t, err)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Equal(t, "5", result.Snapshot.User.ID)
	require.Equal(t, "c1cada", result.Snapshot.User.Username)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, "sk-nested-full-key", record.Snapshot.Keys[0].Key)
	authSession, err := decryptAuthenticatedSession(record.Snapshot.StoredCredential.Session)
	require.NoError(t, err)
	require.Equal(t, "5", authSession.NewAPI.UserID)
	require.Equal(t, "nested-access-token", authSession.NewAPI.AccessToken)
}

func TestNewAPIPreviewContinuesWhenStatusEndpointFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"success":false,"message":"status upstream timeout"}`))
		case "/api/user/login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default"}}`))
		case "/api/user/self":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default","quota":500000,"used_quota":0}}`))
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1,"desc":"Default"}}}`))
		case "/api/ratio_config":
			_, _ = w.Write([]byte(`{"success":false,"message":"hidden"}`))
		case "/api/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":11,"name":"key-a","key":"sk-status-full-key","group":"default","status":1,"model_limits":"gpt-4o","remain_quota":0,"used_quota":0}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformNewAPI,
		BaseURL:  server.URL,
		Username: "alice",
		Password: "secret",
	}})

	require.NoError(t, err)
	require.Len(t, result.Snapshot.Keys, 1)
	require.NotEmpty(t, result.Snapshot.Warnings)
	require.Contains(t, strings.Join(result.Snapshot.Warnings, "\n"), "/api/status 不可用")
	require.Equal(t, float64(1), *result.Snapshot.Balance.BalanceUSD)
}

func TestNewAPIPreviewFallsBackToSingleKeyReveal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":100}}`))
		case "/api/user/login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default"}}`))
		case "/api/user/self":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default","quota":0,"used_quota":0}}`))
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1,"desc":"Default"}}}`))
		case "/api/ratio_config":
			_, _ = w.Write([]byte(`{"success":false,"message":"hidden"}`))
		case "/api/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":11,"name":"key-a","key":"sk-***abcd","group":"default","status":1,"model_limits":"gpt-4o","remain_quota":120,"used_quota":30}],"total":1}}`))
		case "/api/token/batch/keys":
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"success":false,"message":"rate limited"}`))
		case "/api/token/11/key":
			_, _ = w.Write([]byte(`{"success":true,"data":{"key":"sk-single-full-key"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformNewAPI,
		BaseURL:  server.URL,
		Username: "alice",
		Password: "secret",
	}})

	require.NoError(t, err)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Equal(t, "sk-sin...-key", result.Snapshot.Keys[0].MaskedKey)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, "sk-single-full-key", record.Snapshot.Keys[0].Key)
}

func TestNewAPIPreviewFailsWhenFullKeyCannotBeRevealed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":100}}`))
		case "/api/user/login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default"}}`))
		case "/api/user/self":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default","quota":0,"used_quota":0}}`))
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1,"desc":"Default"}}}`))
		case "/api/ratio_config":
			_, _ = w.Write([]byte(`{"success":false,"message":"hidden"}`))
		case "/api/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":11,"name":"key-a","key":"sk-***abcd","group":"default","status":1,"model_limits":"gpt-4o","remain_quota":120,"used_quota":30}],"total":1}}`))
		case "/api/token/batch/keys", "/api/token/11/key":
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"success":false,"message":"rate limited"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformNewAPI,
		BaseURL:  server.URL,
		Username: "alice",
		Password: "secret",
	}})

	require.Error(t, err)
	require.Contains(t, err.Error(), "读取 new-api 完整 Key 失败")
}

func TestNewAPIPreviewReturnsChallengeWhenTwoFARequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":100}}`))
		case "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "pending-session", Path: "/"})
			_, _ = w.Write([]byte(`{"success":true,"message":"Please enter two-factor authentication code","data":{"require_2fa":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformNewAPI,
		BaseURL:  server.URL,
		Username: "alice",
		Password: "secret",
	}})

	require.NoError(t, err)
	require.Empty(t, result.PreviewID)
	require.Nil(t, result.Snapshot)
	require.NotNil(t, result.Challenge)
	require.Equal(t, PlatformNewAPI, result.Challenge.Platform)
	require.Equal(t, authChallengeTypeTOTP, result.Challenge.Type)
	require.NotEmpty(t, result.Challenge.ChallengeID)

	record, found, err := authChallengeCache.Get(result.Challenge.ChallengeID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "alice", record.Username)
	require.Equal(t, server.URL, record.BaseURL)
	require.NotNil(t, record.Credential)
	decryptedPassword, err := common.DecryptSensitiveString(record.Credential.Password)
	require.NoError(t, err)
	require.Equal(t, "secret", decryptedPassword)
	require.Equal(t, float64(100), record.NewAPI.QuotaPerUnit)
	require.NotEmpty(t, record.NewAPI.Cookies)
}

func TestNewAPIPreviewCompletesTwoFAChallenge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/login/2fa":
			cookie, err := r.Cookie("session")
			require.NoError(t, err)
			require.Equal(t, "pending-session", cookie.Value)
			var body map[string]string
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "123456", body["code"])
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "authenticated-session", Path: "/"})
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default"}}`))
		case "/api/user/self":
			require.Equal(t, "7", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default","quota":250,"used_quota":50}}`))
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1,"desc":"Default"}}}`))
		case "/api/ratio_config":
			_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{"gpt-4o":2},"completion_ratio":{},"cache_ratio":{},"create_cache_ratio":{},"model_price":{}}}`))
		case "/api/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":11,"name":"key-a","key":"sk-***abcd","group":"default","status":1,"model_limits":"gpt-4o","remain_quota":120,"used_quota":30}],"total":1}}`))
		case "/api/token/batch/keys":
			_, _ = w.Write([]byte(`{"success":true,"data":{"11":"sk-newapi-full-key"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	api, err := newHTTPClient(server.URL, &http.Client{Jar: jar})
	require.NoError(t, err)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "pending-session", Path: "/"}})

	challenge, err := saveAuthChallenge(AuthChallengeRecord{
		Platform: PlatformNewAPI,
		BaseURL:  api.baseURL,
		Username: "alice",
		Credential: func() *StoredCredential {
			stored, err := buildStoredCredentialWithBase(
				PlatformNewAPI,
				api.baseURL,
				Credential{
					Platform: PlatformNewAPI,
					BaseURL:  api.baseURL,
					Username: "alice",
					Password: "secret",
				},
			)
			require.NoError(t, err)
			return stored
		}(),
		NewAPI: &NewAPIChallengeData{
			QuotaPerUnit: 100,
			Cookies:      storeCookiesFromJar(api),
		},
	})
	require.NoError(t, err)

	result, err := CompletePreview2FA(context.Background(), Preview2FARequest{
		ChallengeID: challenge.ChallengeID,
		Code:        "123456",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.PreviewID)
	require.Nil(t, result.Challenge)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Empty(t, result.Snapshot.Keys[0].Key)
	require.Equal(t, "sk-new...-key", result.Snapshot.Keys[0].MaskedKey)
	require.Equal(t, float64(2.5), *result.Snapshot.Balance.BalanceUSD)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, "sk-newapi-full-key", record.Snapshot.Keys[0].Key)
	require.NotNil(t, record.Snapshot.StoredCredential)
	decryptedPassword, err := common.DecryptSensitiveString(record.Snapshot.StoredCredential.Password)
	require.NoError(t, err)
	require.Equal(t, "secret", decryptedPassword)
	authSession, err := decryptAuthenticatedSession(record.Snapshot.StoredCredential.Session)
	require.NoError(t, err)
	require.NotNil(t, authSession)
	require.Equal(t, PlatformNewAPI, authSession.Platform)
	require.Equal(t, "7", authSession.NewAPI.UserID)
	require.NotEmpty(t, authSession.NewAPI.Cookies)

	_, found, err := authChallengeCache.Get(challenge.ChallengeID)
	require.NoError(t, err)
	require.False(t, found)
}

func TestNewAPIPreviewUsesSavedAuthenticatedSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":100}}`))
		case "/api/user/login":
			t.Fatalf("已保存登录态可用时不应重新调用 new-api 登录接口")
		case "/api/user/self":
			require.Equal(t, "7", r.Header.Get("New-Api-User"))
			cookie, err := r.Cookie("session")
			require.NoError(t, err)
			require.Equal(t, "authenticated-session", cookie.Value)
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default","quota":250,"used_quota":50}}`))
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1,"desc":"Default"}}}`))
		case "/api/ratio_config":
			_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{"gpt-4o":2},"completion_ratio":{},"cache_ratio":{},"create_cache_ratio":{},"model_price":{}}}`))
		case "/api/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":11,"name":"key-a","key":"sk-***abcd","group":"default","status":1,"model_limits":"gpt-4o","remain_quota":120,"used_quota":30}],"total":1}}`))
		case "/api/token/batch/keys":
			_, _ = w.Write([]byte(`{"success":true,"data":{"11":"sk-newapi-full-key"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformNewAPI,
		BaseURL:  server.URL,
		Session: &AuthenticatedSession{
			Platform: PlatformNewAPI,
			BaseURL:  server.URL,
			NewAPI: &NewAPISessionData{
				UserID: "7",
				Cookies: []StoredHTTPCookie{
					{Name: "session", Value: "authenticated-session", Path: "/"},
				},
			},
		},
	}})

	require.NoError(t, err)
	require.NotEmpty(t, result.PreviewID)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Equal(t, "sk-new...-key", result.Snapshot.Keys[0].MaskedKey)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, "sk-newapi-full-key", record.Snapshot.Keys[0].Key)
	require.NotNil(t, record.Snapshot.StoredCredential)
	require.Empty(t, record.Snapshot.StoredCredential.Password)
	require.NotEmpty(t, record.Snapshot.StoredCredential.Session)
}

func TestNewAPIPreviewImportsSessionCookieAndAutoDetectsUser(t *testing.T) {
	selfCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":100}}`))
		case "/api/user/login":
			t.Fatalf("导入 Cookie 时不应调用 new-api 账号密码登录接口")
		case "/api/user/self":
			cookie, err := r.Cookie("session")
			require.NoError(t, err)
			require.Equal(t, "imported-session", cookie.Value)
			selfCalls++
			if r.Header.Get("New-Api-User") == "" {
				_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default"}}`))
				return
			}
			require.Equal(t, "7", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default","quota":250,"used_quota":50}}`))
		case "/api/user/self/groups":
			require.Equal(t, "7", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1,"desc":"Default"}}}`))
		case "/api/ratio_config":
			_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{"gpt-4o":2},"completion_ratio":{},"cache_ratio":{},"create_cache_ratio":{},"model_price":{}}}`))
		case "/api/token/":
			require.Equal(t, "7", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":11,"name":"key-a","key":"sk-***abcd","group":"default","status":1,"model_limits":"gpt-4o","remain_quota":120,"used_quota":30}],"total":1}}`))
		case "/api/token/batch/keys":
			require.Equal(t, "7", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"11":"sk-cookie-full-key"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform:      PlatformNewAPI,
		BaseURL:       server.URL,
		AuthMode:      AuthModeSessionCookie,
		SessionCookie: "session=imported-session; theme=dark",
	}})

	require.NoError(t, err)
	require.Equal(t, 2, selfCalls)
	require.NotEmpty(t, result.PreviewID)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Empty(t, result.Snapshot.Keys[0].Key)
	require.Equal(t, "sk-coo...-key", result.Snapshot.Keys[0].MaskedKey)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, "sk-cookie-full-key", record.Snapshot.Keys[0].Key)
	require.NotNil(t, record.Snapshot.StoredCredential)
	require.Equal(t, AuthModeSessionCookie, record.Snapshot.StoredCredential.AuthMode)
	require.Empty(t, record.Snapshot.StoredCredential.Password)
	authSession, err := decryptAuthenticatedSession(record.Snapshot.StoredCredential.Session)
	require.NoError(t, err)
	require.NotNil(t, authSession)
	require.Equal(t, AuthModeSessionCookie, authSession.AuthMode)
	require.Equal(t, "7", authSession.NewAPI.UserID)
	require.Len(t, authSession.NewAPI.Cookies, 2)
}

func TestNewAPIPreviewImportsAccessTokenFromUserscript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/status" && r.URL.Path != "/api/ratio_config" {
			require.Equal(t, "Bearer newapi-access-token", r.Header.Get("Authorization"))
			require.Equal(t, "7", r.Header.Get("New-Api-User"))
		}
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":100}}`))
		case "/api/user/login":
			t.Fatalf("导入 Access Token 时不应调用 new-api 账号密码登录接口")
		case "/api/user/self":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default","quota":250,"used_quota":50}}`))
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1,"desc":"Default"}}}`))
		case "/api/ratio_config":
			_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{"gpt-4o":2},"completion_ratio":{},"cache_ratio":{},"create_cache_ratio":{},"model_price":{}}}`))
		case "/api/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":11,"name":"key-a","key":"sk-***abcd","group":"default","status":1,"model_limits":"gpt-4o","remain_quota":120,"used_quota":30}],"total":1}}`))
		case "/api/token/batch/keys":
			_, _ = w.Write([]byte(`{"success":true,"data":{"11":"sk-token-full-key"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform:    PlatformNewAPI,
		BaseURL:     server.URL,
		AuthMode:    AuthModeAccessToken,
		UserID:      "7",
		AccessToken: "newapi-access-token",
	}})

	require.NoError(t, err)
	require.NotEmpty(t, result.PreviewID)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Equal(t, "sk-tok...-key", result.Snapshot.Keys[0].MaskedKey)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, "sk-token-full-key", record.Snapshot.Keys[0].Key)
	require.NotNil(t, record.Snapshot.StoredCredential)
	require.Equal(t, AuthModeAccessToken, record.Snapshot.StoredCredential.AuthMode)
	authSession, err := decryptAuthenticatedSession(record.Snapshot.StoredCredential.Session)
	require.NoError(t, err)
	require.Equal(t, "7", authSession.NewAPI.UserID)
	require.Equal(t, "newapi-access-token", authSession.NewAPI.AccessToken)
	require.Empty(t, authSession.NewAPI.Cookies)
}

func TestCompletePreview2FARejectsExpiredChallenge(t *testing.T) {
	_, err := CompletePreview2FA(context.Background(), Preview2FARequest{
		ChallengeID: "missing",
		Code:        "123456",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "二次验证会话不存在或已过期")
}

func TestCompletePreview2FARejectsEmptyCodeWithoutConsumingChallenge(t *testing.T) {
	challenge, err := saveAuthChallenge(AuthChallengeRecord{
		Platform: PlatformSub2API,
		BaseURL:  "http://example.test",
		Email:    "alice@example.com",
		Sub2API: &Sub2APIChallengeData{
			TempToken: "temp-token",
		},
	})
	require.NoError(t, err)

	_, err = CompletePreview2FA(context.Background(), Preview2FARequest{
		ChallengeID: challenge.ChallengeID,
		Code:        " ",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "验证码不能为空")
	_, found, err := authChallengeCache.Get(challenge.ChallengeID)
	require.NoError(t, err)
	require.True(t, found)
	require.NoError(t, authChallengeCache.Purge())
}

func TestSub2APIPreviewFetchesKeysRatesAndBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/login" {
			require.Equal(t, "Bearer sub2-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"sub2-token","user":{"id":5,"email":"alice@example.com","balance":10}}}`))
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25,"peak_rate_multiplier":0.5}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.2}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":4.75,"total_cost":5}}`))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-full-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformSub2API,
		BaseURL:  server.URL,
		Email:    "alice@example.com",
		Password: "secret",
	}})

	require.NoError(t, err)
	require.NotEmpty(t, result.PreviewID)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Empty(t, result.Snapshot.Keys[0].Key)
	require.Equal(t, "sk-sub...-key", result.Snapshot.Keys[0].MaskedKey)
	require.Equal(t, float64(12.5), *result.Snapshot.Balance.BalanceUSD)
	require.Equal(t, float64(4.75), *result.Snapshot.Balance.UsedUSD)
	require.Equal(t, float64(0.2), *result.Snapshot.Keys[0].GroupRatio)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, "sk-sub2-full-key", record.Snapshot.Keys[0].Key)
}

func TestSub2APIPreviewFallsBackToKeyUsageWhenAccountUsageIsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"sub2-token","user":{"id":5,"email":"alice@example.com","balance":10}}}`))
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":1}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":1}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-full-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformSub2API,
		BaseURL:  server.URL,
		Email:    "alice@example.com",
		Password: "secret",
	}})

	require.NoError(t, err)
	require.NotNil(t, result.Snapshot)
	require.NotNil(t, result.Snapshot.Balance)
	require.NotNil(t, result.Snapshot.Balance.UsedUSD)
	require.InDelta(t, 3, *result.Snapshot.Balance.UsedUSD, 0.000001)
	require.True(t, result.Snapshot.Balance.Partial)
	require.True(t, result.Snapshot.Balance.MissingUsedValue)
	require.Equal(t, "sub2api:keys", result.Snapshot.Balance.Source)
}

func TestSub2APIPreviewFallsBackToKeyUsageWhenUsageEndpointFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"sub2-token","user":{"id":5,"email":"alice@example.com","balance":10}}}`))
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":1}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":1}}`))
		case "/api/v1/usage/dashboard/stats":
			http.NotFound(w, r)
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-full-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformSub2API,
		BaseURL:  server.URL,
		Email:    "alice@example.com",
		Password: "secret",
	}})

	require.NoError(t, err)
	require.NotNil(t, result.Snapshot)
	require.NotNil(t, result.Snapshot.Balance)
	require.NotNil(t, result.Snapshot.Balance.UsedUSD)
	require.InDelta(t, 3, *result.Snapshot.Balance.UsedUSD, 0.000001)
	require.True(t, result.Snapshot.Balance.Partial)
	require.True(t, result.Snapshot.Balance.MissingUsedValue)
	require.Equal(t, "sub2api:keys", result.Snapshot.Balance.Source)
}

func TestSub2APIPreviewImportsAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/login" {
			require.Equal(t, "Bearer imported-sub2-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/login":
			t.Fatalf("导入 Access Token 时不应调用 sub2api 账号密码登录接口")
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.25}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":4.75}}`))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-full-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform:     PlatformSub2API,
		BaseURL:      server.URL,
		AuthMode:     AuthModeAccessToken,
		AccessToken:  "imported-sub2-token",
		RefreshToken: "imported-refresh-token",
		ExpiresAt:    common.GetTimestamp() + 3600,
	}})

	require.NoError(t, err)
	require.NotEmpty(t, result.PreviewID)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Equal(t, "sk-sub...-key", result.Snapshot.Keys[0].MaskedKey)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, "sk-sub2-full-key", record.Snapshot.Keys[0].Key)
	require.NotNil(t, record.Snapshot.StoredCredential)
	require.Equal(t, AuthModeAccessToken, record.Snapshot.StoredCredential.AuthMode)
	require.Empty(t, record.Snapshot.StoredCredential.Password)
	authSession, err := decryptAuthenticatedSession(record.Snapshot.StoredCredential.Session)
	require.NoError(t, err)
	require.NotNil(t, authSession)
	require.Equal(t, AuthModeAccessToken, authSession.AuthMode)
	require.Equal(t, "imported-sub2-token", authSession.Sub2API.AccessToken)
	require.Equal(t, "imported-refresh-token", authSession.Sub2API.RefreshToken)
}

func TestSub2APIPreviewDiscoversAPIBaseURLFromAppConfig(t *testing.T) {
	apiHits := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHits++
		http.NotFound(w, r)
	}))
	defer apiServer.Close()

	panelPageHits := 0
	panelManagementHits := 0
	panelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/login" {
			panelManagementHits++
			require.Equal(t, "Bearer imported-sub2-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/", "/home":
			panelPageHits++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<script>window.__APP_CONFIG__={"api_base_url":"` + apiServer.URL + `"}</script>`))
		case "/api/v1/auth/login":
			t.Fatalf("导入 Access Token 时不应调用 sub2api 账号密码登录接口")
		case "/api/v1/auth/me":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25}]}`))
		case "/api/v1/groups/rates":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.25}}`))
		case "/api/v1/usage/dashboard/stats":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":4.75}}`))
		case "/api/v1/keys":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-full-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer panelServer.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform:    PlatformSub2API,
		BaseURL:     panelServer.URL + "/home",
		AuthMode:    AuthModeAccessToken,
		AccessToken: "imported-sub2-token",
		ExpiresAt:   common.GetTimestamp() + 3600,
	}})

	require.NoError(t, err)
	require.GreaterOrEqual(t, panelPageHits, 1)
	require.Greater(t, panelManagementHits, 0)
	require.Equal(t, 0, apiHits)
	require.Equal(t, apiServer.URL, result.Snapshot.BaseURL)
	require.Equal(t, panelServer.URL, result.Snapshot.ManagementBaseURL)
	require.Equal(t, apiServer.URL, result.Snapshot.RelayBaseURL)
	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, apiServer.URL, record.Snapshot.BaseURL)
	require.Equal(t, panelServer.URL, record.Snapshot.ManagementBaseURL)
	require.Equal(t, apiServer.URL, record.Snapshot.RelayBaseURL)
	require.NotNil(t, record.Snapshot.StoredCredential)
	require.Equal(t, panelServer.URL, record.Snapshot.StoredCredential.BaseURL)
	require.Equal(t, panelServer.URL, record.Snapshot.StoredCredential.ManagementBaseURL)
	require.Equal(t, apiServer.URL, record.Snapshot.StoredCredential.RelayBaseURL)
}

func TestReadChannelSyncCredentialHydratesSub2APIAccessTokenFromSession(t *testing.T) {
	settings := mergeChannelSyncMetadataWithCredential(
		"",
		&Snapshot{
			Platform: PlatformSub2API,
			BaseURL:  "https://api.sub2api.example",
			AuthSession: &AuthenticatedSession{
				Platform: PlatformSub2API,
				BaseURL:  "https://api.sub2api.example",
				AuthMode: AuthModeAccessToken,
				Sub2API: &Sub2APISessionData{
					AccessToken:  "saved-session-token",
					RefreshToken: "saved-refresh-token",
					ExpiresAt:    common.GetTimestamp() + 3600,
				},
			},
		},
		Credential{
			Platform: PlatformSub2API,
			BaseURL:  "https://api.sub2api.example",
			AuthMode: AuthModeAccessToken,
		},
	)

	credential, ok, err := ReadChannelSyncCredential(settings)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, AuthModeAccessToken, credential.AuthMode)
	require.Equal(t, "saved-session-token", credential.AccessToken)
	require.Equal(t, "saved-refresh-token", credential.RefreshToken)
	require.Greater(t, credential.ExpiresAt, common.GetTimestamp())

	prepared, err := PrepareImportedCredential(credential)
	require.NoError(t, err)
	require.NotNil(t, prepared.Session)
	require.Equal(t, "saved-session-token", prepared.Session.Sub2API.AccessToken)
}

func TestSub2APIPreviewCompletesTwoFAChallenge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/login" && r.URL.Path != "/api/v1/auth/login/2fa" {
			require.Equal(t, "Bearer sub2-2fa-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_, _ = w.Write([]byte(`{"code":0,"data":{"requires_2fa":true,"temp_token":"temp-123"}}`))
		case "/api/v1/auth/login/2fa":
			var body map[string]string
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "temp-123", body["temp_token"])
			require.Equal(t, "654321", body["totp_code"])
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"sub2-2fa-token","refresh_token":"sub2-refresh","expires_in":3600,"user":{"id":5,"email":"alice@example.com","balance":10}}}`))
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.25}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":4.75}}`))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-2fa-full-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	first, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformSub2API,
		BaseURL:  server.URL,
		Email:    "alice@example.com",
		Password: "secret",
	}})
	require.NoError(t, err)
	require.Empty(t, first.PreviewID)
	require.Nil(t, first.Snapshot)
	require.NotNil(t, first.Challenge)
	require.Equal(t, PlatformSub2API, first.Challenge.Platform)
	require.Equal(t, "alice@example.com", first.Challenge.Username)

	result, err := CompletePreview2FA(context.Background(), Preview2FARequest{
		ChallengeID: first.Challenge.ChallengeID,
		Code:        "654321",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.PreviewID)
	require.Nil(t, result.Challenge)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Empty(t, result.Snapshot.Keys[0].Key)
	require.Equal(t, "sk-sub...-key", result.Snapshot.Keys[0].MaskedKey)
	require.Equal(t, float64(12.5), *result.Snapshot.Balance.BalanceUSD)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, "sk-sub2-2fa-full-key", record.Snapshot.Keys[0].Key)
	require.NotNil(t, record.Snapshot.StoredCredential)
	authSession, err := decryptAuthenticatedSession(record.Snapshot.StoredCredential.Session)
	require.NoError(t, err)
	require.NotNil(t, authSession)
	require.Equal(t, PlatformSub2API, authSession.Platform)
	require.Equal(t, "sub2-2fa-token", authSession.Sub2API.AccessToken)
	require.Equal(t, "sub2-refresh", authSession.Sub2API.RefreshToken)
	require.Greater(t, authSession.Sub2API.ExpiresAt, common.GetTimestamp())

	_, found, err := authChallengeCache.Get(first.Challenge.ChallengeID)
	require.NoError(t, err)
	require.False(t, found)
}

func TestSub2APIPreviewRefreshesExpiredImportedToken(t *testing.T) {
	refreshCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/refresh":
			refreshCalls++
			var body map[string]string
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "old-refresh", body["refresh_token"])
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"token_type":"Bearer"}}`))
		case "/api/v1/auth/me":
			require.Equal(t, "Bearer new-access", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			require.Equal(t, "Bearer new-access", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			require.Equal(t, "Bearer new-access", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25}]}`))
		case "/api/v1/groups/rates":
			require.Equal(t, "Bearer new-access", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.25}}`))
		case "/api/v1/usage/dashboard/stats":
			require.Equal(t, "Bearer new-access", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":4.75}}`))
		case "/api/v1/keys":
			require.Equal(t, "Bearer new-access", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-refreshed-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform:     PlatformSub2API,
		BaseURL:      server.URL,
		AuthMode:     AuthModeAccessToken,
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    common.GetTimestamp() - 1,
	}})
	require.NoError(t, err)
	require.Equal(t, 1, refreshCalls)
	require.Len(t, result.Snapshot.Keys, 1)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	authSession, err := decryptAuthenticatedSession(record.Snapshot.StoredCredential.Session)
	require.NoError(t, err)
	require.Equal(t, "new-access", authSession.Sub2API.AccessToken)
	require.Equal(t, "new-refresh", authSession.Sub2API.RefreshToken)
	require.Greater(t, authSession.Sub2API.ExpiresAt, common.GetTimestamp())
}

func TestSub2APIPreviewExpiredImportedTokenWithoutRefreshTokenNeedsRecapture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer server.Close()

	_, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform:    PlatformSub2API,
		BaseURL:     server.URL,
		AuthMode:    AuthModeAccessToken,
		AccessToken: "expired-access",
		ExpiresAt:   common.GetTimestamp() - 1,
	}})

	require.Error(t, err)
	require.Contains(t, err.Error(), "Access Token 已过期")
	require.Contains(t, err.Error(), "重新使用油猴脚本采集")
	require.NotContains(t, err.Error(), "Access Token 不能为空")
}

func TestSub2APIPreviewUnauthorizedAuthMeWithoutRefreshTokenNeedsRecapture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/me":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"message":"unauthorized"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform:    PlatformSub2API,
		BaseURL:     server.URL,
		AuthMode:    AuthModeAccessToken,
		AccessToken: "invalid-access",
		ExpiresAt:   common.GetTimestamp() + 3600,
	}})

	require.Error(t, err)
	require.Contains(t, err.Error(), "Access Token 不可用")
	require.Contains(t, err.Error(), "重新使用油猴脚本采集")
	require.NotContains(t, err.Error(), "Access Token 不能为空")
}

func TestSub2APIPreviewKeysFailureReportsStageInsteadOfTokenFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			require.Equal(t, "Bearer short-lived-access", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.25}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":4.75}}`))
		case "/api/v1/keys":
			http.NotFound(w, r)
			_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform:    PlatformSub2API,
		BaseURL:     server.URL,
		AuthMode:    AuthModeAccessToken,
		AccessToken: "short-lived-access",
		ExpiresAt:   common.GetTimestamp() + 3600,
	}})

	require.Error(t, err)
	require.Contains(t, err.Error(), "读取密钥失败")
	require.Contains(t, err.Error(), "/api/v1/keys")
	require.NotContains(t, err.Error(), "重新使用油猴脚本采集")
	require.NotContains(t, err.Error(), "refresh_token")
	require.NotContains(t, err.Error(), "Access Token 不能为空")
}

func TestSub2APIPreviewAcceptsLoginPageURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/login/") {
			http.Error(w, "<html>login page</html>", http.StatusOK)
			return
		}
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"sub2-token","user":{"id":5,"email":"alice@example.com","balance":10}}}`))
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":1}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":1}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":0}}`))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-full-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Preview(context.Background(), PreviewRequest{Credential: Credential{
		Platform: PlatformSub2API,
		BaseURL:  server.URL + "/login",
		Email:    "alice@example.com",
		Password: "secret",
	}})

	require.NoError(t, err)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Equal(t, "sk-sub...-key", result.Snapshot.Keys[0].MaskedKey)
}

func TestPreviewUsesSavedChannelCredentialWhenChannelIDProvided(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/login" {
			require.Equal(t, "Bearer sub2-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/login":
			var body map[string]string
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "alice@example.com", body["email"])
			require.Equal(t, "secret", body["password"])
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"sub2-token","user":{"id":5,"email":"alice@example.com","balance":10}}}`))
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25,"peak_rate_multiplier":0.5}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.2}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":4.75,"total_cost":5}}`))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-full-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	settings := mergeChannelSyncMetadataWithCredential(
		"",
		&Snapshot{
			Platform: PlatformSub2API,
			BaseURL:  server.URL,
		},
		Credential{
			Platform: PlatformSub2API,
			BaseURL:  server.URL,
			Email:    "alice@example.com",
			Password: "secret",
		},
	)
	channel := model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Key:           constant.ChannelCredentialModeAccountPool,
		Name:          "saved-credential-channel",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: settings,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	result, err := Preview(context.Background(), PreviewRequest{ChannelID: channel.Id})
	require.NoError(t, err)
	require.NotEmpty(t, result.PreviewID)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Equal(t, "sk-sub...-key", result.Snapshot.Keys[0].MaskedKey)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.NotNil(t, record.Snapshot.StoredCredential)
	decryptedPassword, err := common.DecryptSensitiveString(record.Snapshot.StoredCredential.Password)
	require.NoError(t, err)
	require.Equal(t, "secret", decryptedPassword)
}

func TestPreviewUsesSavedChannelAuthenticatedSessionBeforePassword(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/login" {
			require.Equal(t, "Bearer saved-session-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/login":
			t.Fatalf("已保存登录态可用时不应重新调用 sub2api 登录接口")
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25,"peak_rate_multiplier":0.5}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.2}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":4.75,"total_cost":5}}`))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-full-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	settings := mergeChannelSyncMetadataWithCredential(
		"",
		&Snapshot{
			Platform: PlatformSub2API,
			BaseURL:  server.URL,
			AuthSession: &AuthenticatedSession{
				Platform: PlatformSub2API,
				BaseURL:  server.URL,
				Sub2API: &Sub2APISessionData{
					AccessToken:  "saved-session-token",
					RefreshToken: "saved-refresh-token",
					ExpiresAt:    common.GetTimestamp() + 3600,
				},
			},
		},
		Credential{
			Platform: PlatformSub2API,
			BaseURL:  server.URL,
			Email:    "alice@example.com",
			Password: "secret",
		},
	)
	channel := model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Key:           constant.ChannelCredentialModeAccountPool,
		Name:          "saved-session-channel",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: settings,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	credential, ok, err := ReadChannelSyncCredential(channel.OtherSettings)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "secret", credential.Password)
	require.NotNil(t, credential.Session)
	require.Equal(t, "saved-session-token", credential.Session.Sub2API.AccessToken)

	result, err := Preview(context.Background(), PreviewRequest{ChannelID: channel.Id})
	require.NoError(t, err)
	require.NotEmpty(t, result.PreviewID)
	require.Len(t, result.Snapshot.Keys, 1)
	require.Equal(t, "sk-sub...-key", result.Snapshot.Keys[0].MaskedKey)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.NotNil(t, record.Snapshot.StoredCredential)
	authSession, err := decryptAuthenticatedSession(record.Snapshot.StoredCredential.Session)
	require.NoError(t, err)
	require.NotNil(t, authSession)
	require.Equal(t, "saved-session-token", authSession.Sub2API.AccessToken)
}

func TestSub2APIKeyStatusAcceptsStringEnums(t *testing.T) {
	var active sub2APIKey
	require.NoError(t, common.Unmarshal([]byte(`{"status":"active"}`), &active))
	require.Equal(t, common.ChannelStatusEnabled, active.Status.value)

	var inactive sub2APIKey
	require.NoError(t, common.Unmarshal([]byte(`{"status":"quota_exhausted"}`), &inactive))
	require.Equal(t, common.ChannelStatusManuallyDisabled, inactive.Status.value)

	var numeric sub2APIKey
	require.NoError(t, common.Unmarshal([]byte(`{"status":1}`), &numeric))
	require.Equal(t, common.ChannelStatusEnabled, numeric.Status.value)
}

func TestNewAPITokenKeysResponseAcceptsWrappedAndDirectMaps(t *testing.T) {
	var wrapped newAPITokenKeysResponse
	require.NoError(t, common.Unmarshal([]byte(`{"keys":{"50":"sk-wrapped-key"}}`), &wrapped))
	require.Equal(t, "sk-wrapped-key", wrapped.Keys["50"])

	var emptyWrapped newAPITokenKeysResponse
	require.NoError(t, common.Unmarshal([]byte(`{"keys":{}}`), &emptyWrapped))
	require.Empty(t, emptyWrapped.Keys)

	var direct newAPITokenKeysResponse
	require.NoError(t, common.Unmarshal([]byte(`{"51":"sk-direct-key"}`), &direct))
	require.Equal(t, "sk-direct-key", direct.Keys["51"])
}

func TestParseImportedCookiesAcceptsHeaderAndJSONFormats(t *testing.T) {
	headerCookies, err := ParseImportedCookies("session=abc; theme=dark; empty")
	require.NoError(t, err)
	require.Len(t, headerCookies, 2)
	require.Equal(t, "session", headerCookies[0].Name)
	require.Equal(t, "abc", headerCookies[0].Value)

	arrayCookies, err := ParseImportedCookies(`[{"name":"session","value":"abc","path":"/"},{"name":"theme","value":"dark"}]`)
	require.NoError(t, err)
	require.Len(t, arrayCookies, 2)
	require.Equal(t, "/", arrayCookies[1].Path)

	mapCookies, err := ParseImportedCookies(`{"session":"abc","theme":"dark"}`)
	require.NoError(t, err)
	require.Len(t, mapCookies, 2)
}
