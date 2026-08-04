package upstreamaccount

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
	require.Contains(t, start.UserscriptURL, "https://nexus.example.com/api/channel/upstream-account/capture-session/"+start.CaptureID+"/userscript.user.js")
	require.Contains(t, start.UserscriptURL, "install_token=")
	require.Equal(t, "https://sub.example.com", start.LoginURL)

	record, found, err := captureSessionCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, record.Secret)
	require.NotEmpty(t, record.InstallToken)

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

func TestCaptureSessionCompletesSub2APIBrowserSessionRestorePayload(t *testing.T) {
	start, err := StartCaptureSession(17, CaptureSessionStartRequest{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub.example.com",
	}, "https://nexus.example.com")
	require.NoError(t, err)
	record, found, err := captureSessionCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)

	result, err := CompleteCaptureSession(start.CaptureID, CaptureSessionCompleteRequest{
		CaptureSecret: record.Secret,
		CaptureSource: "browser_session_restore",
		Origin:        "https://sub.example.com",
		BaseURL:       "https://sub.example.com",
		APIBaseURL:    "https://api.sub.example.com",
		AccessToken:   "restored-sub2-access-token",
		ExpiresIn:     900,
		AuthUser: map[string]any{
			"email": "linuxdo-user@example.com",
		},
		Diagnostics: &CaptureDiagnostics{
			PageOrigin:                  "https://sub.example.com",
			APIBaseURLSeen:              "https://api.sub.example.com",
			AuthClientIDPresent:         true,
			BrowserSessionRestorePath:   "/api/v1/auth/session/restore",
			BrowserSessionRestoreStatus: "authenticated",
			AuthMePath:                  "/api/v1/auth/me",
		},
	})
	require.NoError(t, err)
	require.Equal(t, captureStatusCompleted, result.Status)
	require.NotNil(t, result.Summary)
	require.Equal(t, "browser_session_restore", result.Summary.CaptureSource)
	require.Equal(t, "https://sub.example.com", result.BaseURL)
	require.Equal(t, "https://sub.example.com", result.ManagementBaseURL)
	require.Equal(t, "https://api.sub.example.com", result.RelayBaseURL)
	require.Equal(t, "https://api.sub.example.com", result.APIBaseURL)
	require.Equal(t, "https://sub.example.com", result.Summary.BaseURL)
	require.Equal(t, "https://sub.example.com", result.Summary.ManagementBaseURL)
	require.Equal(t, "https://api.sub.example.com", result.Summary.RelayBaseURL)
	require.Equal(t, "https://api.sub.example.com", result.Summary.APIBaseURL)
	require.False(t, result.Summary.RefreshTokenPresent)
	require.Greater(t, result.Summary.ExpiresAt, common.GetTimestamp())
	require.NotNil(t, result.Diagnostics)
	require.True(t, result.Diagnostics.AuthClientIDPresent)
	require.Equal(t, "authenticated", result.Diagnostics.BrowserSessionRestoreStatus)

	credential, err := ResolveCaptureCredential(17, start.CaptureID)
	require.NoError(t, err)
	require.NotNil(t, credential.Session)
	require.Equal(t, "https://sub.example.com", credential.BaseURL)
	require.Equal(t, "https://sub.example.com", credential.ManagementBaseURL)
	require.Equal(t, "https://api.sub.example.com", credential.RelayBaseURL)
	require.Equal(t, "https://sub.example.com", credential.Session.BaseURL)
	require.Equal(t, "restored-sub2-access-token", credential.Session.Sub2API.AccessToken)
	require.Empty(t, credential.Session.Sub2API.RefreshToken)
}

