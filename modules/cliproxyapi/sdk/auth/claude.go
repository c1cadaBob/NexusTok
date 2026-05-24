// auth - claude.go
// 本文件实现了 Anthropic Claude 账号的 OAuth 登录认证流程。
// Claude 认证器通过本地 OAuth 回调服务器接收授权码，然后交换令牌并构建认证记录。
// 支持自动打开浏览器、手动粘贴回调 URL 等多种交互模式。
package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/claude"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/browser"
	// legacy client removed
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

// ClaudeAuthenticator 实现了 Anthropic Claude 账号的 OAuth 登录流程。
// 它通过 PKCE（Proof Key for Code Exchange）增强安全性，
// 并在本地启动 HTTP 服务器接收 OAuth 回调。
type ClaudeAuthenticator struct {
	// CallbackPort 是 OAuth 回调服务器监听的本地端口号。
	// 默认值为 54545，可在创建时通过 opts.CallbackPort 覆盖。
	CallbackPort int
}

// NewClaudeAuthenticator 创建一个带有默认设置的 Claude 认证器实例。
// 默认回调端口为 54545。
func NewClaudeAuthenticator() *ClaudeAuthenticator {
	return &ClaudeAuthenticator{CallbackPort: 54545}
}

// Provider 返回该认证器对应的提供商标识 "claude"。
func (a *ClaudeAuthenticator) Provider() string {
	return "claude"
}

// RefreshLead 返回令牌到期前应提前执行刷新的时间间隔。
// Claude 令牌应在到期前 4 小时开始刷新，以避免服务中断。
func (a *ClaudeAuthenticator) RefreshLead() *time.Duration {
	return new(4 * time.Hour)
}

