// models.go
// 包 auth 提供各种 AI 服务提供商的认证功能。
// 该文件定义了令牌存储的接口，为不同提供商的认证实现提供统一的抽象。
package auth

// TokenStorage 定义了存储认证令牌的接口。
// 此接口的实现应提供将认证令牌持久化到文件系统位置的方法。
// 各个认证提供商（如 Claude、Codex、Antigravity 等）都实现了此接口。
type TokenStorage interface {
	// SaveTokenToFile 将认证令牌持久化到指定的文件路径。
	//
	// 参数：
	//   - authFilePath: 认证令牌应保存的文件路径
	//
	// 返回：
	//   - error: 保存操作失败时返回的错误，成功时返回 nil
	SaveTokenToFile(authFilePath string) error
}
