// 包 auth - codex_device.go
// 该文件实现了 Codex 的设备认证流程（Device Flow）。
// 当标准 OAuth 浏览器流程不可用时，用户可以通过设备码在外部设备上完成认证。
// 包括设备码请求、轮询等待授权、令牌交换和认证记录构建等功能。
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
	codexLoginModeMetadataKey             = "codex_login_mode"              // 登录模式元数据键
	codexLoginModeDevice                  = "device"                        // 设备认证模式值
	codexDeviceUserCodeURL                = "https://auth.openai.com/api/accounts/deviceauth/usercode" // 设备码请求端点
	codexDeviceTokenURL                   = "https://auth.openai.com/api/accounts/deviceauth/token"    // 设备令牌轮询端点
	codexDeviceVerificationURL            = "https://auth.openai.com/codex/device"                      // 设备验证页面 URL
	codexDeviceTokenExchangeRedirectURI   = "https://auth.openai.com/deviceauth/callback"              // 令牌交换重定向 URI
	codexDeviceTimeout                    = 15 * time.Minute // 设备认证超时时间
	codexDeviceDefaultPollIntervalSeconds = 5                // 默认轮询间隔（秒）
)

// codexDeviceUserCodeRequest 是设备码请求的载荷结构体。
type codexDeviceUserCodeRequest struct {
	ClientID string `json:"client_id"` // OAuth 客户端 ID
}

// codexDeviceUserCodeResponse 是设备码请求的响应结构体。
type codexDeviceUserCodeResponse struct {

// codexDeviceUserCodeResponse 是设备码请求的响应结构体。
type codexDeviceUserCodeResponse struct {
	DeviceAuthID string          `json:"device_auth_id"` // 设备认证 ID
	UserCode     string          `json:"user_code"`      // 用户设备码
	UserCodeAlt  string          `json:"usercode"`       // 用户设备码（备选字段名）
	Interval     json.RawMessage `json:"interval"`       // 轮询间隔（秒，可为字符串或整数）
}

// codexDeviceTokenRequest 是设备令牌轮询请求的载荷结构体。
type codexDeviceTokenRequest struct {
	DeviceAuthID string `json:"device_auth_id"` // 设备认证 ID
	UserCode     string `json:"user_code"`      // 用户设备码
}

// codexDeviceTokenResponse 是设备令牌轮询的响应结构体。
type codexDeviceTokenResponse struct {
	AuthorizationCode string `json:"authorization_code"` // 授权码
	CodeVerifier      string `json:"code_verifier"`      // PKCE 验证码
	CodeChallenge     string `json:"code_challenge"`     // PKCE 挑战码
}

// shouldUseCodexDeviceFlow 检查登录选项是否指定了设备认证模式。
//
// 参数:
//   - opts: 登录选项
//
// 返回:
//   - bool: 如果使用设备认证模式返回 true
func shouldUseCodexDeviceFlow(opts *LoginOptions) bool {
	if opts == nil || opts.Metadata == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(opts.Metadata[codexLoginModeMetadataKey]), codexLoginModeDevice)
}

// loginWithDeviceFlow 执行 Codex 的设备认证流程。
// 用户在浏览器中输入设备码完成认证后，系统自动获取授权码并交换令牌。
//
// 参数:
//   - ctx: 请求上下文
//   - cfg: 应用配置
//   - opts: 登录选项
//
// 返回:
//   - *coreauth.Auth: 认证结果
//   - error: 认证失败时返回错误信息
func (a *CodexAuthenticator) loginWithDeviceFlow(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	httpClient := util.SetProxy(&cfg.SDKConfig, &http.Client{})

	userCodeResp, err := requestCodexDeviceUserCode(ctx, httpClient)
	if err != nil {
		return nil, err
	}

	deviceCode := strings.TrimSpace(userCodeResp.UserCode)
	if deviceCode == "" {
		deviceCode = strings.TrimSpace(userCodeResp.UserCodeAlt)
	}
	deviceAuthID := strings.TrimSpace(userCodeResp.DeviceAuthID)
	if deviceCode == "" || deviceAuthID == "" {
		return nil, fmt.Errorf("codex device flow did not return required fields")
	}

	pollInterval := parseCodexDevicePollInterval(userCodeResp.Interval)

	fmt.Println("Starting Codex device authentication...")
	fmt.Printf("Codex device URL: %s\n", codexDeviceVerificationURL)
	fmt.Printf("Codex device code: %s\n", deviceCode)

	if !opts.NoBrowser {
		if !browser.IsAvailable() {
			log.Warn("No browser available; please open the device URL manually")
		} else if errOpen := browser.OpenURL(codexDeviceVerificationURL); errOpen != nil {
			log.Warnf("Failed to open browser automatically: %v", errOpen)
		}
	}

	tokenResp, err := pollCodexDeviceToken(ctx, httpClient, deviceAuthID, deviceCode, pollInterval)
	if err != nil {
		return nil, err
	}

	authCode := strings.TrimSpace(tokenResp.AuthorizationCode)
	codeVerifier := strings.TrimSpace(tokenResp.CodeVerifier)
	codeChallenge := strings.TrimSpace(tokenResp.CodeChallenge)
	if authCode == "" || codeVerifier == "" || codeChallenge == "" {
		return nil, fmt.Errorf("codex device flow token response missing required fields")
	}

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

	return a.buildAuthRecord(authSvc, authBundle)
}

