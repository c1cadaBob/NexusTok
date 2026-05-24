// auth - xai.go
// 本文件实现了 xAI Grok 账号的 OAuth 登录认证流程。
// xAI 认证器通过 OIDC（OpenID Connect）发现机制获取授权端点，
// 使用 PKCE 增强安全性，并在本地启动 HTTP 服务器接收 OAuth 回调。
// 支持自动打开浏览器、手动粘贴回调 Token 等多种交互模式。
package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	xaiauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/xai"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/browser"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

// XAIAuthenticator 实现了 xAI Grok 账号的 OAuth 回环登录流程。
// 它通过 OIDC 发现机制自动获取授权端点和令牌端点，
// 并在本地启动 HTTP 服务器接收 OAuth 回调。
type XAIAuthenticator struct{}

// NewXAIAuthenticator 创建一个新的 xAI 认证器实例。
func NewXAIAuthenticator() Authenticator {
	return &XAIAuthenticator{}
}

// Provider 返回该认证器对应的提供商标识 "xai"。
func (XAIAuthenticator) Provider() string {
	return "xai"
}

// RefreshLead 返回令牌到期前应提前执行刷新的时间间隔。
// 该值由内部 xaiauth 包定义。
func (XAIAuthenticator) RefreshLead() *time.Duration {
	lead := xaiauth.RefreshLead()
	return &lead
}

