// Package common - crypto.go
// 该文件提供了加密和哈希相关的工具函数
//
// 包含的功能：
// - HMAC-SHA256: 用于 Webhook 签名、请求验证等
// - bcrypt: 用于密码哈希和验证
package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

// GenerateHMACWithKey 使用指定密钥生成 HMAC-SHA256 签名
//
// HMAC（Hash-based Message Authentication Code）是一种基于哈希的消息认证码
// 使用 SHA-256 作为哈希函数，生成 64 字符的十六进制签名
//
// 使用场景：
// - Webhook 请求签名验证
// - API 请求签名
// - 数据完整性校验
//
// 参数：
//   - key: HMAC 密钥（字节切片）
//   - data: 要签名的数据
//
// 返回值：
//   - string: 十六进制编码的 HMAC-SHA256 签名
func GenerateHMACWithKey(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateHMAC 使用系统默认密钥生成 HMAC-SHA256 签名
//
// 使用全局 CryptoSecret 作为 HMAC 密钥
// CryptoSecret 在系统启动时随机生成
//
// 参数：
//   - data: 要签名的数据
//
// 返回值：
//   - string: 十六进制编码的 HMAC-SHA256 签名
func GenerateHMAC(data string) string {
	h := hmac.New(sha256.New, []byte(CryptoSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// Password2Hash 将明文密码转换为 bcrypt 哈希
//
// bcrypt 是一种密码哈希函数，具有以下特点：
// - 自适应：可以通过调整 cost 参数增加计算时间
// - 盐值：每次哈希都使用随机盐值，防止彩虹表攻击
// - 慢速：故意设计为计算密集型，防止暴力破解
//
// 使用 bcrypt.DefaultCost（10）作为 cost 参数
//
// 参数：
//   - password: 明文密码
//
// 返回值：
//   - string: bcrypt 哈希字符串（60 字符）
//   - error: 哈希错误
func Password2Hash(password string) (string, error) {
	passwordBytes := []byte(password)
	hashedPassword, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	return string(hashedPassword), err
}

// ValidatePasswordAndHash 验证明文密码是否与 bcrypt 哈希匹配
//
// 使用 bcrypt.CompareHashAndPassword 进行安全比较
// 该函数使用常量时间比较，防止时序攻击
//
// 参数：
//   - password: 明文密码
//   - hash: bcrypt 哈希字符串
//
// 返回值：
//   - bool: 密码是否匹配
func ValidatePasswordAndHash(password string, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
