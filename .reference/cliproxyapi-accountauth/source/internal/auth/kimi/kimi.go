// kimi - kimi.go
// 包 kimi 提供 Kimi（Moonshot AI）API 的认证和令牌管理功能。
// 该文件实现了 RFC 8628 OAuth2 设备授权授权流程（Device Authorization Grant），
// 用于安全认证。包含设备代码请求、轮询令牌交换和刷新令牌等功能。
package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	log "github.com/sirupsen/logrus"
)

const (
	// kimiClientID 是 Kimi Code 的 OAuth 客户端 ID
	kimiClientID = "17e5f671-d194-4dfb-9706-5516cb48c098"
	// kimiOAuthHost 是 OAuth 服务器端点
	kimiOAuthHost = "https://auth.kimi.com"
	// kimiDeviceCodeURL 是请求设备代码的端点
	kimiDeviceCodeURL = kimiOAuthHost + "/api/oauth/device_authorization"
	// kimiTokenURL 是用设备代码交换令牌的端点
	kimiTokenURL = kimiOAuthHost + "/api/oauth/token"
	// KimiAPIBaseURL 是 Kimi API 请求的基础 URL
	KimiAPIBaseURL = "https://api.kimi.com/coding"
	// defaultPollInterval 是轮询令牌端点的默认间隔
	defaultPollInterval = 5 * time.Second
	// maxPollDuration 是等待用户授权的最大时间
	maxPollDuration = 15 * time.Minute
	// refreshThresholdSeconds 是令牌过期前刷新的阈值（5 分钟）
	refreshThresholdSeconds = 300
)

// KimiAuth 处理 Kimi 认证流程。
type KimiAuth struct {
	// deviceClient 是设备流程客户端
	deviceClient *DeviceFlowClient
	// cfg 是应用程序配置
	cfg *config.Config
}

// NewKimiAuth 创建一个新的 KimiAuth 服务实例。
//
// 参数：
//   - cfg: 应用程序配置，包含代理设置
//
// 返回：
//   - *KimiAuth: 新的 KimiAuth 服务实例
func NewKimiAuth(cfg *config.Config) *KimiAuth {
	return &KimiAuth{
		deviceClient: NewDeviceFlowClient(cfg),
		cfg:          cfg,
	}
}

// StartDeviceFlow 启动设备流程认证。
// 向 Kimi 服务器请求设备代码，返回设备代码响应供用户完成授权。
//
// 参数：
//   - ctx: 请求的上下文
//
// 返回：
//   - *DeviceCodeResponse: 设备代码响应，包含用户需要输入的代码
//   - error: 请求失败时返回的错误
func (k *KimiAuth) StartDeviceFlow(ctx context.Context) (*DeviceCodeResponse, error) {
	return k.deviceClient.RequestDeviceCode(ctx)
}

// WaitForAuthorization 轮询等待用户授权并返回认证包。
// 持续轮询令牌端点，直到用户完成授权或设备代码过期。
//
// 参数：
//   - ctx: 请求的上下文
//   - deviceCode: 设备代码响应
//
// 返回：
//   - *KimiAuthBundle: 认证包，包含令牌数据和设备 ID
//   - error: 授权失败时返回的错误
func (k *KimiAuth) WaitForAuthorization(ctx context.Context, deviceCode *DeviceCodeResponse) (*KimiAuthBundle, error) {
	tokenData, err := k.deviceClient.PollForToken(ctx, deviceCode)
	if err != nil {
		return nil, err
	}

	return &KimiAuthBundle{
		TokenData: tokenData,
		DeviceID:  k.deviceClient.deviceID,
	}, nil
}

// CreateTokenStorage 从认证包创建 KimiTokenStorage。
// 将认证包中的令牌数据转换为适合持久化存储的结构。
//
// 参数：
//   - bundle: 包含令牌数据的认证包
//
// 返回：
//   - *KimiTokenStorage: 新的令牌存储实例
func (k *KimiAuth) CreateTokenStorage(bundle *KimiAuthBundle) *KimiTokenStorage {
	expired := ""
	if bundle.TokenData.ExpiresAt > 0 {
		expired = time.Unix(bundle.TokenData.ExpiresAt, 0).UTC().Format(time.RFC3339)
	}
	return &KimiTokenStorage{
		AccessToken:  bundle.TokenData.AccessToken,
		RefreshToken: bundle.TokenData.RefreshToken,
		TokenType:    bundle.TokenData.TokenType,
		Scope:        bundle.TokenData.Scope,
		DeviceID:     strings.TrimSpace(bundle.DeviceID),
		Expired:      expired,
		Type:         "kimi",
	}
}

// DeviceFlowClient 处理 Kimi 的 OAuth2 设备流程。
type DeviceFlowClient struct {
	// httpClient 是用于发送 HTTP 请求的客户端
	httpClient *http.Client
	// cfg 是应用程序配置
	cfg *config.Config
	// deviceID 是设备标识符
	deviceID string
}

