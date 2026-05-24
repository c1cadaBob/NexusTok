// auth - codex_device.go
// 本文件实现了 OpenAI Codex 的设备流（Device Flow）认证模式。
// 设备流适用于无法直接进行 OAuth 回调的场景（如无浏览器的服务器环境），
// 用户通过访问验证 URL 并输入设备码来完成授权，系统通过轮询获取令牌。
package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/codex"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/browser"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

const (
	// codexLoginModeMetadataKey 是 LoginOptions.Metadata 中用于指定登录模式的键名。
	codexLoginModeMetadataKey = "codex_loginMode"
	// codexLoginModeDevice 是设备流登录模式的值。
	codexLoginModeDevice = "device"
	// codexDeviceUserCodeURL 是请求设备码的 API 端点。
	codexDeviceUserCodeURL = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	// codexDeviceTokenURL 是轮询设备令牌的 API 端点。
	codexDeviceTokenURL = "https://auth.openai.com/api/accounts/deviceauth/token"
	// codexDeviceVerificationURL 是用户输入设备码的验证页面 URL。
	codexDeviceVerificationURL = "https://auth.openai.com/codex/device"
	// codexDeviceTokenExchangeRedirectURI 是设备流令牌交换使用的重定向 URI。
	codexDeviceTokenExchangeRedirectURI = "https://auth.openai.com/deviceauth/callback"
	// codexDeviceTimeout 是设备流认证的总超时时间（15 分钟）。
	codexDeviceTimeout = 15 * time.Minute
	// codexDeviceDefaultPollIntervalSeconds 是轮询设备令牌的默认间隔（秒）。
	codexDeviceDefaultPollIntervalSeconds = 5
)

// codexDeviceUserCodeRequest 是请求设备码的请求体结构。
type codexDeviceUserCodeRequest struct {
	// ClientID 是 OAuth 客户端标识符。
	ClientID string `json:"client_id"`
}

// codexDeviceUserCodeResponse 是请求设备码的响应体结构。
type codexDeviceUserCodeResponse struct {
	// DeviceAuthID 是设备认证会话的唯一标识符，用于后续轮询。
	DeviceAuthID string `json:"device_auth_id"`
	// UserCode 是用户需要在验证页面输入的短码。
	UserCode string `json:"user_code"`
	// UserCodeAlt 是备选的用户码字段（兼容不同 API 版本）。
	UserCodeAlt string `json:"usercode"`
	// Interval 是服务器建议的轮询间隔（秒），可以是字符串或整数。
	Interval json.RawMessage `json:"interval"`
}

// codexDeviceTokenRequest 是轮询设备令牌的请求体结构。
type codexDeviceTokenRequest struct {
	// DeviceAuthID 是设备认证会话的标识符。
	DeviceAuthID string `json:"device_auth_id"`
	// UserCode 是用户码。
	UserCode string `json:"user_code"`
}

// codexDeviceTokenResponse 是轮询设备令牌的响应体结构。
// 当用户完成授权后，服务器返回授权码和 PKCE 参数。
type codexDeviceTokenResponse struct {
	// AuthorizationCode 是 OAuth 授权码，用于交换访问令牌。
	AuthorizationCode string `json:"authorization_code"`
	// CodeVerifier 是 PKCE 验证码。
	CodeVerifier string `json:"code_verifier"`
	// CodeChallenge 是 PKCE 挑战码。
	CodeChallenge string `json:"code_challenge"`
}

// shouldUseCodexDeviceFlow 检查登录选项是否指定了设备流模式。
// 当 opts.Metadata["codex_login_mode"] 的值为 "device"（不区分大小写）时返回 true。
func shouldUseCodexDeviceFlow(opts *LoginOptions) bool {
	if opts == nil || opts.Metadata == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(opts.Metadata[codexLoginModeMetadataKey]), codexLoginModeDevice)
}

