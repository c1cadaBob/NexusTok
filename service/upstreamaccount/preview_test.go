package upstreamaccount

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
	require.Empty(t, result.Snapshot.Keys[0].Key)
	require.Equal(t, "sk-new...-key", result.Snapshot.Keys[0].MaskedKey)
	require.Equal(t, "vip", result.Snapshot.Keys[0].GroupName)
	require.Equal(t, float64(2.5), *result.Snapshot.Balance.BalanceUSD)
	require.Equal(t, float64(0.5), *result.Snapshot.Balance.UsedUSD)

	record, err := GetPreviewRecord(result.PreviewID)
	require.NoError(t, err)
	require.Equal(t, "sk-newapi-full-key", record.Snapshot.Keys[0].Key)
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
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-full-key","status":1,"group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
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
