// auth - codex.go
// 本文件实现了 OpenAI Codex 账号的 OAuth 登录认证流程。
// Codex 认证器支持两种登录模式：标准 OAuth 回调流和设备流（Device Flow）。
// 标准模式通过本地 HTTP 服务器接收回调，设备流模式通过轮询等待用户授权。
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

// CodexAuthenticator 实现了 OpenAI Codex 账号的 OAuth 登录流程。
// 支持标准 OAuth 回调和设备流（Device Flow）两种认证模式。
type CodexAuthenticator struct {
	// CallbackPort 是 OAuth 回调服务器监听的本地端口号。
	// 默认值为 1455，可在创建时或通过 opts.CallbackPort 覆盖。
	CallbackPort int
}

// NewCodexAuthenticator 创建一个带有默认设置的 Codex 认证器实例。
// 默认回调端口为 1455。
func NewCodexAuthenticator() *CodexAuthenticator {
	return &CodexAuthenticator{CallbackPort: 1455}
}

// Provider 返回该认证器对应的提供商标识 "codex"。
func (a *CodexAuthenticator) Provider() string {
	return "codex"
}

// RefreshLead 返回令牌到期前应提前执行刷新的时间间隔。
// Codex 令牌应在到期前 5 天开始刷新，因为 Codex 令牌的有效期较长。
func (a *CodexAuthenticator) RefreshLead() *time.Duration {
	return new(5 * 24 * time.Hour)
}

// Login 执行 Codex 账号的完整 OAuth 登录流程。
// 流程概述：
//  1. 检查是否启用设备流模式（通过 opts.Metadata["codex_login_mode"] == "device"）
//  2. 如果是设备流模式，委托给 loginWithDeviceFlow 方法
//  3. 否则执行标准 OAuth 回调流：生成 PKCE 码、启动回调服务器、等待回调、交换令牌
//
// 参数说明：
//   - ctx: 上下文，用于控制请求超时和取消
//   - cfg: 全局配置，不能为 nil
//   - opts: 登录选项，可为 nil 使用默认值
//
// 返回包含令牌存储和元数据的认证记录，或错误信息。
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

	// 检查是否使用设备流模式
	if shouldUseCodexDeviceFlow(opts) {
		return a.loginWithDeviceFlow(ctx, cfg, opts)
	}

	// 标准 OAuth 回调流
	// 确定回调端口
	callbackPort := a.CallbackPort
	if opts.CallbackPort > 0 {
		callbackPort = opts.CallbackPort
	}

	// 生成 PKCE 码
	pkceCodes, err := codex.GeneratePKCECodes()
	if err != nil {
		return nil, fmt.Errorf("codex pkce generation failed: %w", err)
	}

	// 生成随机 state 参数
	state, err := misc.GenerateRandomState()
	if err != nil {
		return nil, fmt.Errorf("codex state generation failed: %w", err)
	}

	// 启动本地 OAuth 回调 HTTP 服务器
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

	// 创建 Codex 认证服务实例
	authSvc := codex.NewCodexAuth(cfg)

	// 生成 OAuth 授权 URL
	authURL, err := authSvc.GenerateAuthURL(state, pkceCodes)
	if err != nil {
		return nil, fmt.Errorf("codex authorization url generation failed: %w", err)
	}

	// 根据 NoBrowser 选项决定是否自动打开浏览器
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

	// 创建回调结果通道
	callbackCh := make(chan *codex.OAuthResult, 1)
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

	var result *codex.OAuthResult
	// 手动输入提示定时器
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
				return nil, codex.NewAuthenticationError(codex.ErrCallbackTimeout, err)
			}
			return nil, err
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
			case err = <-callbackErrCh:
				if strings.Contains(err.Error(), "timeout") {
					return nil, codex.NewAuthenticationError(codex.ErrCallbackTimeout, err)
				}
				return nil, err
			default:
			}
			// 异步提示用户手动粘贴回调 URL
			manualInputCh, manualInputErrCh = misc.AsyncPrompt(opts.Prompt, "Paste the Codex callback URL (or press Enter to keep waiting): ")
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
			result = &codex.OAuthResult{
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
		return nil, codex.NewOAuthError(result.Error, manualDescription, http.StatusBadRequest)
	}

	// 验证 state 参数
	if result.State != state {
		return nil, codex.NewAuthenticationError(codex.ErrInvalidState, fmt.Errorf("state mismatch"))
	}

	log.Debug("Codex authorization code received; exchanging for tokens")

	// 使用授权码交换令牌
	authBundle, err := authSvc.ExchangeCodeForTokens(ctx, result.Code, pkceCodes)
	if err != nil {
		return nil, codex.NewAuthenticationError(codex.ErrCodeExchangeFailed, err)
	}

	// 构建并返回认证记录
	return a.buildAuthRecord(authSvc, authBundle)
}
