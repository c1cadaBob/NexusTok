// auth - kimi.go
// 本文件实现了 Kimi（Moonshot AI）账号的设备流（Device Flow）OAuth 登录认证流程。
// Kimi 认证器通过设备码授权方式完成认证，用户在浏览器中访问验证 URL 并输入用户码，
// 系统通过轮询等待用户完成授权后获取令牌。
package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/kimi"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/browser"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

// kimiRefreshLead 是 Kimi 令牌到期前应提前执行刷新的时间间隔。
var kimiRefreshLead = 5 * time.Minute

// KimiAuthenticator 实现了 Kimi（Moonshot AI）账号的设备流 OAuth 登录流程。
// 设备流适用于无法直接进行 OAuth 回调的场景，用户通过访问验证 URL 并输入用户码完成授权。
type KimiAuthenticator struct{}

// NewKimiAuthenticator 创建一个新的 Kimi 认证器实例。
func NewKimiAuthenticator() Authenticator {
	return &KimiAuthenticator{}
}

// Provider 返回该认证器对应的提供商标识 "kimi"。
func (KimiAuthenticator) Provider() string {
	return "kimi"
}

// RefreshLead 返回令牌到期前应提前执行刷新的时间间隔。
// Kimi 令牌需要在到期前刷新以避免服务中断。
func (KimiAuthenticator) RefreshLead() *time.Duration {
	return &kimiRefreshLead
}

// Login 执行 Kimi 账号的设备流认证流程。
// 流程概述：
//  1. 启动设备流，获取设备码和用户码
//  2. 显示验证 URL 和用户码，提示用户在浏览器中完成授权
//  3. 尝试自动打开浏览器
//  4. 等待用户完成授权
//  5. 构建并返回认证记录
//
// 参数说明：
//   - ctx: 上下文，用于控制请求超时和取消
//   - cfg: 全局配置，不能为 nil
//   - opts: 登录选项，可为 nil 使用默认值
func (a KimiAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cliproxy auth: configuration is required")
	}
	if opts == nil {
		opts = &LoginOptions{}
	}

	// 创建 Kimi 认证服务
	authSvc := kimi.NewKimiAuth(cfg)

	// 启动设备流，获取设备码信息
	fmt.Println("Starting Kimi authentication...")
	deviceCode, err := authSvc.StartDeviceFlow(ctx)
	if err != nil {
		return nil, fmt.Errorf("kimi: failed to start device flow: %w", err)
	}

	// 确定验证 URL（优先使用完整 URI）
	verificationURL := deviceCode.VerificationURIComplete
	if verificationURL == "" {
		verificationURL = deviceCode.VerificationURI
	}

	// 显示验证 URL 和用户码
	fmt.Printf("\nTo authenticate, please visit:\n%s\n\n", verificationURL)
	if deviceCode.UserCode != "" {
		fmt.Printf("User code: %s\n\n", deviceCode.UserCode)
	}

	// 尝试自动打开浏览器
	if !opts.NoBrowser {
		if browser.IsAvailable() {
			if errOpen := browser.OpenURL(verificationURL); errOpen != nil {
				log.Warnf("Failed to open browser automatically: %v", errOpen)
			} else {
				fmt.Println("Browser opened automatically.")
			}
		}
	}

	fmt.Println("Waiting for authorization...")
	if deviceCode.ExpiresIn > 0 {
		fmt.Printf("(This will timeout in %d seconds if not authorized)\n", deviceCode.ExpiresIn)
	}

	// 等待用户完成授权
	authBundle, err := authSvc.WaitForAuthorization(ctx, deviceCode)
	if err != nil {
		return nil, fmt.Errorf("kimi: %w", err)
	}

	// 从认证包创建令牌存储对象
	tokenStorage := authSvc.CreateTokenStorage(authBundle)

	// 构建元数据
	metadata := map[string]any{
		"type":          "kimi",
		"access_token":  authBundle.TokenData.AccessToken,
		"refresh_token": authBundle.TokenData.RefreshToken,
		"token_type":    authBundle.TokenData.TokenType,
		"scope":         authBundle.TokenData.Scope,
		"timestamp":     time.Now().UnixMilli(),
	}

	// 添加令牌过期时间（如果可用）
	if authBundle.TokenData.ExpiresAt > 0 {
		exp := time.Unix(authBundle.TokenData.ExpiresAt, 0).UTC().Format(time.RFC3339)
		metadata["expired"] = exp
	}
	// 添加设备 ID（如果可用）
	if strings.TrimSpace(authBundle.DeviceID) != "" {
		metadata["device_id"] = strings.TrimSpace(authBundle.DeviceID)
	}

	// 生成唯一文件名（使用时间戳）
	fileName := fmt.Sprintf("kimi-%d.json", time.Now().UnixMilli())

	fmt.Println("\nKimi authentication successful!")

	// 返回认证记录
	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Label:    "Kimi User",
		Storage:  tokenStorage,
		Metadata: metadata,
	}, nil
}