// NewDeviceFlowClient 创建一个新的设备流程客户端。
//
// 参数：
//   - cfg: 应用程序配置
//
// 返回：
//   - *DeviceFlowClient: 新的设备流程客户端实例
func NewDeviceFlowClient(cfg *config.Config) *DeviceFlowClient {
	return NewDeviceFlowClientWithDeviceID(cfg, "")
}

// NewDeviceFlowClientWithDeviceID 使用指定的设备 ID 创建一个新的设备流程客户端。
//
// 参数：
//   - cfg: 应用程序配置
//   - deviceID: 设备标识符
//
// 返回：
//   - *DeviceFlowClient: 新的设备流程客户端实例
func NewDeviceFlowClientWithDeviceID(cfg *config.Config, deviceID string) *DeviceFlowClient {
	return NewDeviceFlowClientWithDeviceIDAndProxyURL(cfg, deviceID, "")
}

// NewDeviceFlowClientWithDeviceIDAndProxyURL 使用代理覆盖创建一个新的设备流程客户端。
// 当 proxyURL 非空时，优先使用它而非 cfg.ProxyURL。
//
// 参数：
//   - cfg: 应用程序配置
//   - deviceID: 设备标识符
//   - proxyURL: 可选的代理 URL
//
// 返回：
//   - *DeviceFlowClient: 新的设备流程客户端实例
func NewDeviceFlowClientWithDeviceIDAndProxyURL(cfg *config.Config, deviceID string, proxyURL string) *DeviceFlowClient {
	client := &http.Client{Timeout: 30 * time.Second}
	effectiveProxyURL := strings.TrimSpace(proxyURL)
	var sdkCfg config.SDKConfig
	if cfg != nil {
		sdkCfg = cfg.SDKConfig
		if effectiveProxyURL == "" {
			effectiveProxyURL = strings.TrimSpace(cfg.ProxyURL)
		}
	}
	sdkCfg.ProxyURL = effectiveProxyURL
	client = util.SetProxy(&sdkCfg, client)

	resolvedDeviceID := strings.TrimSpace(deviceID)
	if resolvedDeviceID == "" {
		resolvedDeviceID = getOrCreateDeviceID()
	}
	return &DeviceFlowClient{
		httpClient: client,
		cfg:        cfg,
		deviceID:   resolvedDeviceID,
	}
}

// getOrCreateDeviceID 返回当前认证流程的内存设备 ID。
//
// 返回：
//   - string: 新生成的 UUID 设备 ID
func getOrCreateDeviceID() string {
	return uuid.New().String()
}

// getDeviceModel 返回设备模型字符串。
// 根据操作系统和架构生成设备描述。
//
// 返回：
//   - string: 设备模型描述字符串
func getDeviceModel() string {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	switch osName {
	case "darwin":
		return fmt.Sprintf("macOS %s", arch)
	case "windows":
		return fmt.Sprintf("Windows %s", arch)
	case "linux":
		return fmt.Sprintf("Linux %s", arch)
	default:
		return fmt.Sprintf("%s %s", osName, arch)
	}
}

// getHostname 返回机器主机名。
//
// 返回：
//   - string: 机器主机名，获取失败时返回 "unknown"
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// commonHeaders 返回 Kimi API 请求所需的通用请求头。
//
// 返回：
//   - map[string]string: 包含设备信息的请求头映射
func (c *DeviceFlowClient) commonHeaders() map[string]string {
	return map[string]string{
		"X-Msh-Platform":     "cli-proxy-api",
		"X-Msh-Version":      "1.0.0",
		"X-Msh-Device-Name":  getHostname(),
		"X-Msh-Device-Model": getDeviceModel(),
		"X-Msh-Device-Id":    c.deviceID,
	}
}

// RequestDeviceCode 通过向 Kimi 请求设备代码来启动设备流程。
//
// 参数：
//   - ctx: 请求的上下文
//
// 返回：
//   - *DeviceCodeResponse: 设备代码响应
//   - error: 请求失败时返回的错误
func (c *DeviceFlowClient) RequestDeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	data := url.Values{}
	data.Set("client_id", kimiClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kimiDeviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("kimi: failed to create device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	for k, v := range c.commonHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kimi: device code request failed: %w", err)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("kimi device code: close body error: %v", errClose)
		}
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kimi: failed to read device code response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kimi: device code request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var deviceCode DeviceCodeResponse
	if err = json.Unmarshal(bodyBytes, &deviceCode); err != nil {
		return nil, fmt.Errorf("kimi: failed to parse device code response: %w", err)
	}

	return &deviceCode, nil
}

