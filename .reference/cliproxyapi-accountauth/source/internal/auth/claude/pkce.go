// claude - pkce.go
// 包 claude 提供 Anthropic Claude API 的认证和令牌管理功能。
// 该文件实现了 PKCE（Proof Key for Code Exchange）代码生成功能，
// 用于 OAuth2 授权码流程的安全增强。
package claude

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// GeneratePKCECodes 生成 PKCE 代码验证器和挑战码对。
// 按照 RFC 7636 规范为 OAuth 2.0 PKCE 扩展生成代码对。
// 通过确保只有发起请求的客户端才能交换授权码，为 OAuth 流程提供额外安全性。
//
// 返回：
//   - *PKCECodes: 包含代码验证器和挑战码的结构体
//   - error: 生成失败时返回的错误
func GeneratePKCECodes() (*PKCECodes, error) {
	// 生成代码验证器：43-128 个字符，URL 安全
	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}

	// 使用 S256 方法生成代码挑战码
	codeChallenge := generateCodeChallenge(codeVerifier)

	return &PKCECodes{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
	}, nil
}

// generateCodeVerifier 创建一个加密安全的随机字符串。
// 生成 128 个字符的 URL 安全 base64 编码字符串，用作 PKCE 流程中的代码验证器。
//
// 返回：
//   - string: 生成的代码验证器
//   - error: 随机字节生成失败时返回的错误
func generateCodeVerifier() (string, error) {
	// 生成 96 个随机字节（将产生 128 个 base64 字符）
	bytes := make([]byte, 96)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// 编码为 URL 安全的 base64，不带填充
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes), nil
}

// generateCodeChallenge 从代码验证器创建代码挑战码。
// 对代码验证器进行 SHA256 哈希，然后使用 URL 安全的 base64 编码。
//
// 参数：
//   - codeVerifier: 代码验证器字符串
//
// 返回：
//   - string: 生成的代码挑战码
func generateCodeChallenge(codeVerifier string) string {
	hash := sha256.Sum256([]byte(codeVerifier))
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
}
