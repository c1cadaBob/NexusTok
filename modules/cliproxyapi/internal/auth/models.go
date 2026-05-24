// Package auth provides authentication functionality for various AI service providers.
// It includes interfaces and implementations for token storage and authentication methods.
//
// auth 包提供了多种 AI 服务商的认证功能。
// 包含令牌存储和认证方法的接口定义及实现。
package auth

// TokenStorage 定义了认证令牌的存储接口。
// 该接口的实现应提供将认证令牌持久化到文件系统的方法。
//
// TokenStorage defines the interface for storing authentication tokens.
// Implementations of this interface should provide methods to persist
// authentication tokens to a file system location.
type TokenStorage interface {
	// SaveTokenToFile 将认证令牌持久化到指定的文件路径。
	//
	// 参数:
	//   - authFilePath: 认证令牌的保存文件路径
	//
	// 返回值:
	//   - error: 保存失败时返回错误，成功时返回 nil
	//
	// SaveTokenToFile persists authentication tokens to the specified file path.
	SaveTokenToFile(authFilePath string) error
}
