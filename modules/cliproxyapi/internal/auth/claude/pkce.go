// claude - pkce.go
// 提供 Claude/Anthropic OAuth2 PKCE（Proof Key for Code Exchange）码对生成功能，
// 用于 OAuth2 授权码流程的安全增强，遵循 RFC 7636 规范。
package claude

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// GeneratePKCECodes 生成符合 RFC 7636 规范的 PKCE CodeVerifier/CodeChallenge 码对。
// 通过确保只有发起请求的客户端才能交换授权码，为 OAuth 流程提供额外安全性。
//
// 返回值:
//   - *PKCECodes: 包含 CodeVerifier 和 CodeChallenge 的结构体
//   - error: 生成失败时返回错误，成功时返回 nil
func GeneratePKCECodes() (*PKCECodes, error) {
	// 生成 CodeVerifier：43-128 个字符，URL 安全
	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}

	// 使用 S256 方法生成 CodeChallenge
	codeChallenge := generateCodeChallenge(codeVerifier)

	return &PKCECodes{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
	}, nil
}

// generateCodeVerifier 创建一个 128 字符的加密随机字符串，使用 URL 安全 Base64 编码。
func generateCodeVerifier() (string, error) {
	// 生成 96 字节随机数据（编码后产生 128 个 Base64 字符）
	bytes := make([]byte, 96)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// 编码为 URL 安全 Base64，无填充
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes), nil
}

// generateCodeChallenge 对 CodeVerifier 进行 SHA256 哈希，并使用 URL 安全 Base64 编码（无填充）。
func generateCodeChallenge(codeVerifier string) string {
	hash := sha256.Sum256([]byte(codeVerifier))
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
}
