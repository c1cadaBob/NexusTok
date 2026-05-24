// 包 auth - codex.go
// 该文件实现了 Codex 账户的 OAuth 登录流程。
// 包括 PKCE 码生成、本地 OAuth 服务器管理、授权码交换和认证记录构建等功能。
// 支持标准浏览器流程和设备码流程两种认证方式。
package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/codex"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/browser"
	// legacy client removed
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

// CodexAuthenticator 实现了 Codex 账户的 OAuth 登录认证器。
type CodexAuthenticator struct {
	CallbackPort int // OAuth 回调服务器监听端口，默认 1455
}

// NewCodexAuthenticator 使用默认设置构造一个 Codex 认证器实例。
//
// 返回:
//   - *CodexAuthenticator: Codex 认证器实例（回调端口 1455）
func NewCodexAuthenticator() *CodexAuthenticator {
	return &CodexAuthenticator{CallbackPort: 1455}
}

// Provider 返回 Codex 提供商的标识名称。
func (a *CodexAuthenticator) Provider() string {
	return "codex"
}

// RefreshLead 指示管理器在令牌过期前五天执行刷新。
//
// 返回:
//   - *time.Duration: 提前刷新的时间间隔
func (a *CodexAuthenticator) RefreshLead() *time.Duration {
	return new(5 * 24 * time.Hour)
}

// Login 执行 Codex 的完整 OAuth 登录流程。
// 如果登录选项指定了设备模式，则走设备认证流程；否则走标准浏览器 OAuth 流程。
//
// 参数:
//   - ctx: 请求上下文
//   - cfg: 应用配置
//   - opts: 登录选项
//
// 返回:
//   - *coreauth.Auth: 认证结果，包含令牌存储和元数据
//   - error: 登录失败时返回错误信息
func (a *CodexAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cliproxy auth: configuration is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts == nil {
		opts = &LoginOptions{}
	}

	if shouldUseCodexDeviceFlow(opts) {
		return a.loginWithDeviceFlow(ctx, cfg, opts)
	}

	callbackPort := a.CallbackPort
	if opts.CallbackPort > 0 {
		callbackPort = opts.CallbackPort
	}

	pkceCodes, err := codex.GeneratePKCECodes()
	if err != nil {
		return nil, fmt.Errorf("codex pkce generation failed: %w", err)
	}

	state, err := misc.GenerateRandomState()
	if err != nil {
		return nil, fmt.Errorf("codex state generation failed: %w", err)
	}

	oauthServer := codex.NewOAuthServer(callbackPort)
	if err = oauthServer.Start(); err != nil {
		if strings.Contains(err.Error(), "already in use") {
			return nil, codex.NewAuthenticationError(codex.ErrPortInUse, err)
		}
		return nil, codex.NewAuthenticationError(codex.ErrServerStartFailed, err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if stopErr := oauthServer.Stop(stopCtx); stopErr != nil {
			log.Warnf("codex oauth server stop error: %v", stopErr)
		}
	}()

	authSvc := codex.NewCodexAuth(cfg)

	authURL, err := authSvc.GenerateAuthURL(state, pkceCodes)
	if err != nil {
		return nil, fmt.Errorf("codex authorization url generation failed: %w", err)
	}

	if !opts.NoBrowser {
		fmt.Println("Opening browser for Codex authentication")
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

	fmt.Println("Waiting for Codex authentication callback...")

	callbackCh := make(chan *codex.OAuthResult, 1)
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

	var result *codex.OAuthResult
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
				return nil, codex.NewAuthenticationError(codex.ErrCallbackTimeout, err)
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
					return nil, codex.NewAuthenticationError(codex.ErrCallbackTimeout, err)
				}
				return nil, err
			default:
			}
			manualInputCh, manualInputErrCh = misc.AsyncPrompt(opts.Prompt, "Paste the Codex callback URL (or press Enter to keep waiting): ")
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
			result = &codex.OAuthResult{
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
		return nil, codex.NewOAuthError(result.Error, manualDescription, http.StatusBadRequest)
	}

	if result.State != state {
		return nil, codex.NewAuthenticationError(codex.ErrInvalidState, fmt.Errorf("state mismatch"))
	}

	log.Debug("Codex authorization code received; exchanging for tokens")

	authBundle, err := authSvc.ExchangeCodeForTokens(ctx, result.Code, pkceCodes)
	if err != nil {
		return nil, codex.NewAuthenticationError(codex.ErrCodeExchangeFailed, err)
	}

	return a.buildAuthRecord(authSvc, authBundle)
}