// PollForToken 轮询令牌端点，直到用户完成授权或设备代码过期。
//
// 参数：
//   - ctx: 请求的上下文
//   - deviceCode: 设备代码响应
//
// 返回：
//   - *KimiTokenData: 令牌数据
//   - error: 授权失败时返回的错误
func (c *DeviceFlowClient) PollForToken(ctx context.Context, deviceCode *DeviceCodeResponse) (*KimiTokenData, error) {
	if deviceCode == nil {
		return nil, fmt.Errorf("kimi: device code is nil")
	}

	interval := time.Duration(deviceCode.Interval) * time.Second
	if interval < defaultPollInterval {
		interval = defaultPollInterval
	}

	deadline := time.Now().Add(maxPollDuration)
	if deviceCode.ExpiresIn > 0 {
		codeDeadline := time.Now().Add(time.Duration(deviceCode.ExpiresIn) * time.Second)
		if codeDeadline.Before(deadline) {
			deadline = codeDeadline
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("kimi: context cancelled: %w", ctx.Err())
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("kimi: device code expired")
			}

			token, pollErr, shouldContinue := c.exchangeDeviceCode(ctx, deviceCode.DeviceCode)
			if token != nil {
				return token, nil
			}
			if !shouldContinue {
				return nil, pollErr
			}
			// Continue polling
		}
	}
}

// exchangeDeviceCode 尝试用设备代码交换访问令牌。
// 返回 (令牌, 错误, 是否应继续轮询)。
//
// 参数：
//   - ctx: 请求的上下文
//   - deviceCode: 设备代码字符串
//
// 返回：
//   - *KimiTokenData: 令牌数据，成功时非 nil
//   - error: 错误信息
//   - bool: 是否应继续轮询
func (c *DeviceFlowClient) exchangeDeviceCode(ctx context.Context, deviceCode string) (*KimiTokenData, error, bool) {
	data := url.Values{}
	data.Set("client_id", kimiClientID)
	data.Set("device_code", deviceCode)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kimiTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("kimi: failed to create token request: %w", err), false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	for k, v := range c.commonHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kimi: token request failed: %w", err), false
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("kimi token exchange: close body error: %v", errClose)
		}
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kimi: failed to read token response: %w", err), false
	}

	// Parse response - Kimi returns 200 for both success and pending states
	var oauthResp struct {
		Error            string  `json:"error"`
		ErrorDescription string  `json:"error_description"`
		AccessToken      string  `json:"access_token"`
		RefreshToken     string  `json:"refresh_token"`
		TokenType        string  `json:"token_type"`
		ExpiresIn        float64 `json:"expires_in"`
		Scope            string  `json:"scope"`
	}

	if err = json.Unmarshal(bodyBytes, &oauthResp); err != nil {
		return nil, fmt.Errorf("kimi: failed to parse token response: %w", err), false
	}

	if oauthResp.Error != "" {
		switch oauthResp.Error {
		case "authorization_pending":
			return nil, nil, true // Continue polling
		case "slow_down":
			return nil, nil, true // Continue polling (with increased interval handled by caller)
		case "expired_token":
			return nil, fmt.Errorf("kimi: device code expired"), false
		case "access_denied":
			return nil, fmt.Errorf("kimi: access denied by user"), false
		default:
			return nil, fmt.Errorf("kimi: OAuth error: %s - %s", oauthResp.Error, oauthResp.ErrorDescription), false
		}
	}

	if oauthResp.AccessToken == "" {
		return nil, fmt.Errorf("kimi: empty access token in response"), false
	}

	var expiresAt int64
	if oauthResp.ExpiresIn > 0 {
		expiresAt = time.Now().Unix() + int64(oauthResp.ExpiresIn)
	}

	return &KimiTokenData{
		AccessToken:  oauthResp.AccessToken,
		RefreshToken: oauthResp.RefreshToken,
		TokenType:    oauthResp.TokenType,
		ExpiresAt:    expiresAt,
		Scope:        oauthResp.Scope,
	}, nil, false
}

// RefreshToken 用刷新令牌交换新的访问令牌。
//
// 参数：
//   - ctx: 请求的上下文
//   - refreshToken: 刷新令牌
//
// 返回：
//   - *KimiTokenData: 新的令牌数据
//   - error: 刷新失败时返回的错误
func (c *DeviceFlowClient) RefreshToken(ctx context.Context, refreshToken string) (*KimiTokenData, error) {
	data := url.Values{}
	data.Set("client_id", kimiClientID)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kimiTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("kimi: failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	for k, v := range c.commonHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kimi: refresh request failed: %w", err)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("kimi refresh token: close body error: %v", errClose)
		}
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kimi: failed to read refresh response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("kimi: refresh token rejected (status %d)", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kimi: refresh failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp struct {
		AccessToken  string  `json:"access_token"`
		RefreshToken string  `json:"refresh_token"`
		TokenType    string  `json:"token_type"`
		ExpiresIn    float64 `json:"expires_in"`
		Scope        string  `json:"scope"`
	}

	if err = json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("kimi: failed to parse refresh response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("kimi: empty access token in refresh response")
	}

	var expiresAt int64
	if tokenResp.ExpiresIn > 0 {
		expiresAt = time.Now().Unix() + int64(tokenResp.ExpiresIn)
	}

	return &KimiTokenData{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    expiresAt,
		Scope:        tokenResp.Scope,
	}, nil
}
