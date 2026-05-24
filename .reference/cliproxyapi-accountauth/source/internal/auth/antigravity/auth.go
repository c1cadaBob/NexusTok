// antigravity - auth.go
// 包 antigravity 提供 Antigravity 提供商的 OAuth2 认证功能。
// 该文件实现了完整的 OAuth2 认证流程，包括构建授权 URL、
// 用授权码换取令牌、获取用户信息以及获取/创建项目 ID 等功能。
// Antigravity 是基于 Google OAuth2 体系的认证服务。
package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	log "github.com/sirupsen/logrus"
)

// TokenResponse 表示从 Google OAuth2 端点返回的令牌响应结构。
// 包含访问令牌、刷新令牌、过期时间和令牌类型等信息。
type TokenResponse struct {
	// AccessToken 是用于访问受保护资源的 OAuth2 访问令牌
	AccessToken string `json:"access_token"`
	// RefreshToken 是用于获取新访问令牌的刷新令牌
	RefreshToken string `json:"refresh_token"`
	// ExpiresIn 表示访问令牌的过期时间（秒）
	ExpiresIn int64 `json:"expires_in"`
	// TokenType 表示令牌类型，通常为 "Bearer"
	TokenType string `json:"token_type"`
}

// userInfo 表示从 Google 用户信息端点获取的用户个人资料。
// 仅包含用户的电子邮件地址信息。
type userInfo struct {
	// Email 是用户的电子邮件地址
	Email string `json:"email"`
}

// AntigravityAuth 是 Antigravity OAuth 认证的核心处理器。
// 它封装了 HTTP 客户端，提供 OAuth2 认证流程所需的各项操作方法。
type AntigravityAuth struct {
	// httpClient 是用于发送 HTTP 请求的客户端，支持代理配置
	httpClient *http.Client
}

// NewAntigravityAuth 创建一个新的 Antigravity 认证服务实例。
// 如果未提供配置或 HTTP 客户端，则使用默认值创建。
//
// 参数：
//   - cfg: 应用程序配置，包含代理设置等信息
//   - httpClient: 可选的自定义 HTTP 客户端，为 nil 时使用默认客户端
//
// 返回：
//   - *AntigravityAuth: 新的 Antigravity 认证服务实例
func NewAntigravityAuth(cfg *config.Config, httpClient *http.Client) *AntigravityAuth {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if httpClient != nil {
		return &AntigravityAuth{httpClient: httpClient}
	}
	return &AntigravityAuth{
		httpClient: util.SetProxy(&cfg.SDKConfig, &http.Client{}),
	}
}

// loadCodeAssistUserAgent 加载 Code Assist 用户代理字符串。
// 该方法从 misc 包获取 Antigravity 相关的 User-Agent 配置。
//
// 返回：
//   - string: 用于 Code Assist 请求的 User-Agent 字符串
func (o *AntigravityAuth) loadCodeAssistUserAgent() string {
	return misc.AntigravityLoadCodeAssistUserAgent("")
}

// BuildAuthURL 生成 OAuth 授权 URL。
// 构建包含所有必需参数的 Google OAuth2 授权请求 URL，
// 包括客户端 ID、重定向 URI、响应类型、权限范围和状态参数。
//
// 参数：
//   - state: 用于 CSRF 防护的随机状态参数
//   - redirectURI: OAuth 回调地址，为空时使用默认的本地回调地址
//
// 返回：
//   - string: 完整的 OAuth 授权 URL
func (o *AntigravityAuth) BuildAuthURL(state, redirectURI string) string {
	if strings.TrimSpace(redirectURI) == "" {
		redirectURI = fmt.Sprintf("http://localhost:%d/oauth-callback", CallbackPort)
	}
	params := url.Values{}
	params.Set("access_type", "offline")
	params.Set("client_id", ClientID)
	params.Set("prompt", "consent")
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join(Scopes, " "))
	params.Set("state", state)
	return AuthEndpoint + "?" + params.Encode()
}