// Login 执行 Claude 账号的完整 OAuth 登录流程。
// 流程概述：
//  1. 生成 PKCE 码和随机 state 参数
//  2. 启动本地 OAuth 回调 HTTP 服务器
//  3. 生成授权 URL 并尝试打开浏览器
//  4. 等待 OAuth 回调（支持自动回调和手动粘贴两种模式）
//  5. 验证 state 参数，交换授权码获取令牌
//  6. 构建并返回认证记录
//
// 参数说明：
//   - ctx: 上下文，用于控制请求超时和取消
//   - cfg: 全局配置，不能为 nil
//   - opts: 登录选项，可为 nil 使用默认值
//
// 返回包含令牌存储和元数据的认证记录，或错误信息。
func (a *ClaudeAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cliproxy auth: configuration is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts == nil {
		opts = &LoginOptions{}
	}

	// 确定回调端口：优先使用 opts 中的自定义端口
	callbackPort := a.CallbackPort
	if opts.CallbackPort > 0 {
		callbackPort = opts.CallbackPort
	}

	// 生成 PKCE 码（code_verifier 和 code_challenge）
	pkceCodes, err := claude.GeneratePKCECodes()
	if err != nil {
		return nil, fmt.Errorf("claude pkce generation failed: %w", err)
	}

	// 生成随机 state 参数用于防止 CSRF 攻击
	state, err := misc.GenerateRandomState()
	if err != nil {
		return nil, fmt.Errorf("claude state generation failed: %w", err)
	}

	// 启动本地 OAuth 回调 HTTP 服务器
	oauthServer := claude.NewOAuthServer(callbackPort)
	if err = oauthServer.Start(); err != nil {
		if strings.Contains(err.Error(), "already in use") {
			return nil, claude.NewAuthenticationError(claude.ErrPortInUse, err)
		}
		return nil, claude.NewAuthenticationError(claude.ErrServerStartFailed, err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if stopErr := oauthServer.Stop(stopCtx); stopErr != nil {
			log.Warnf("claude oauth server stop error: %v", stopErr)
		}
	}()

	// 创建 Claude 认证服务实例
	authSvc := claude.NewClaudeAuth(cfg)

	// 生成 OAuth 授权 URL
	authURL, returnedState, err := authSvc.GenerateAuthURL(state, pkceCodes)
	if err != nil {
		return nil, fmt.Errorf("claude authorization url generation failed: %w", err)
	}
	state = returnedState

	// 根据 NoBrowser 选项决定是否自动打开浏览器
	if !opts.NoBrowser {
		fmt.Println("Opening browser for Claude authentication")
		if !browser.IsAvailable() {
			log.Warn("No browser available; please open the URL manually")
			util.PrintSSHTunnelInstructions(callbackPort)
			fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
		} else if err = browser.OpenURL(authURL); err != nil {
			log.Warnf("Failed to open browser automatically: %v", err)
			util.PrintSSHTunnelInstructions(callbackPort)
			fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
		}
	} else {
		util.PrintSSHTunnelInstructions(callbackPort)
		fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
	}

	fmt.Println("Waiting for Claude authentication callback...")

	// 创建回调结果通道和错误通道
	callbackCh := make(chan *claude.OAuthResult, 1)
	callbackErrCh := make(chan error, 1)
	manualDescription := ""

	// 启动协程等待 OAuth 回调
	go func() {
		result, errWait := oauthServer.WaitForCallback(5 * time.Minute)
		if errWait != nil {
			callbackErrCh <- errWait
			return
		}
		callbackCh <- result
	}()

	var result *claude.OAuthResult
	// 手动输入提示定时器：等待 15 秒后提示用户手动粘贴回调 URL
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
		case err = <-callbackErrCh:
			// 回调服务器返回错误
			if strings.Contains(err.Error(), "timeout") {
				return nil, claude.NewAuthenticationError(claude.ErrCallbackTimeout, err)
			}
			return nil, err
		case <-manualPromptC:
			// 15 秒超时，提示用户手动输入
			manualPromptC = nil
			if manualPromptTimer != nil {
				manualPromptTimer.Stop()
			}
			// 再次检查是否有自动回调到达
			select {
			case result = <-callbackCh:
				break waitForCallback
			case err = <-callbackErrCh:
				if strings.Contains(err.Error(), "timeout") {
					return nil, claude.NewAuthenticationError(claude.ErrCallbackTimeout, err)
				}
				return nil, err
			default:
			}
			// 异步提示用户手动粘贴回调 URL
			manualInputCh, manualInputErrCh = misc.AsyncPrompt(opts.Prompt, "Paste the Claude callback URL (or press Enter to keep waiting): ")
			continue
		case input := <-manualInputCh:
			// 收到用户手动粘贴的回调 URL
			manualInputCh = nil
			manualInputErrCh = nil
			parsed, errParse := misc.ParseOAuthCallback(input)
			if errParse != nil {
				return nil, errParse
			}
			if parsed == nil {
				continue
			}
			manualDescription = parsed.ErrorDescription
			result = &claude.OAuthResult{
				Code:  parsed.Code,
				State: parsed.State,
				Error: parsed.Error,
			}
			break waitForCallback
		case errManual := <-manualInputErrCh:
			// 手动输入过程中发生错误
			return nil, errManual
		}
	}

	// 检查 OAuth 回调是否返回错误
	if result.Error != "" {
		return nil, claude.NewOAuthError(result.Error, manualDescription, http.StatusBadRequest)
	}

	// 验证 state 参数以防止 CSRF 攻击
	if result.State != state {
		log.Errorf("State mismatch: expected %s, got %s", state, result.State)
		return nil, claude.NewAuthenticationError(claude.ErrInvalidState, fmt.Errorf("state mismatch"))
	}

	log.Debug("Claude authorization code received; exchanging for tokens")
	log.Debugf("Code: %s, State: %s", result.Code[:min(20, len(result.Code))], state)

	// 使用授权码交换令牌
	authBundle, err := authSvc.ExchangeCodeForTokens(ctx, result.Code, state, pkceCodes)
	if err != nil {
		log.Errorf("Token exchange failed: %v", err)
		return nil, claude.NewAuthenticationError(claude.ErrCodeExchangeFailed, err)
	}

	// 从认证包创建令牌存储对象
	tokenStorage := authSvc.CreateTokenStorage(authBundle)

	// 验证令牌存储包含必要的账号信息
	if tokenStorage == nil || tokenStorage.Email == "" {
		return nil, fmt.Errorf("claude token storage missing account information")
	}

	// 构建认证文件名和元数据
	fileName := fmt.Sprintf("claude-%s.json", tokenStorage.Email)
	metadata := map[string]any{
		"email": tokenStorage.Email,
	}

	fmt.Println("Claude authentication successful")
	if authBundle.APIKey != "" {
		fmt.Println("Claude API key obtained and stored")
	}

	// 返回认证记录
	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Storage:  tokenStorage,
		Metadata: metadata,
	}, nil
}
