// Package oauth - provider.go
// 该文件定义了 OAuth 提供商的统一接口
//
// 主要接口：
// - Provider：OAuth 提供商接口，定义了所有提供商必须实现的方法
//
// 接口方法：
// - GetName：获取提供商显示名称
// - IsEnabled：检查提供商是否启用
// - ExchangeToken：使用授权码换取访问令牌
// - GetUserInfo：使用访问令牌获取用户信息
// - IsUserIDTaken：检查提供商用户 ID 是否已被关联
// - FillUserByProviderID：通过提供商用户 ID 填充用户信息
// - SetProviderUserID：设置用户的提供商用户 ID
// - GetProviderPrefix：获取自动生成用户名的前缀
package oauth

import (
	"context"

	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
)

// Provider 定义了所有 OAuth 提供商必须实现的统一接口
// 通过该接口，系统可以以多态方式处理不同的 OAuth 提供商（如 GitHub、Discord、OIDC 等）
// 新增提供商只需实现此接口并在 init() 中注册即可
type Provider interface {
	// GetName 返回提供商的显示名称（如 "GitHub"、"Discord"、"OIDC"）
	// 用于前端展示和日志记录
	GetName() string

	// IsEnabled 检查当前提供商是否已启用
	// 如果提供商的配置不完整或管理员手动禁用，返回 false
	IsEnabled() bool

	// ExchangeToken 使用授权码（authorization code）换取访问令牌（access token）
	// 这是 OAuth 2.0 授权码流程的第二步
	// 参数：
	//   - ctx：请求上下文，用于超时控制和日志追踪
	//   - code：OAuth 授权码，由前端从提供商回调 URL 中获取
	//   - c：Gin 上下文，某些提供商需要从中获取请求信息（如构造 redirect_uri）
	// 返回值：
	//   - *OAuthToken：包含 access_token、refresh_token 等信息
	//   - error：交换失败时返回错误（可能是网络错误或授权码无效）
	ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error)

	// GetUserInfo 使用访问令牌从提供商获取用户信息
	// 这是 OAuth 流程的第三步：用 token 换取用户资料
	// 参数：
	//   - ctx：请求上下文
	//   - token：通过 ExchangeToken 获取的访问令牌
	// 返回值：
	//   - *OAuthUser：包含用户 ID、用户名、邮箱等信息
	//   - error：获取失败时返回错误
	GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error)

	// IsUserIDTaken 检查指定的提供商用户 ID 是否已被系统中的其他账号关联
	// 用于防止同一提供商账号被多个系统用户绑定
	// 参数：
	//   - providerUserID：提供商侧的用户唯一标识（如 GitHub 的数字 ID）
	// 返回值：如果已被占用返回 true
	IsUserIDTaken(providerUserID string) bool

	// FillUserByProviderID 通过提供商用户 ID 查找并填充用户模型
	// 如果找到匹配的用户绑定记录，将用户信息填充到传入的 user 指针中
	// 参数：
	//   - user：目标用户模型指针，找到后会被填充
	//   - providerUserID：提供商侧的用户唯一标识
	// 返回值：查找失败时返回错误
	FillUserByProviderID(user *model.User, providerUserID string) error

	// SetProviderUserID 将提供商用户 ID 设置到用户模型上
	// 用于新用户注册时，将提供商 ID 关联到用户账号
	// 对于内置提供商（如 GitHub），直接设置 User.GitHubId 字段
	// 对于自定义提供商（Generic），通过 OAuth 绑定表存储
	SetProviderUserID(user *model.User, providerUserID string)

	// GetProviderPrefix 返回自动生成用户名时使用的前缀
	// 当用户通过 OAuth 登录但系统需要自动生成用户名时使用
	// 例如 GitHub 返回 "github_"，Discord 返回 "discord_"
	GetProviderPrefix() string
}