// Login 执行 xAI 账号的完整 OAuth 登录流程。
// 流程概述：
//  1. 生成 PKCE 码、随机 state 和 nonce 参数
//  2. 通过 OIDC 发现机制获取授权端点和令牌端点
//  3. 启动本地 OAuth 回调 HTTP 服务器
//  4. 生成授权 URL 并尝试打开浏览器
//  5. 等待 OAuth 回调（支持自动回调和手动粘贴两种模式）
//  6. 验证 state 参数，交换授权码获取令牌
//  7. 构建并返回认证记录
//
// 参数说明：
//   - ctx: 上下文，用于控制请求超时和取消
//   - cfg: 全局配置，不能为 nil
//   - opts: 登录选项，可为 nil 使用默认值
func (a XAIAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cliproxy auth: configuration is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts == nil {
		opts = &LoginOptions{}
	}

	// 确定回调端口
	callbackPort := xaiauth.CallbackPort
	if opts.CallbackPort > 0 {
		callbackPort = opts.CallbackPort
	}

	// 生成 PKCE 码
	pkceCodes, err := xaiauth.GeneratePKCECodes()
	if err != nil {
		return nil, fmt.Errorf("xai pkce generation failed: %w", err)
	}
	// 生成随机 state 参数
	state, err := misc.GenerateRandomState()
	if err != nil {
		return nil, fmt.Errorf("xai state generation failed: %w", err)
	}
	// 生成随机 nonce 参数（用于 OIDC）
	nonce, err := misc.GenerateRandomState()
	if err != nil {
		return nil, fmt.Errorf("xai nonce generation failed: %w", err)
	}

	// 创建 xAI 认证服务并执行 OIDC 发现
	authSvc := xaiauth.NewXAIAuth(cfg)
	discovery, err := authSvc.Discover(ctx)
	if err != nil {
		return nil, err
	}

	// 启动本地 OAuth 回调 HTTP 服务器
	srv, port, callbackCh, errServer := startXAICallbackServer(callbackPort)
	if errServer != nil {
		return nil, fmt.Errorf("xai: failed to start callback server: %w", errServer)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if errShutdown := srv.Shutdown(shutdownCtx); errShutdown != nil {
			log.Warnf("xai callback server shutdown error: %v", errShutdown)
		}
	}()

	// 构建重定向 URI 和授权 URL
	redirectURI := fmt.Sprintf("http://%s:%d%s", xaiauth.RedirectHost, port, xaiauth.RedirectPath)
	authURL, err := xaiauth.BuildAuthorizeURL(xaiauth.AuthorizeURLParams{
		AuthorizationEndpoint: discovery.AuthorizationEndpoint,
		RedirectURI:           redirectURI,
		CodeChallenge:         pkceCodes.CodeChallenge,
		State:                 state,
		Nonce:                 nonce,
	})
	if err != nil {
		return nil, err
	}

	// 根据 NoBrowser 选项决定是否自动打开浏览器
	if !opts.NoBrowser {
		fmt.Println("Opening browser for xAI authentication")
		if !browser.IsAvailable() {
			log.Warn("No browser available; please open the URL manually")
			util.PrintSSHTunnelInstructions(port)
			fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
		} else if errOpen := browser.OpenURL(authURL); errOpen != nil {
			log.Warnf("Failed to open browser automatically: %v", errOpen)
			util.PrintSSHTunnelInstructions(port)
			fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
		}
	} else {
		util.PrintSSHTunnelInstructions(port)
		fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
	}

	fmt.Println("Waiting for xAI authentication callback...")

	var result callbackResult
	// 认证总超时定时器（5 分钟）
	timeoutTimer := time.NewTimer(5 * time.Minute)
	defer timeoutTimer.Stop()

	// 手动输入提示定时器（15 秒后提示）
	var manualPromptTimer *time.Timer
	var manualPromptC <-chan time.Time
	if opts.Prompt != nil {
		manualPromptTimer = time.NewTimer(15 * time.Second)
		manualPromptC = manualPromptTimer.C
		defer manualPromptTimer.Stop()
	}

	var manualInputCh <-chan string
	var manualInputErrCh <-chan error

	// 等待 OAuth 回调结果的主循环
waitForCallback:
	for {
		select {
		case result = <-callbackCh:
			// 收到自动回调结果
			break waitForCallback
		case <-manualPromptC:
			// 超时后提示用户手动输入
			manualPromptC = nil
			if manualPromptTimer != nil {
				manualPromptTimer.Stop()
			}
			// 再次检查是否有自动回调到达
			select {
			case result = <-callbackCh:
				break waitForCallback
			default:
			}
			// 异步提示用户手动粘贴回调 Token
			manualInputCh, manualInputErrCh = misc.AsyncPrompt(opts.Prompt, "Paste the xAI callback Token (or press Enter to keep waiting): ")
			continue
		case input := <-manualInputCh:
			// 收到用户手动粘贴的 Token
			manualInputCh = nil
			manualInputErrCh = nil
			manualResult, ok, errParse := parseXAIManualCallbackToken(input, state)
			if errParse != nil {
				return nil, errParse
			}
			if !ok {
				continue
			}
			result = manualResult
			break waitForCallback
		case errManual := <-manualInputErrCh:
			// 手动输入过程中发生错误
			return nil, errManual
		case <-timeoutTimer.C:
			// 认证超时
			return nil, fmt.Errorf("xai: authentication timed out")
		}
	}

	// 检查 OAuth 回调是否返回错误
	if result.Error != "" {
		return nil, fmt.Errorf("xai: authentication failed: %s", result.Error)
	}
	// 验证 state 参数
	if result.State != state {
		return nil, fmt.Errorf("xai: invalid state")
	}
	// 检查授权码是否为空
	if result.Code == "" {
		return nil, fmt.Errorf("xai: missing authorization code")
	}

	// 使用授权码交换令牌
	bundle, errExchange := authSvc.ExchangeCodeForTokens(ctx, result.Code, redirectURI, pkceCodes, discovery.TokenEndpoint)
	if errExchange != nil {
		return nil, fmt.Errorf("xai: token exchange failed: %w", errExchange)
	}
	// 从认证包创建令牌存储对象
	tokenStorage := authSvc.CreateTokenStorage(bundle)
	if tokenStorage == nil || strings.TrimSpace(tokenStorage.AccessToken) == "" {
		return nil, fmt.Errorf("xai token storage missing access token")
	}

	// 构建认证文件名和标签
	fileName := xaiauth.CredentialFileName(tokenStorage.Email, tokenStorage.Subject)
	label := strings.TrimSpace(tokenStorage.Email)
	if label == "" {
		label = "xAI"
	}

	// 构建元数据
	metadata := map[string]any{
		"type":           "xai",
		"access_token":   tokenStorage.AccessToken,
		"refresh_token":  tokenStorage.RefreshToken,
		"id_token":       tokenStorage.IDToken,
		"token_type":     tokenStorage.TokenType,
		"expires_in":     tokenStorage.ExpiresIn,
		"expired":        tokenStorage.Expire,
		"last_refresh":   tokenStorage.LastRefresh,
		"base_url":       tokenStorage.BaseURL,
		"redirect_uri":   tokenStorage.RedirectURI,
		"token_endpoint": tokenStorage.TokenEndpoint,
		"auth_kind":      "oauth",
	}
	if tokenStorage.Email != "" {
		metadata["email"] = tokenStorage.Email
	}
	if tokenStorage.Subject != "" {
		metadata["sub"] = tokenStorage.Subject
	}

	fmt.Println("xAI authentication successful")

	// 返回认证记录
	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Label:    label,
		Storage:  tokenStorage,
		Metadata: metadata,
		Attributes: map[string]string{
			"auth_kind": "oauth",
			"base_url":  tokenStorage.BaseURL,
		},
	}, nil
}

