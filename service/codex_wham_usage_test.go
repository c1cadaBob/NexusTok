package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"

	"github.com/stretchr/testify/require"
)

func TestFetchCodexWhamUsageBuildsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/backend-api/wham/usage", r.URL.Path)
		require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		require.Equal(t, "account-1", r.Header.Get("chatgpt-account-id"))
		require.Equal(t, "codex_cli_rs", r.Header.Get("originator"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	statusCode, body, err := FetchCodexWhamUsage(context.Background(), server.Client(), server.URL+"/", " access-token ", " account-1 ")

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.JSONEq(t, `{"ok":true}`, string(body))
}

func TestFetchCodexWhamRateLimitResetCreditsBuildsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/backend-api/wham/rate-limit-reset-credits", r.URL.Path)
		require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		require.Equal(t, "account-1", r.Header.Get("chatgpt-account-id"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"credits":2}`))
	}))
	defer server.Close()

	statusCode, body, err := FetchCodexWhamRateLimitResetCredits(context.Background(), server.Client(), server.URL, "access-token", "account-1")

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.JSONEq(t, `{"credits":2}`, string(body))
}

func TestConsumeCodexWhamRateLimitResetCreditBuildsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/backend-api/wham/rate-limit-reset-credits/consume", r.URL.Path)
		require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		require.Equal(t, "account-1", r.Header.Get("chatgpt-account-id"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var payload map[string]string
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		require.NotEmpty(t, payload["redeem_request_id"])

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"reset":true}`))
	}))
	defer server.Close()

	statusCode, body, err := ConsumeCodexWhamRateLimitResetCredit(context.Background(), server.Client(), server.URL, "access-token", "account-1")

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.JSONEq(t, `{"reset":true}`, string(body))
}

func TestFetchCodexWhamRejectsMissingClient(t *testing.T) {
	_, _, err := FetchCodexWhamRateLimitResetCredits(context.Background(), nil, "https://example.com", "token", "account")
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil http client")
}
