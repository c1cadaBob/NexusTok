// Package common - hash.go
// 该文件提供了哈希和摘要算法的工具函数
//
// 支持的算法：
// - SHA-256: 用于数据完整性校验、缓存键生成等
// - SHA-1: 用于兼容旧系统（不推荐用于安全场景）
// - HMAC-SHA256: 用于消息认证码、Webhook 签名等
package common

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
)

// Sha256Raw 计算 SHA-256 哈希（返回原始字节）
//
// 参数：
//   - data: 要哈希的数据
//
// 返回值：
//   - []byte: 32 字节的 SHA-256 哈希值
func Sha256Raw(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

// Sha1Raw 计算 SHA-1 哈希（返回原始字节）
//
// 参数：
//   - data: 要哈希的数据
//
// 返回值：
//   - []byte: 20 字节的 SHA-1 哈希值
func Sha1Raw(data []byte) []byte {
	h := sha1.New()
	h.Write(data)
	return h.Sum(nil)
}

// Sha1 计算 SHA-1 哈希（返回十六进制字符串）
//
// 参数：
//   - data: 要哈希的数据
//
// 返回值：
//   - string: 40 字符的十六进制 SHA-1 哈希值
func Sha1(data []byte) string {
	return hex.EncodeToString(Sha1Raw(data))
}

// HmacSha256Raw 计算 HMAC-SHA256（返回原始字节）
//
// 参数：
//   - message: 要认证的消息
//   - key: HMAC 密钥
//
// 返回值：
//   - []byte: 32 字节的 HMAC-SHA256 值
func HmacSha256Raw(message, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(message)
	return h.Sum(nil)
}

// HmacSha256 计算 HMAC-SHA256（返回十六进制字符串）
//
// 参数：
//   - message: 要认证的消息
//   - key: HMAC 密钥
//
// 返回值：
//   - string: 64 字符的十六进制 HMAC-SHA256 值
func HmacSha256(message, key string) string {
	return hex.EncodeToString(HmacSha256Raw([]byte(message), []byte(key)))
}
