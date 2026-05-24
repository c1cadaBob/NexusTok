// 包 auth - interfaces.go
// 该文件定义了认证器的公共接口和登录选项结构体。
// 提供了跨认证器共享的通用登录参数和认证器接口规范。
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// ErrRefreshNotSupported 表示当前认证器不支持令牌刷新操作。
var ErrRefreshNotSupported = errors.New("cliproxy auth: refresh not supported")

// LoginOptions 捕获跨认证器共享的通用登录参数。
// 提供商特定的逻辑可以通过 Metadata 传递额外参数。
type LoginOptions struct {
	NoBrowser    bool              // 是否禁止自动打开浏览器（适用于无头环境）
	ProjectID    string            // GCP 项目 ID（适用于需要选择项目的提供商）
	CallbackPort int               // OAuth 回调服务的监听端口
	Metadata     map[string]string // 提供商特定的额外参数键值对
	Prompt       func(prompt string) (string, error) // 用于交互式提示用户输入的回调函数
}

// Authenticator 定义了认证器的公共接口，管理登录和可选的令牌刷新流程。
type Authenticator interface {
	// Provider 返回此认证器处理的提供商标识名称。
	Provider() string

	// Login 执行登录流程，获取或更新认证凭据。
	//
	// 参数:
	//   - ctx: 请求上下文
	//   - cfg: 应用配置
	//   - opts: 登录选项
	//
	// 返回:
	//   - *coreauth.Auth: 认证结果
	//   - error: 登录失败时返回错误信息
	Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error)

	// RefreshLead 返回令牌刷新应提前执行的时间间隔。
	// 返回 nil 表示不支持自动刷新。
	RefreshLead() *time.Duration
}