func TestCaptureSessionRejectsCrossSiteSub2APIEndpoint(t *testing.T) {
	start, err := StartCaptureSession(18, CaptureSessionStartRequest{
		Platform: PlatformSub2API,
		BaseURL:  "https://panel.example.com",
	}, "https://nexus.example.com")
	require.NoError(t, err)
	record, found, err := captureSessionCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)

	_, err = CompleteCaptureSession(start.CaptureID, CaptureSessionCompleteRequest{
		CaptureSecret: record.Secret,
		CaptureSource: "browser_session_restore",
		Origin:        "https://panel.example.com",
		BaseURL:       "https://api.attacker.com",
		APIBaseURL:    "https://api.attacker.com",
		AccessToken:   "restored-sub2-access-token",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "跨站 API 端点")
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
	require.Contains(t, script, "Username/email values such as c1cada or linuxdo-...")
	require.Contains(t, script, "candidateAPIPrefixes")
	require.Contains(t, script, "readFirstJSON")
	require.Contains(t, script, "normalizeNewAPIUserID")
	require.Contains(t, script, "This page looks like a new-api/NexusTok site")
	require.Contains(t, script, "readSub2APILoginState")
	require.Contains(t, script, "@grant        unsafeWindow")
	require.Contains(t, script, "const pageWindow")
	require.Contains(t, script, "collectSub2APIDiagnostics")
	require.Contains(t, script, "sub2api browser session restore")
	require.Contains(t, script, "auth/session/restore")
	require.Contains(t, script, "X-Sub2API-Auth-Client")
	require.Contains(t, script, "credentials: 'include'")
	require.Contains(t, script, "readSub2APIAuthClientID")
	require.Contains(t, script, "api_base_url")
	require.Contains(t, script, "management_base_url")
	require.Contains(t, script, "relay_base_url")

	record, found, err := captureSessionCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, record.InstallToken)

	_, err = RenderCaptureUserscriptWithInstallToken(start.CaptureID, "", "https://nexus.example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "install_token")

	_, err = RenderCaptureUserscriptWithInstallToken(start.CaptureID, "wrong-token", "https://nexus.example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "无效")

	publicScript, err := RenderCaptureUserscriptWithInstallToken(start.CaptureID, record.InstallToken, "https://nexus.example.com")
	require.NoError(t, err)
	require.Contains(t, publicScript, "// ==UserScript==")

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

	status, err := GetCaptureSessionStatus(9, start.CaptureID, "https://nexus.example.com")
	require.NoError(t, err)
	require.Equal(t, "completed", status.Status)
	parsedInstallURL, err := url.Parse(status.UserscriptURL)
	require.NoError(t, err)
	require.Equal(t, "/api/channel/upstream-account/capture-session/"+start.CaptureID+"/userscript.user.js", parsedInstallURL.Path)
	require.NotEmpty(t, parsedInstallURL.Query().Get("install_token"))
	require.Equal(t, "https://new.example.com/dashboard", status.LoginURL)
	require.Equal(t, "17", status.Summary.UserID)
	require.True(t, strings.HasPrefix(status.Summary.AccessTokenMasked, "new-ap"))

	credential, err := ResolveCaptureCredential(9, start.CaptureID)
	require.NoError(t, err)
	require.NotNil(t, credential.Session.NewAPI)
	require.Equal(t, "17", credential.Session.NewAPI.UserID)
	require.Equal(t, "new-api-access-token", credential.Session.NewAPI.AccessToken)
}

func TestCaptureSessionStoresOnlySafeDiagnostics(t *testing.T) {
	start, err := StartCaptureSession(13, CaptureSessionStartRequest{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub.example.com",
	}, "https://nexus.example.com")
	require.NoError(t, err)
	record, found, err := captureSessionCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)

	const secretToken = "diagnostic-secret-token"
	_, err = CompleteCaptureSession(start.CaptureID, CaptureSessionCompleteRequest{
		CaptureSecret: record.Secret,
		Origin:        "https://sub.example.com",
		Error:         "sub2api access token was not found",
		Diagnostics: &CaptureDiagnostics{
			PageOrigin:                   "https://sub.example.com",
			LocalStorageKeys:             []string{"auth_token", "refresh_token", "theme"},
			SessionStorageKeys:           []string{"temporary"},
			AuthTokenPresent:             false,
			AccessTokenPresent:           false,
			RefreshTokenPresent:          true,
			OAuthHashTokenPresent:        false,
			AuthClientIDPresent:          true,
			AuthMePath:                   "",
			BrowserSessionRestorePath:    "/api/v1/auth/session/restore",
			BrowserSessionRestoreStatus:  "failed",
			BrowserSessionRestoreMessage: "restore failed",
		},
	})
	require.Error(t, err)

	status, err := GetCaptureSessionStatus(13, start.CaptureID, "https://nexus.example.com")
	require.NoError(t, err)
	require.Equal(t, captureStatusFailed, status.Status)
	require.NotNil(t, status.Diagnostics)
	require.Equal(t, []string{"auth_token", "refresh_token", "theme"}, status.Diagnostics.LocalStorageKeys)
	require.True(t, status.Diagnostics.RefreshTokenPresent)
	require.True(t, status.Diagnostics.AuthClientIDPresent)
	require.Equal(t, "failed", status.Diagnostics.BrowserSessionRestoreStatus)
	require.NotContains(t, status.Message, secretToken)

	raw, err := common.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(raw), secretToken)
}

func TestRenderedCaptureUserscriptHasValidJavaScriptSyntax(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("环境没有 Node.js，跳过动态油猴脚本语法检查")
	}

	start, err := StartCaptureSession(14, CaptureSessionStartRequest{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub.example.com",
	}, "https://nexus.example.com")
	require.NoError(t, err)
	record, found, err := captureSessionCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)

	script, err := RenderCaptureUserscriptWithInstallToken(
		start.CaptureID,
		record.InstallToken,
		"https://nexus.example.com",
	)
	require.NoError(t, err)
	scriptPath := filepath.Join(t.TempDir(), "capture.user.js")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o600))

	// 生成脚本最终会直接交给 Tampermonkey；在单测中先用 Node 的语法检查拦截
	// 模板拼接造成的括号、引号或转义错误，避免只能到浏览器里才发现脚本无法安装。
	output, err := exec.Command(nodePath, "--check", scriptPath).CombinedOutput()
	require.NoError(t, err, string(output))
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

func TestParseCredentialDraftAcceptsSub2APIAccessTokenAliases(t *testing.T) {
	result, err := ParseCredentialDraft(CredentialParseRequest{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub.example.com",
		Raw:      `{"local_storage":{"access_token":"sub2-access-token","rt":"sub-refresh","expires_at":"1893456000000"},"auth_user":{"email":"alice@example.com"}}`,
	})
	require.NoError(t, err)
	require.NotNil(t, result.Summary)
	require.Equal(t, "sub2-a...oken", result.Summary.AccessTokenMasked)
	require.True(t, result.Summary.RefreshTokenPresent)
	require.Equal(t, int64(1893456000), result.Summary.ExpiresAt)
}

func TestNewAPICaptureRejectsUsernameAsUserID(t *testing.T) {
	start, err := StartCaptureSession(12, CaptureSessionStartRequest{
		Platform: PlatformNewAPI,
		BaseURL:  "https://new.example.com",
	}, "https://nexus.example.com")
	require.NoError(t, err)
	record, found, err := captureSessionCache.Get(start.CaptureID)
	require.NoError(t, err)
	require.True(t, found)

	_, err = CompleteCaptureSession(start.CaptureID, CaptureSessionCompleteRequest{
		CaptureSecret: record.Secret,
		Origin:        "https://new.example.com",
		UserID:        "linuxdo-323305@linuxdo-connect.invalid",
		AccessToken:   "new-api-access-token",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "数字用户 ID")
}