// parseXAIManualCallbackToken 解析用户手动粘贴的 xAI 回调 Token。
// 该函数验证输入是否为纯 Token（非 URL 格式），并将其与 state 参数组合为回调结果。
// 参数说明：
//   - input: 用户粘贴的原始输入
//   - state: 期望的 state 参数值
//
// 返回值：
//   - callbackResult: 解析后的回调结果
//   - bool: 是否成功解析（输入为空时返回 false）
//   - error: 解析错误（如输入包含 URL 格式）
func parseXAIManualCallbackToken(input string, state string) (callbackResult, bool, error) {
	token := strings.TrimSpace(input)
	if token == "" {
		return callbackResult{}, false, nil
	}
	// 拒绝 URL 格式的输入，要求用户仅粘贴 Token
	if strings.Contains(token, "://") || strings.Contains(token, "?") || strings.Contains(token, "code=") {
		return callbackResult{}, false, fmt.Errorf("xai: paste only the callback token")
	}
	return callbackResult{Code: token, State: state}, true, nil
}

// startXAICallbackServer 启动一个本地 HTTP 服务器用于接收 xAI OAuth 回调。
// 服务器监听指定端口（或自动分配可用端口），在收到回调请求后通过通道传递结果。
// 参数说明：
//   - port: 监听端口号，<= 0 时使用默认端口
//
// 返回值：
//   - *http.Server: HTTP 服务器实例
//   - int: 实际监听的端口号
//   - <-chan callbackResult: 回调结果通道
//   - error: 启动错误
func startXAICallbackServer(port int) (*http.Server, int, <-chan callbackResult, error) {
	if port <= 0 {
		port = xaiauth.CallbackPort
	}
	addr := fmt.Sprintf("%s:%d", xaiauth.RedirectHost, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, 0, nil, err
	}
	// 获取实际分配的端口号（当指定端口为 0 时）
	port = listener.Addr().(*net.TCPAddr).Port
	resultCh := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(xaiauth.RedirectPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		result := callbackResult{
			Code:  strings.TrimSpace(q.Get("code")),
			Error: strings.TrimSpace(q.Get("error")),
			State: strings.TrimSpace(q.Get("state")),
		}
		resultCh <- result
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if result.Code != "" && result.Error == "" {
			_, _ = w.Write([]byte("<h1>Login successful</h1><p>You can close this window.</p>"))
			return
		}
		_, _ = w.Write([]byte("<h1>Login failed</h1><p>Please check the CLI output.</p>"))
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}
	go func() {
		if errServe := srv.Serve(listener); errServe != nil && !strings.Contains(errServe.Error(), "Server closed") {
			log.Warnf("xai callback server error: %v", errServe)
		}
	}()

	return srv, port, resultCh, nil
}
