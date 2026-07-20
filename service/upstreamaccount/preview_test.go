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
	"github.com/stretchr/testify/require"
)

func TestNewAPIPreviewFetchesKeysRatesAndBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":100}}`))
		case "/api/user/login":
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

	_, found, err := authChallengeCache.Get(challenge.ChallengeID)
	require.NoError(t, err)
	require.False(t, found)
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
		if r.URL.Path != "/api/v1/auth/login" {
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

func TestSub2APIPreviewCompletesTwoFAChallenge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/login" && r.URL.Path != "/api/v1/auth/login/2fa" {
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
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"sub2-2fa-token","user":{"id":5,"email":"alice@example.com","balance":10}}}`))
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

	_, found, err := authChallengeCache.Get(first.Challenge.ChallengeID)
	require.NoError(t, err)
	require.False(t, found)
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
