// auth - interfaces.go
// 本文件定义了 CLI Proxy API SDK 认证子系统的核心接口和通用类型。
// 包含登录选项结构体 LoginOptions 和认证器接口 Authenticator，
// 所有第三方提供商（如 Claude、Codex、xAI 等）的认证实现均需实现该接口。
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// ErrRefreshNotSupported 表示当前认证提供者不支持令牌刷新操作。
// 当调用方尝试对不支持刷新的提供者执行刷新时，应返回此错误。
var ErrRefreshNotSupported = errors.New("cliproxy auth: refresh not supported")

// LoginOptions 封装了各认证器通用的登录配置选项。
// 提供商特定的逻辑可以通过 Metadata 字段传递额外参数。
type LoginOptions struct {
	// NoBrowser 指示是否禁止自动打开浏览器。
	// 当为 true 时，仅在终端打印认证 URL，由用户手动访问。
	NoBrowser bool

	// ProjectID 是可选的项目标识符，用于需要指定 GCP 项目 ID 的提供商（如 Gemini）。
	ProjectID string

	// CallbackPort 指定 OAuth 回调监听的本地端口号。
	// 当为 0 时，使用各认证器的默认端口。
	CallbackPort int

	// Metadata 是一个键值对映射，用于传递提供商特定的附加参数。
	// 例如 Codex 设备流登录模式通过 Metadata["codex_login_mode"] = "device" 来启用。
	Metadata map[string]string

	// Prompt 是一个可选的交互式输入回调函数。
	// 当浏览器不可用或回调超时时，认证器可通过该函数提示用户手动输入回调 URL 或令牌。
	// 函数接收提示字符串，返回用户输入内容和可能的错误。
	Prompt func(prompt string) (string, error)
}

// Authenticator 定义了认证器的核心接口。
// 每个第三方 AI 提供商（如 Claude、Codex、xAI、Gemini 等）都需要实现此接口，
// 以提供统一的登录和令牌刷新前置时间查询能力。
type Authenticator interface {
	// Provider 返回该认证器对应的提供商标识字符串。
	// 该标识用于在 Manager 中注册和查找认证器，例如 "claude"、"codex"、"xai"。
	Provider() string

	// Login 执行该提供商的完整登录流程，包括启动 OAuth 回调服务器、
	// 生成授权 URL、等待回调、交换令牌等步骤，最终返回认证记录。
	// 参数说明：
	//   - ctx: 上下文，用于控制请求超时和取消
	//   - cfg: 全局配置，包含代理设置、认证目录等
	//   - opts: 登录选项，可为 nil 使用默认值
	// 返回值为包含令牌存储和元数据的认证记录，或错误信息。
	Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error)

	// RefreshLead 返回令牌到期前应提前执行刷新的时间间隔。
	// 返回 nil 表示该认证器不支持令牌刷新。
	// 例如 Claude 返回 4 小时，表示应在令牌到期前 4 小时开始刷新。
	RefreshLead() *time.Duration
}
