// Package common - secret.go
// 该文件实现了敏感数据的加密和解密功能
//
// 使用 AES-GCM（Galois/Counter Mode）加密算法
// AES-GCM 是一种认证加密算法，同时提供：
// - 机密性：数据被加密，无法被未授权方读取
// - 完整性：数据被篡改后解密会失败
// - 认证：确保数据来源可信
//
// 密钥派生：
// - 使用 SHA-256 从 CryptoSecret 派生 256 位 AES 密钥
// - CryptoSecret 在系统启动时随机生成
//
// 加密格式：
// - 前缀："enc:v1:"
// - 内容：Base64URL 编码的 (nonce + ciphertext)
//
// 使用场景：
// - 渠道 API Key 加密存储
// - OAuth Token 加密存储
// - 其他敏感配置加密
package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const encryptedSecretPrefix = "enc:v1:" // 加密数据前缀，用于标识已加密的数据

// secretCipher 创建 AES-GCM 密码器
//
// 使用 SHA-256 从 CryptoSecret 派生 256 位 AES 密钥
// 返回 AES-GCM 密码器，用于加密和解密
//
// 返回值：
//   - cipher.AEAD: AES-GCM 密码器
//   - error: 创建错误
func secretCipher() (cipher.AEAD, error) {
	sum := sha256.Sum256([]byte(CryptoSecret))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// EncryptSensitiveString 使用 CRYPTO_SECRET 派生密钥加密敏感数据
//
// 加密流程：
// 1. 如果数据为空或已加密，直接返回
// 2. 创建 AES-GCM 密码器
// 3. 生成随机 nonce（Number used ONCE）
// 4. 使用 AES-GCM 加密数据
// 5. 返回 "enc:v1:" + Base64URL(nonce + ciphertext)
//
// 参数：
//   - plain: 明文数据
//
// 返回值：
//   - string: 加密后的数据（带前缀）
//   - error: 加密错误
func EncryptSensitiveString(plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", nil
	}
	// 如果已加密，直接返回
	if strings.HasPrefix(plain, encryptedSecretPrefix) {
		return plain, nil
	}
	aead, err := secretCipher()
	if err != nil {
		return "", err
	}
	// 生成随机 nonce
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	// 加密数据
	ciphertext := aead.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)
	return encryptedSecretPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

// DecryptSensitiveString 解密敏感数据
//
// 解密流程：
// 1. 如果数据为空，返回空字符串
// 2. 如果数据未加密（无前缀），原样返回（兼容历史明文数据）
// 3. 创建 AES-GCM 密码器
// 4. Base64URL 解码 payload
// 5. 分离 nonce 和 ciphertext
// 6. 使用 AES-GCM 解密数据
//
// 参数：
//   - value: 加密后的数据（可能带前缀）
//
// 返回值：
//   - string: 解密后的明文数据
//   - error: 解密错误
func DecryptSensitiveString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	// 如果未加密，原样返回（兼容历史明文数据）
	if !strings.HasPrefix(value, encryptedSecretPrefix) {
		return value, nil
	}
	aead, err := secretCipher()
	if err != nil {
		return "", err
	}
	// Base64URL 解码 payload
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, encryptedSecretPrefix))
	if err != nil {
		return "", err
	}
	if len(payload) <= aead.NonceSize() {
		return "", errors.New("invalid encrypted secret payload")
	}
	// 分离 nonce 和 ciphertext
	nonce := payload[:aead.NonceSize()]
	ciphertext := payload[aead.NonceSize():]
	// 解密数据
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
