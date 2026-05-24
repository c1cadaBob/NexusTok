// 包 auth - claude.go
// 该文件实现了 Anthropic Claude 账户的 OAuth 登录流程。
// 包括 PKCE 码生成、本地回调服务器管理、授权码交换和令牌存储构建等功能。
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

// ClaudeAuthenticator 实现了 Anthropic Claude 账户的 OAuth 登录认证器。
type ClaudeAuthenticator struct {
	CallbackPort int // OAuth 回调服务器监听端口，默认 54545
}

// NewClaudeAuthenticator 使用默认设置构造一个 Claude 认证器实例。
//
// 返回:
//   - *ClaudeAuthenticator: Claude 认证器实例（回调端口 54545）
func NewClaudeAuthenticator() *ClaudeAuthenticator {
	return &ClaudeAuthenticator{CallbackPort: 54545}
}

// Provider 返回 Claude 提供商的标识名称。
func (a *ClaudeAuthenticator) Provider() string {
	return "claude"
}

// RefreshLead 指示管理器在令牌过期前四小时执行刷新。
//
// 返回:
//   - *time.Duration: 提前刷新的时间间隔
func (a *ClaudeAuthenticator) RefreshLead() *time.Duration {
	return new(4 * time.Hour)
}

// Login 执行 Claude 的完整 OAuth 登录流程。
// 流程包括：PKCE 码生成、启动回调服务器、打开浏览器、等待授权回调、交换令牌和构建认证记录。
//
// 参数:
//   - ctx: 请求上下文
//   - cfg: 应用配置
//   - opts: 登录选项
//
// 返回:
//   - *coreauth.Auth: 认证结果，包含令牌存储和元数据
//   - error: 登录失败时返回错误信息
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

	callbackPort := a.CallbackPort
	if opts.CallbackPort > 0 {
		callbackPort = opts.CallbackPort
	}

	pkceCodes, err := claude.GeneratePKCECodes()
	if err != nil {
		return nil, fmt.Errorf("claude pkce generation failed: %w", err)
	}

	state, err := misc.GenerateRandomState()
	if err != nil {
		return nil, fmt.Errorf("claude state generation failed: %w", err)
	}

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

	authSvc := claude.NewClaudeAuth(cfg)

	authURL, returnedState, err := authSvc.GenerateAuthURL(state, pkceCodes)
	if err != nil {
		return nil, fmt.Errorf("claude authorization url generation failed: %w", err)
	}
	state = returnedState

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

	callbackCh := make(chan *claude.OAuthResult, 1)
	callbackErrCh := make(chan error, 1)
	manualDescription := ""

	go func() {
		result, errWait := oauthServer.WaitForCallback(5 * time.Minute)
		if errWait != nil {
			callbackErrCh <- errWait
			return
		}
		callbackCh <- result
	}()

	var result *claude.OAuthResult
	var manualPromptTimer *time.Timer
	var manualPromptC <-chan time.Time
	if opts.Prompt != nil {
		manualPromptTimer = time.NewTimer(15 * time.Second)
		manualPromptC = manualPromptTimer.C
		defer manualPromptTimer.Stop()
	}

	var manualInputCh <-chan string
	var manualInputErrCh <-chan error

waitForCallback:
	for {
		select {
		case result = <-callbackCh:
			break waitForCallback
		case err = <-callbackErrCh:
			if strings.Contains(err.Error(), "timeout") {
				return nil, claude.NewAuthenticationError(claude.ErrCallbackTimeout, err)
			}
			return nil, err
		case <-manualPromptC:
			manualPromptC = nil
			if manualPromptTimer != nil {
				manualPromptTimer.Stop()
			}
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
			manualInputCh, manualInputErrCh = misc.AsyncPrompt(opts.Prompt, "Paste the Claude callback URL (or press Enter to keep waiting): ")
			continue
		case input := <-manualInputCh:
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
			return nil, errManual
		}
	}

	if result.Error != "" {
		return nil, claude.NewOAuthError(result.Error, manualDescription, http.StatusBadRequest)
	}

	if result.State != state {
		log.Errorf("State mismatch: expected %s, got %s", state, result.State)
		return nil, claude.NewAuthenticationError(claude.ErrInvalidState, fmt.Errorf("state mismatch"))
	}

	log.Debug("Claude authorization code received; exchanging for tokens")
	log.Debugf("Code: %s, State: %s", result.Code[:min(20, len(result.Code))], state)

	authBundle, err := authSvc.ExchangeCodeForTokens(ctx, result.Code, state, pkceCodes)
	if err != nil {
		log.Errorf("Token exchange failed: %v", err)
		return nil, claude.NewAuthenticationError(claude.ErrCodeExchangeFailed, err)
	}

	tokenStorage := authSvc.CreateTokenStorage(authBundle)

	if tokenStorage == nil || tokenStorage.Email == "" {
		return nil, fmt.Errorf("claude token storage missing account information")
	}

	fileName := fmt.Sprintf("claude-%s.json", tokenStorage.Email)
	metadata := map[string]any{
		"email": tokenStorage.Email,
	}

	fmt.Println("Claude authentication successful")
	if authBundle.APIKey != "" {
		fmt.Println("Claude API key obtained and stored")
	}

	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Storage:  tokenStorage,
		Metadata: metadata,
	}, nil
}