// loginWithDeviceFlow 执行 Codex 设备流认证模式的完整流程。
// 流程概述：
//  1. 向 OpenAI 服务器请求设备码和用户码
//  2. 显示验证 URL 和用户码，提示用户在浏览器中完成授权
//  3. 轮询服务器等待用户完成授权
//  4. 获取授权码和 PKCE 参数后交换令牌
//  5. 构建并返回认证记录
func (a *CodexAuthenticator) loginWithDeviceFlow(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// 创建带代理配置的 HTTP 客户端
	httpClient := util.SetProxy(&cfg.SDKConfig, &http.Client{})

	// 请求设备码
	userCodeResp, err := requestCodexDeviceUserCode(ctx, httpClient)
	if err != nil {
		return nil, err
	}

	// 提取用户码（优先使用 UserCode，备选 UserCodeAlt）
	deviceCode := strings.TrimSpace(userCodeResp.UserCode)
	if deviceCode == "" {
		deviceCode = strings.TrimSpace(userCodeResp.UserCodeAlt)
	}
	deviceAuthID := strings.TrimSpace(userCodeResp.DeviceAuthID)
	if deviceCode == "" || deviceAuthID == "" {
		return nil, fmt.Errorf("codex device flow did not return required fields")
	}

	// 解析服务器建议的轮询间隔
	pollInterval := parseCodexDevicePollInterval(userCodeResp.Interval)

	// 显示设备码和验证 URL
	fmt.Println("Starting Codex device authentication...")
	fmt.Printf("Codex device URL: %s\n", codexDeviceVerificationURL)
	fmt.Printf("Codex device code: %s\n", deviceCode)

	// 尝试自动打开浏览器
	if !opts.NoBrowser {
		if !browser.IsAvailable() {
			log.Warn("No browser available; please open the device URL manually")
		} else if errOpen := browser.OpenURL(codexDeviceVerificationURL); errOpen != nil {
			log.Warnf("Failed to open browser automatically: %v", errOpen)
		}
	}

	// 轮询等待用户完成授权
	tokenResp, err := pollCodexDeviceToken(ctx, httpClient, deviceAuthID, deviceCode, pollInterval)
	if err != nil {
		return nil, err
	}

	// 提取授权码和 PKCE 参数
	authCode := strings.TrimSpace(tokenResp.AuthorizationCode)
	codeVerifier := strings.TrimSpace(tokenResp.CodeVerifier)
	codeChallenge := strings.TrimSpace(tokenResp.CodeChallenge)
	if authCode == "" || codeVerifier == "" || codeChallenge == "" {
		return nil, fmt.Errorf("codex device flow token response missing required fields")
	}

	// 使用授权码交换令牌
	authSvc := codex.NewCodexAuth(cfg)
	authBundle, err := authSvc.ExchangeCodeForTokensWithRedirect(
		ctx,
		authCode,
		codexDeviceTokenExchangeRedirectURI,
		&codex.PKCECodes{
			CodeVerifier:  codeVerifier,
			CodeChallenge: codeChallenge,
		},
	)
	if err != nil {
		return nil, codex.NewAuthenticationError(codex.ErrCodeExchangeFailed, err)
	}

	// 构建并返回认证记录
	return a.buildAuthRecord(authSvc, authBundle)
}

// requestCodexDeviceUserCode 向 OpenAI 服务器请求设备码和用户码。
// 返回的响应包含设备认证 ID、用户码和轮询间隔。
func requestCodexDeviceUserCode(ctx context.Context, client *http.Client) (*codexDeviceUserCodeResponse, error) {
	// 构建请求体
	body, err := json.Marshal(codexDeviceUserCodeRequest{ClientID: codex.ClientID})
	if err != nil {
		return nil, fmt.Errorf("failed to encode codex device request: %w", err)
	}

	// 发送 POST 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexDeviceUserCodeURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create codex device request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request codex device code: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read codex device code response: %w", err)
	}

	// 检查响应状态码
	if !codexDeviceIsSuccessStatus(resp.StatusCode) {
		trimmed := strings.TrimSpace(string(respBody))
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("codex device endpoint is unavailable (status %d)", resp.StatusCode)
		}
		if trimmed == "" {
			trimmed = "empty response body"
		}
		return nil, fmt.Errorf("codex device code request failed with status %d: %s", resp.StatusCode, trimmed)
	}

	// 解析响应 JSON
	var parsed codexDeviceUserCodeResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode codex device code response: %w", err)
	}

	return &parsed, nil
}