// ExchangeCodeForTokens 使用授权码换取访问令牌和刷新令牌。
// 向 Google OAuth2 令牌端点发送 POST 请求，将授权码交换为访问令牌。
//
// 参数：
//   - ctx: 请求的上下文，用于控制请求的生命周期
//   - code: 从 OAuth 回调中获取的授权码
//   - redirectURI: 与授权请求中使用的重定向 URI 一致
//
// 返回：
//   - *TokenResponse: 包含访问令牌和刷新令牌的响应
//   - error: 令牌交换失败时返回的错误
func (o *AntigravityAuth) ExchangeCodeForTokens(ctx context.Context, code, redirectURI string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", ClientID)
	data.Set("client_secret", ClientSecret)
	data.Set("redirect_uri", redirectURI)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("antigravity token exchange: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, errDo := o.httpClient.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("antigravity token exchange: execute request: %w", errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("antigravity token exchange: close body error: %v", errClose)
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, errRead := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		if errRead != nil {
			return nil, fmt.Errorf("antigravity token exchange: read response: %w", errRead)
		}
		body := strings.TrimSpace(string(bodyBytes))
		if body == "" {
			return nil, fmt.Errorf("antigravity token exchange: request failed: status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("antigravity token exchange: request failed: status %d: %s", resp.StatusCode, body)
	}

	var token TokenResponse
	if errDecode := json.NewDecoder(resp.Body).Decode(&token); errDecode != nil {
		return nil, fmt.Errorf("antigravity token exchange: decode response: %w", errDecode)
	}
	return &token, nil
}

// FetchUserInfo 从 Google 用户信息端点获取用户的电子邮件地址。
// 使用提供的访问令牌调用 Google OAuth2 用户信息 API 来获取用户资料。
//
// 参数：
//   - ctx: 请求的上下文
//   - accessToken: 用于身份验证的 OAuth2 访问令牌
//
// 返回：
//   - string: 用户的电子邮件地址
//   - error: 获取用户信息失败时返回的错误
func (o *AntigravityAuth) FetchUserInfo(ctx context.Context, accessToken string) (string, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return "", fmt.Errorf("antigravity userinfo: missing access token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, UserInfoEndpoint, nil)
	if err != nil {
		return "", fmt.Errorf("antigravity userinfo: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", o.loadCodeAssistUserAgent())

	resp, errDo := o.httpClient.Do(req)
	if errDo != nil {
		return "", fmt.Errorf("antigravity userinfo: execute request: %w", errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("antigravity userinfo: close body error: %v", errClose)
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, errRead := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		if errRead != nil {
			return "", fmt.Errorf("antigravity userinfo: read response: %w", errRead)
		}
		body := strings.TrimSpace(string(bodyBytes))
		if body == "" {
			return "", fmt.Errorf("antigravity userinfo: request failed: status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("antigravity userinfo: request failed: status %d: %s", resp.StatusCode, body)
	}
	var info userInfo
	if errDecode := json.NewDecoder(resp.Body).Decode(&info); errDecode != nil {
		return "", fmt.Errorf("antigravity userinfo: decode response: %w", errDecode)
	}
	email := strings.TrimSpace(info.Email)
	if email == "" {
		return "", fmt.Errorf("antigravity userinfo: response missing email")
	}
	return email, nil
}

// FetchProjectID 获取已认证用户的项目 ID。
// 通过调用 loadCodeAssist API 来检索与用户关联的云代码伴侣项目 ID。
// 如果项目不存在，则自动调用 OnboardUser 来创建新项目。
//
// 参数：
//   - ctx: 请求的上下文
//   - accessToken: 用于身份验证的 OAuth2 访问令牌
//
// 返回：
//   - string: 用户的云代码伴侣项目 ID
//   - error: 获取项目 ID 失败时返回的错误
func (o *AntigravityAuth) FetchProjectID(ctx context.Context, accessToken string) (string, error) {
	userAgent := o.loadCodeAssistUserAgent()
	loadReqBody := map[string]any{
		"metadata": map[string]string{
			"ide_type":    "ANTIGRAVITY",
			"ide_version": misc.AntigravityVersionFromUserAgent(userAgent),
			"ide_name":    "antigravity",
		},
	}

	rawBody, errMarshal := json.Marshal(loadReqBody)
	if errMarshal != nil {
		return "", fmt.Errorf("marshal request body: %w", errMarshal)
	}

	endpointURL := fmt.Sprintf("%s/%s:loadCodeAssist", APIEndpoint, APIVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, strings.NewReader(string(rawBody)))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Goog-Api-Client", misc.AntigravityGoogAPIClientUA)

	resp, errDo := o.httpClient.Do(req)
	if errDo != nil {
		return "", fmt.Errorf("execute request: %w", errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("antigravity loadCodeAssist: close body error: %v", errClose)
		}
	}()

	bodyBytes, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return "", fmt.Errorf("read response: %w", errRead)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var loadResp map[string]any
	if errDecode := json.Unmarshal(bodyBytes, &loadResp); errDecode != nil {
		return "", fmt.Errorf("decode response: %w", errDecode)
	}

	// Extract projectID from response
	projectID := ""
	if id, ok := loadResp["cloudaicompanionProject"].(string); ok {
		projectID = strings.TrimSpace(id)
	}
	if projectID == "" {
		if projectMap, ok := loadResp["cloudaicompanionProject"].(map[string]any); ok {
			if id, okID := projectMap["id"].(string); okID {
				projectID = strings.TrimSpace(id)
			}
		}
	}

	if projectID == "" {
		tierID := "legacy-tier"
		if tiers, okTiers := loadResp["allowedTiers"].([]any); okTiers {
			for _, rawTier := range tiers {
				tier, okTier := rawTier.(map[string]any)
				if !okTier {
					continue
				}
				if isDefault, okDefault := tier["isDefault"].(bool); okDefault && isDefault {
					if id, okID := tier["id"].(string); okID && strings.TrimSpace(id) != "" {
						tierID = strings.TrimSpace(id)
						break
					}
				}
			}
		}

		projectID, err = o.OnboardUser(ctx, accessToken, tierID)
		if err != nil {
			return "", err
		}
		return projectID, nil
	}

	return projectID, nil
}

// OnboardUser 尝试通过轮询方式获取项目 ID。
// 当用户尚未创建项目时，此方法会调用 onboardUser API 来注册用户，
// 并通过轮询等待操作完成以获取新创建的项目 ID。
//
// 参数：
//   - ctx: 请求的上下文
//   - accessToken: 用于身份验证的 OAuth2 访问令牌
//   - tierID: 用户的服务层级 ID
//
// 返回：
//   - string: 新创建的项目 ID
//   - error: 用户注册失败时返回的错误
func (o *AntigravityAuth) OnboardUser(ctx context.Context, accessToken, tierID string) (string, error) {
	log.Infof("Antigravity: onboarding user with tier: %s", tierID)
	userAgent := o.loadCodeAssistUserAgent()
	requestBody := map[string]any{
		"tierId": tierID,
		"metadata": map[string]string{
			"ide_type":    "ANTIGRAVITY",
			"ide_version": misc.AntigravityVersionFromUserAgent(userAgent),
			"ide_name":    "antigravity",
		},
	}

	rawBody, errMarshal := json.Marshal(requestBody)
	if errMarshal != nil {
		return "", fmt.Errorf("marshal request body: %w", errMarshal)
	}

	maxAttempts := 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Debugf("Polling attempt %d/%d", attempt, maxAttempts)

		reqCtx := ctx
		var cancel context.CancelFunc
		if reqCtx == nil {
			reqCtx = context.Background()
		}
		reqCtx, cancel = context.WithTimeout(reqCtx, 30*time.Second)

		endpointURL := fmt.Sprintf("%s/%s:onboardUser", APIEndpoint, APIVersion)
		req, errRequest := http.NewRequestWithContext(reqCtx, http.MethodPost, endpointURL, strings.NewReader(string(rawBody)))
		if errRequest != nil {
			cancel()
			return "", fmt.Errorf("create request: %w", errRequest)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("X-Goog-Api-Client", misc.AntigravityGoogAPIClientUA)

		resp, errDo := o.httpClient.Do(req)
		if errDo != nil {
			cancel()
			return "", fmt.Errorf("execute request: %w", errDo)
		}

		bodyBytes, errRead := io.ReadAll(resp.Body)
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("close body error: %v", errClose)
		}
		cancel()

		if errRead != nil {
			return "", fmt.Errorf("read response: %w", errRead)
		}

		if resp.StatusCode == http.StatusOK {
			var data map[string]any
			if errDecode := json.Unmarshal(bodyBytes, &data); errDecode != nil {
				return "", fmt.Errorf("decode response: %w", errDecode)
			}

			if done, okDone := data["done"].(bool); okDone && done {
				projectID := ""
				if responseData, okResp := data["response"].(map[string]any); okResp {
					switch projectValue := responseData["cloudaicompanionProject"].(type) {
					case map[string]any:
						if id, okID := projectValue["id"].(string); okID {
							projectID = strings.TrimSpace(id)
						}
					case string:
						projectID = strings.TrimSpace(projectValue)
					}
				}

				if projectID != "" {
					log.Infof("Successfully fetched project_id: %s", projectID)
					return projectID, nil
				}

				return "", fmt.Errorf("no project_id in response")
			}

			time.Sleep(2 * time.Second)
			continue
		}

		responsePreview := strings.TrimSpace(string(bodyBytes))
		if len(responsePreview) > 500 {
			responsePreview = responsePreview[:500]
		}

		responseErr := responsePreview
		if len(responseErr) > 200 {
			responseErr = responseErr[:200]
		}
		return "", fmt.Errorf("http %d: %s", resp.StatusCode, responseErr)
	}

	return "", nil
}
