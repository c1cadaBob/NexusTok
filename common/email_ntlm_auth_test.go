package common

import (
	"bytes"
	"net/smtp"
	"testing"

	"github.com/stretchr/testify/require"
)

// resetSMTPAuthTestGlobals 重置会影响认证选择的全局 SMTP 配置，并在测试结束后恢复。
func resetSMTPAuthTestGlobals(t *testing.T) {
	t.Helper()

	originalServer := SMTPServer
	originalAccount := SMTPAccount
	originalToken := SMTPToken
	originalForceLogin := SMTPForceAuthLogin
	originalLoginServers := append([]string(nil), EmailLoginAuthServerList...)

	SMTPServer = "smtp.example.com"
	SMTPAccount = "sender@example.com"
	SMTPToken = "secret"
	SMTPForceAuthLogin = false
	EmailLoginAuthServerList = []string{}

	t.Cleanup(func() {
		SMTPServer = originalServer
		SMTPAccount = originalAccount
		SMTPToken = originalToken
		SMTPForceAuthLogin = originalForceLogin
		EmailLoginAuthServerList = originalLoginServers
	})
}

// smtpServerInfo 构造带 TLS 标记的 SMTP 服务器能力信息，避免 PLAIN 测试被明文连接保护拦截。
func smtpServerInfo(auth ...string) *smtp.ServerInfo {
	return &smtp.ServerInfo{
		Name: SMTPServer,
		TLS:  true,
		Auth: auth,
	}
}

// TestAutoSMTPAuthUsesPlainWhenServerSupportsPlain 验证标准 SMTP 服务器仍优先使用 PLAIN。
func TestAutoSMTPAuthUsesPlainWhenServerSupportsPlain(t *testing.T) {
	resetSMTPAuthTestGlobals(t)

	mech, initial, err := getSMTPAuth().Start(smtpServerInfo("PLAIN", "LOGIN"))

	require.NoError(t, err)
	require.Equal(t, "PLAIN", mech)
	require.Equal(t, []byte("\x00sender@example.com\x00secret"), initial)
}

// TestAutoSMTPAuthUsesLoginWhenForced 验证管理员显式强制 LOGIN 时不会被自动探测覆盖。
func TestAutoSMTPAuthUsesLoginWhenForced(t *testing.T) {
	resetSMTPAuthTestGlobals(t)
	SMTPForceAuthLogin = true

	auth := getSMTPAuth()
	mech, initial, err := auth.Start(smtpServerInfo("PLAIN", "NTLM"))

	require.NoError(t, err)
	require.Equal(t, "LOGIN", mech)
	require.Empty(t, initial)

	username, err := auth.Next([]byte("Username:"), true)
	require.NoError(t, err)
	require.Equal(t, []byte("sender@example.com"), username)
}

// TestAutoSMTPAuthUsesLoginForLegacyLoginServer 验证历史 LOGIN 白名单服务器继续使用 LOGIN。
func TestAutoSMTPAuthUsesLoginForLegacyLoginServer(t *testing.T) {
	resetSMTPAuthTestGlobals(t)
	EmailLoginAuthServerList = []string{SMTPServer}

	mech, initial, err := getSMTPAuth().Start(smtpServerInfo("PLAIN", "LOGIN"))

	require.NoError(t, err)
	require.Equal(t, "LOGIN", mech)
	require.Empty(t, initial)
}

// TestAutoSMTPAuthUsesNTLMWhenServerOnlySupportsNTLM 验证只开放 NTLM 的企业 SMTP 可进入 NTLM 协商。
func TestAutoSMTPAuthUsesNTLMWhenServerOnlySupportsNTLM(t *testing.T) {
	resetSMTPAuthTestGlobals(t)

	mech, initial, err := getSMTPAuth().Start(smtpServerInfo("NTLM"))

	require.NoError(t, err)
	require.Equal(t, "NTLM", mech)
	require.True(t, bytes.HasPrefix(initial, []byte("NTLMSSP")), "unexpected NTLM negotiate message: %q", initial)
}

// TestAutoSMTPAuthUsesNTLMForMicrosoftAccountWhenOnlyNTLMIsAdvertised 验证 onmicrosoft 账号在服务器仅声明 NTLM 时不会误走 LOGIN。
func TestAutoSMTPAuthUsesNTLMForMicrosoftAccountWhenOnlyNTLMIsAdvertised(t *testing.T) {
	resetSMTPAuthTestGlobals(t)
	SMTPAccount = "sender@contoso.onmicrosoft.com"

	mech, initial, err := getSMTPAuth().Start(smtpServerInfo("NTLM"))

	require.NoError(t, err)
	require.Equal(t, "NTLM", mech)
	require.True(t, bytes.HasPrefix(initial, []byte("NTLMSSP")), "unexpected NTLM negotiate message: %q", initial)
}
