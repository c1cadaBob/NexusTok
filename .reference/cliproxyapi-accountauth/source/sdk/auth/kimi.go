// 包 auth - kimi.go
// 该文件实现了 Kimi（月之暗面 Moonshot AI）的设备码认证流程。
// 通过设备码流程获取访问令牌和刷新令牌，并将认证信息持久化到文件。
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

// kimiRefreshLead 是令牌过期前应执行刷新的时间间隔。
var kimiRefreshLead = 5 * time.Minute

// KimiAuthenticator 实现了 Kimi（月之暗面 Moonshot AI）的设备码登录认证器。
type KimiAuthenticator struct{}

// NewKimiAuthenticator 构造一个新的 Kimi 认证器实例。
//
// 返回:
//   - Authenticator: Kimi 认证器实例
func NewKimiAuthenticator() Authenticator {
	return &KimiAuthenticator{}
}

// Provider 返回 Kimi 提供商的标识名称。
func (KimiAuthenticator) Provider() string {
	return "kimi"
}

// RefreshLead 返回令牌过期前应执行刷新的时间间隔。
// Kimi 令牌有过期时间，需要在过期前刷新。
//
// 返回:
//   - *time.Duration: 提前刷新的时间间隔（5 分钟）
func (KimiAuthenticator) RefreshLead() *time.Duration {
	return &kimiRefreshLead
}

// Login 启动 Kimi 的设备码认证流程。
// 用户在浏览器中输入设备码完成认证后，系统自动获取令牌并构建认证记录。
//
// 参数:
//   - ctx: 请求上下文
//   - cfg: 应用配置
//   - opts: 登录选项
//
// 返回:
//   - *coreauth.Auth: 认证结果，包含令牌存储和元数据
//   - error: 登录失败时返回错误信息
func (a KimiAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cliproxy auth: configuration is required")
	}
	if opts == nil {
		opts = &LoginOptions{}
	}

	authSvc := kimi.NewKimiAuth(cfg)

	// Start the device flow
	fmt.Println("Starting Kimi authentication...")
	deviceCode, err := authSvc.StartDeviceFlow(ctx)
	if err != nil {
		return nil, fmt.Errorf("kimi: failed to start device flow: %w", err)
	}

	// Display the verification URL
	verificationURL := deviceCode.VerificationURIComplete
	if verificationURL == "" {
		verificationURL = deviceCode.VerificationURI
	}

	fmt.Printf("\nTo authenticate, please visit:\n%s\n\n", verificationURL)
	if deviceCode.UserCode != "" {
		fmt.Printf("User code: %s\n\n", deviceCode.UserCode)
	}

	// Try to open the browser automatically
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

	// Wait for user authorization
	authBundle, err := authSvc.WaitForAuthorization(ctx, deviceCode)
	if err != nil {
		return nil, fmt.Errorf("kimi: %w", err)
	}

	// Create the token storage
	tokenStorage := authSvc.CreateTokenStorage(authBundle)

	// Build metadata with token information
	metadata := map[string]any{
		"type":          "kimi",
		"access_token":  authBundle.TokenData.AccessToken,
		"refresh_token": authBundle.TokenData.RefreshToken,
		"token_type":    authBundle.TokenData.TokenType,
		"scope":         authBundle.TokenData.Scope,
		"timestamp":     time.Now().UnixMilli(),
	}

	if authBundle.TokenData.ExpiresAt > 0 {
		exp := time.Unix(authBundle.TokenData.ExpiresAt, 0).UTC().Format(time.RFC3339)
		metadata["expired"] = exp
	}
	if strings.TrimSpace(authBundle.DeviceID) != "" {
		metadata["device_id"] = strings.TrimSpace(authBundle.DeviceID)
	}

	// Generate a unique filename
	fileName := fmt.Sprintf("kimi-%d.json", time.Now().UnixMilli())

	fmt.Println("\nKimi authentication successful!")

	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Label:    "Kimi User",
		Storage:  tokenStorage,
		Metadata: metadata,
	}, nil
}
