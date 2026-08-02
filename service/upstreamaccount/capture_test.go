package upstreamaccount

import (
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/require"
)

func TestCaptureSessionCompletesSub2APIPayload(t *testing.T) {
	start, err := StartCaptureSession(7, CaptureSessionStartRequest{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub.example.com/login",
	}, "https://nexus.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, start.CaptureID)
	require.Equal(t, "https://sub.example.com", start.Origin)

	record, found, err := captureSessionCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, record.Secret)

	_, err = CompleteCaptureSession(start.CaptureID, CaptureSessionCompleteRequest{
		CaptureSecret: record.Secret,
		Origin:        "https://evil.example.com",
		AccessToken:   "sub2-access-token",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "来源不匹配")

	result, err := CompleteCaptureSession(start.CaptureID, CaptureSessionCompleteRequest{
		CaptureSecret: record.Secret,
		CaptureSource: "userscript",
		Origin:        "https://sub.example.com",
		AccessToken:   "sub2-access-token",
		RefreshToken:  "sub-refresh",
		ExpiresIn:     3600,
		AuthUser: map[string]any{
			"email":    "alice@example.com",
			"username": "alice",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.NotNil(t, result.Summary)
	require.Equal(t, "sub2-a...oken", result.Summary.AccessTokenMasked)
	require.True(t, result.Summary.RefreshTokenPresent)
	require.Greater(t, result.Summary.ExpiresAt, common.GetTimestamp())

	credential, err := ResolveCaptureCredential(7, start.CaptureID)
	require.NoError(t, err)
	require.Equal(t, PlatformSub2API, credential.Platform)
	require.Equal(t, AuthModeAccessToken, credential.AuthMode)
	require.NotNil(t, credential.Session)
	require.Equal(t, "sub2-access-token", credential.Session.Sub2API.AccessToken)
	require.Equal(t, "sub-refresh", credential.Session.Sub2API.RefreshToken)

	_, err = CompleteCaptureSession(start.CaptureID, CaptureSessionCompleteRequest{
		CaptureSecret: record.Secret,
		Origin:        "https://sub.example.com",
		AccessToken:   "another-token",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "已完成")

	_, err = ResolveCaptureCredential(8, start.CaptureID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "无权访问")
}

func TestCaptureSessionCompletesNewAPIAccessTokenPayloadAndRendersScript(t *testing.T) {
	start, err := StartCaptureSession(9, CaptureSessionStartRequest{
		Platform: PlatformNewAPI,
		BaseURL:  "https://new.example.com/dashboard",
	}, "https://nexus.example.com")
	require.NoError(t, err)

	script, err := RenderCaptureUserscript(9, start.CaptureID, "https://nexus.example.com")
	require.NoError(t, err)
	require.Contains(t, script, "@match        https://new.example.com/*")
	require.Contains(t, script, "/capture-session/"+start.CaptureID+"/complete")
	require.Contains(t, script, "Send login to NexusTok")

	record, found, err := captureSessionCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)

	_, err = CompleteCaptureSession(start.CaptureID, CaptureSessionCompleteRequest{
		CaptureSecret: record.Secret,
		Origin:        "https://new.example.com",
		AccessToken:   "new-api-access-token",
		AuthUser: map[string]any{
			"id":       17,
			"username": "bob",
		},
	})
	require.NoError(t, err)

	status, err := GetCaptureSessionStatus(9, start.CaptureID)
	require.NoError(t, err)
	require.Equal(t, "completed", status.Status)
	require.Equal(t, "17", status.Summary.UserID)
	require.True(t, strings.HasPrefix(status.Summary.AccessTokenMasked, "new-ap"))

	credential, err := ResolveCaptureCredential(9, start.CaptureID)
	require.NoError(t, err)
	require.NotNil(t, credential.Session.NewAPI)
	require.Equal(t, "17", credential.Session.NewAPI.UserID)
	require.Equal(t, "new-api-access-token", credential.Session.NewAPI.AccessToken)
}

func TestParseCredentialDraftReturnsSanitizedSummary(t *testing.T) {
	result, err := ParseCredentialDraft(CredentialParseRequest{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub.example.com",
		Raw:      `{"auth_token":"sub2-access-token","refresh_token":"sub-refresh","token_expires_at":"1893456000","auth_user":{"username":"alice"}}`,
	})
	require.NoError(t, err)
	require.NotNil(t, result.Summary)
	require.Equal(t, PlatformSub2API, result.Summary.Platform)
	require.Equal(t, "sub2-a...oken", result.Summary.AccessTokenMasked)
	require.True(t, result.Summary.RefreshTokenPresent)
	require.Empty(t, result.Summary.UserID)
}