// pollCodexDeviceToken 轮询 OpenAI 服务器等待用户完成设备授权。
// 当用户在验证页面输入设备码并授权后，服务器返回授权码和 PKCE 参数。
// 轮询在超时（15 分钟）或上下文取消时终止。
// 参数说明：
//   - ctx: 上下文，用于控制取消
//   - client: HTTP 客户端
//   - deviceAuthID: 设备认证会话标识符
//   - userCode: 用户码
//   - interval: 轮询间隔
func pollCodexDeviceToken(ctx context.Context, client *http.Client, deviceAuthID, userCode string, interval time.Duration) (*codexDeviceTokenResponse, error) {
	deadline := time.Now().Add(codexDeviceTimeout)

	for {
		// 检查是否超时
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("codex device authentication timed out after 15 minutes")
		}

		// 构建轮询请求体
		body, err := json.Marshal(codexDeviceTokenRequest{
			DeviceAuthID: deviceAuthID,
			UserCode:     userCode,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to encode codex device poll request: %w", err)
		}

		// 发送轮询请求
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexDeviceTokenURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create codex device poll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to poll codex device token: %w", err)
		}

		// 读取响应体
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read codex device poll response: %w", readErr)
		}

		switch {
		case codexDeviceIsSuccessStatus(resp.StatusCode):
			// 成功：解析并返回令牌响应
			var parsed codexDeviceTokenResponse
			if err := json.Unmarshal(respBody, &parsed); err != nil {
				return nil, fmt.Errorf("failed to decode codex device token response: %w", err)
			}
			return &parsed, nil
		case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound:
			// 用户尚未授权：等待后重试
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
				continue
			}
		default:
			// 其他错误状态码：返回错误
			trimmed := strings.TrimSpace(string(respBody))
			if trimmed == "" {
				trimmed = "empty response body"
			}
			return nil, fmt.Errorf("codex device token polling failed with status %d: %s", resp.StatusCode, trimmed)
		}
	}
}

// parseCodexDevicePollInterval 从原始 JSON 消息中解析轮询间隔。
// 支持字符串和整数两种格式，解析失败时返回默认间隔（5 秒）。
func parseCodexDevicePollInterval(raw json.RawMessage) time.Duration {
	defaultInterval := time.Duration(codexDeviceDefaultPollIntervalSeconds) * time.Second
	if len(raw) == 0 {
		return defaultInterval
	}

	// 尝试作为字符串解析
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if seconds, convErr := strconv.Atoi(strings.TrimSpace(asString)); convErr == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}

	// 尝试作为整数解析
	var asInt int
	if err := json.Unmarshal(raw, &asInt); err == nil && asInt > 0 {
		return time.Duration(asInt) * time.Second
	}

	return defaultInterval
}

// codexDeviceIsSuccessStatus 判断 HTTP 状态码是否表示成功（2xx）。
func codexDeviceIsSuccessStatus(code int) bool {
	return code >= 200 && code < 300
}

// buildAuthRecord 从认证包构建认证记录。
// 该方法解析 ID 令牌以提取计划类型和账号 ID，然后构建文件名和元数据。
// 参数说明：
//   - authSvc: Codex 认证服务实例
//   - authBundle: 令牌交换返回的认证包
func (a *CodexAuthenticator) buildAuthRecord(authSvc *codex.CodexAuth, authBundle *codex.CodexAuthBundle) (*coreauth.Auth, error) {
	// 从认证包创建令牌存储对象
	tokenStorage := authSvc.CreateTokenStorage(authBundle)

	// 验证令牌存储包含必要的账号信息
	if tokenStorage == nil || tokenStorage.Email == "" {
		return nil, fmt.Errorf("codex token storage missing account information")
	}

	// 从 ID 令牌中解析计划类型和账号 ID
	planType := ""
	hashAccountID := ""
	if tokenStorage.IDToken != "" {
		if claims, errParse := codex.ParseJWTToken(tokenStorage.IDToken); errParse == nil && claims != nil {
			planType = strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType)
			accountID := strings.TrimSpace(claims.CodexAuthInfo.ChatgptAccountID)
			if accountID != "" {
				// 对账号 ID 进行 SHA-256 哈希并取前 8 个字符作为短标识
				digest := sha256.Sum256([]byte(accountID))
				hashAccountID = hex.EncodeToString(digest[:])[:8]
			}
		}
	}

	// 构建认证文件名
	fileName := codex.CredentialFileName(tokenStorage.Email, planType, hashAccountID, true)
	metadata := map[string]any{
		"email": tokenStorage.Email,
	}

	fmt.Println("Codex authentication successful")
	if authBundle.APIKey != "" {
		fmt.Println("Codex API key obtained and stored")
	}

	// 返回认证记录
	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Storage:  tokenStorage,
		Metadata: metadata,
		Attributes: map[string]string{
			"plan_type": planType,
		},
	}, nil
}
