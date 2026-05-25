// Package controller - account_pool_proxy_test.go
// 该文件覆盖 CPAMC 管理代理中的主 Relay 接管逻辑。
//
// 测试重点：
// - 不带 authIndex 的模型请求会被识别为可交给 NexusTok 主 Relay；
// - 带 authIndex 的官方账号运行时请求继续交给 CLIProxyAPI Sidecar；
// - 上游凭据类请求头不会进入主 Relay，避免覆盖 NexusTok 渠道凭证。
package controller

import (
	"net/http"
	"testing"

	"github.com/c1cada/NexusTok/types"
	"github.com/stretchr/testify/require"
)

func TestResolveAccountPoolMainRelayTargetChatCompletions(t *testing.T) {
	payload := &accountPoolAPICallRequest{
		Method: "post",
		URL:    "https://api.openai.com/v1/chat/completions?key=upstream-secret&trace=1",
		Data:   `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`,
	}

	target, ok := resolveAccountPoolMainRelayTarget(payload)

	require.True(t, ok)
	require.Equal(t, http.MethodPost, target.Method)
	require.Equal(t, "/v1/chat/completions", target.Path)
	require.Equal(t, "trace=1", target.RawQuery)
	require.Equal(t, types.RelayFormatOpenAI, target.RelayFormat)
}

func TestResolveAccountPoolMainRelayTargetSkipsCredentialBoundRequest(t *testing.T) {
	authIndex := "codex-auth-1"
	payload := &accountPoolAPICallRequest{
		AuthIndexCamel: &authIndex,
		Method:         "POST",
		URL:            "https://api.openai.com/v1/chat/completions",
		Data:           `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`,
	}

	_, ok := resolveAccountPoolMainRelayTarget(payload)

	require.False(t, ok)
}

func TestCopyAccountPoolMainRelayHeadersDropsUpstreamCredentials(t *testing.T) {
	headers := http.Header{}

	copyAccountPoolMainRelayHeaders(headers, map[string]string{
		"Authorization":     "Bearer upstream",
		"X-Api-Key":         "claude-key",
		"X-Goog-Api-Key":    "gemini-key",
		"Content-Type":      "application/json",
		"Anthropic-Version": "2023-06-01",
	})

	require.Empty(t, headers.Get("Authorization"))
	require.Empty(t, headers.Get("X-Api-Key"))
	require.Empty(t, headers.Get("X-Goog-Api-Key"))
	require.Equal(t, "application/json", headers.Get("Content-Type"))
	require.Equal(t, "2023-06-01", headers.Get("Anthropic-Version"))
	require.Equal(t, "true", headers.Get("X-NexusTok-CPAMC-Main-Relay"))
}