// requestCodexDeviceUserCode 向 OpenAI 认证服务器请求设备码。
//
// 参数:
//   - ctx: 请求上下文
//   - client: HTTP 客户端
//
// 返回:
//   - *codexDeviceUserCodeResponse: 设备码响应
//   - error: 请求失败时返回错误信息
func requestCodexDeviceUserCode(ctx context.Context, client *http.Client) (*codexDeviceUserCodeResponse, error) {
	body, err := json.Marshal(codexDeviceUserCodeRequest{ClientID: codex.ClientID})
	if err != nil {
		return nil, fmt.Errorf("failed to encode codex device request: %w", err)
	}

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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read codex device code response: %w", err)
	}

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

	var parsed codexDeviceUserCodeResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode codex device code response: %w", err)
	}

	return &parsed, nil
}

// pollCodexDeviceToken 轮询 OpenAI 认证服务器等待用户完成设备码认证。
// 在超时前持续轮询，直到获得授权码或认证失败。
//
// 参数:
//   - ctx: 请求上下文
//   - client: HTTP 客户端
//   - deviceAuthID: 设备认证 ID
//   - userCode: 用户设备码
//   - interval: 轮询间隔
//
// 返回:
//   - *codexDeviceTokenResponse: 令牌响应（包含授权码和 PKCE 码）
//   - error: 轮询超时或失败时返回错误信息
func pollCodexDeviceToken(ctx context.Context, client *http.Client, deviceAuthID, userCode string, interval time.Duration) (*codexDeviceTokenResponse, error) {
	deadline := time.Now().Add(codexDeviceTimeout)

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("codex device authentication timed out after 15 minutes")
		}

		body, err := json.Marshal(codexDeviceTokenRequest{
			DeviceAuthID: deviceAuthID,
			UserCode:     userCode,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to encode codex device poll request: %w", err)
		}

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

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read codex device poll response: %w", readErr)
		}

		switch {
		case codexDeviceIsSuccessStatus(resp.StatusCode):
			var parsed codexDeviceTokenResponse
			if err := json.Unmarshal(respBody, &parsed); err != nil {
				return nil, fmt.Errorf("failed to decode codex device token response: %w", err)
			}
			return &parsed, nil
		case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound:
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
				continue
			}
		default:
			trimmed := strings.TrimSpace(string(respBody))
			if trimmed == "" {
				trimmed = "empty response body"
			}
			return nil, fmt.Errorf("codex device token polling failed with status %d: %s", resp.StatusCode, trimmed)
		}
	}
}

// parseCodexDevicePollInterval 解析服务器返回的轮询间隔。
// 支持字符串和整数两种格式，解析失败时返回默认值（5 秒）。
//
// 参数:
//   - raw: 原始 JSON 值
//
// 返回:
//   - time.Duration: 解析后的轮询间隔
func parseCodexDevicePollInterval(raw json.RawMessage) time.Duration {
	defaultInterval := time.Duration(codexDeviceDefaultPollIntervalSeconds) * time.Second
	if len(raw) == 0 {
		return defaultInterval
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if seconds, convErr := strconv.Atoi(strings.TrimSpace(asString)); convErr == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}

	var asInt int
	if err := json.Unmarshal(raw, &asInt); err == nil && asInt > 0 {
		return time.Duration(asInt) * time.Second
	}

	return defaultInterval
}

// codexDeviceIsSuccessStatus 检查 HTTP 状态码是否表示成功（2xx）。
//
// 参数:
//   - code: HTTP 状态码
//
// 返回:
//   - bool: 如果是 2xx 状态码返回 true
func codexDeviceIsSuccessStatus(code int) bool {
	return code >= 200 && code < 300
}

// buildAuthRecord 根据认证服务结果构建认证记录。
// 从 JWT ID 令牌中提取计划类型和账户 ID，并生成文件名。
//
// 参数:
//   - authSvc: Codex 认证服务
//   - authBundle: 认证令牌包
//
// 返回:
//   - *coreauth.Auth: 认证记录
//   - error: 构建失败时返回错误信息
func (a *CodexAuthenticator) buildAuthRecord(authSvc *codex.CodexAuth, authBundle *codex.CodexAuthBundle) (*coreauth.Auth, error) {
	tokenStorage := authSvc.CreateTokenStorage(authBundle)

	if tokenStorage == nil || tokenStorage.Email == "" {
		return nil, fmt.Errorf("codex token storage missing account information")
	}

	planType := ""
	hashAccountID := ""
	if tokenStorage.IDToken != "" {
		if claims, errParse := codex.ParseJWTToken(tokenStorage.IDToken); errParse == nil && claims != nil {
			planType = strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType)
			accountID := strings.TrimSpace(claims.CodexAuthInfo.ChatgptAccountID)
			if accountID != "" {
				digest := sha256.Sum256([]byte(accountID))
				hashAccountID = hex.EncodeToString(digest[:])[:8]
			}
		}
	}

	fileName := codex.CredentialFileName(tokenStorage.Email, planType, hashAccountID, true)
	metadata := map[string]any{
		"email": tokenStorage.Email,
	}

	fmt.Println("Codex authentication successful")
	if authBundle.APIKey != "" {
		fmt.Println("Codex API key obtained and stored")
	}

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
