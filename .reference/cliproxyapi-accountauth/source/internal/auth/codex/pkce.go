// codex - pkce.go
// 包 codex 提供 OpenAI Codex API 的认证和令牌管理功能。
// 该文件实现了 PKCE（Proof Key for Code Exchange）代码生成功能，
// 用于 OAuth2 授权码流程的安全增强。
package codex

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// GeneratePKCECodes 生成一对新的 PKCE（Proof Key for Code Exchange）代码。
// 创建一个加密安全的随机代码验证器和对应的 SHA256 代码挑战码，
// 如 RFC 7636 所规定。这是 OAuth 2.0 授权码流程的关键安全特性。
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

// generateCodeVerifier 创建一个加密安全的随机字符串，用作 PKCE 流程中的代码验证器。
// 验证器是一个高熵字符串，用于证明发起授权请求的客户端的身份。
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

// generateCodeChallenge 从给定的代码验证器创建代码挑战码。
// 通过对验证器进行 SHA256 哈希，然后进行 Base64 URL 编码来派生挑战码。
// 这个挑战码在初始授权请求中发送，稍后与验证器进行验证。
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
