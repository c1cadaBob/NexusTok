// Package common - session_secret.go
// 该文件实现了会话密钥（Session Secret）的管理
//
// 会话密钥用于：
// - JWT Token 签名和验证
// - Cookie 签名
// - 其他需要密钥的会话操作
//
// 密钥解析优先级：
// 1. SESSION_SECRET 环境变量（显式配置）
// 2. 持久化文件（自动生成并保存）
// 3. 新生成随机密钥（首次运行）
//
// 持久化文件路径：
// - 容器内：/data/session_secret
// - 本地：data/session_secret
// - 可通过 SESSION_SECRET_FILE 环境变量自定义
package common

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultContainerSessionSecretFile = "/data/session_secret"   // 容器内默认密钥文件路径
	defaultLocalSessionSecretFile     = "data/session_secret"    // 本地默认密钥文件路径
	generatedSessionSecretBytes       = 48                       // 生成的密钥字节数（48 字节 = 64 字符 Base64URL）
)

var errDefaultSessionSecret = errors.New("SESSION_SECRET uses the unsafe default value") // 使用不安全的默认值错误

// resolveSessionSecret 解析当前进程使用的会话密钥
//
// 解析流程：
// 1. 检查 SESSION_SECRET 环境变量
// 2. 如果未配置，尝试从持久化文件读取
// 3. 如果文件不存在，生成新密钥并保存到文件
//
// 设计目的：
// - 没有显式配置 SESSION_SECRET 时，密钥会落盘到持久化文件中
// - 这样热更新或容器重启后 cookie 签名密钥保持一致
// - 用户不会因为进程重启而被迫重新登录
//
// 返回值：
//   - string: 会话密钥
//   - error: 解析错误
func resolveSessionSecret() (string, error) {
	// 1. 检查环境变量
	if secret := strings.TrimSpace(os.Getenv("SESSION_SECRET")); secret != "" {
		if secret == "random_string" {
			return "", errDefaultSessionSecret
		}
		return secret, nil
	}

	// 2. 尝试从持久化文件读取
	secretFile := getSessionSecretFile()
	if data, err := os.ReadFile(secretFile); err == nil {
		secret := strings.TrimSpace(string(data))
		if secret != "" {
			return secret, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read session secret file %q: %w", secretFile, err)
	}

	// 3. 生成新密钥并保存
	secret, err := generateSessionSecret()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(secretFile), 0700); err != nil {
		return "", fmt.Errorf("create session secret dir %q: %w", filepath.Dir(secretFile), err)
	}
	if err := os.WriteFile(secretFile, []byte(secret+"\n"), 0600); err != nil {
		return "", fmt.Errorf("write session secret file %q: %w", secretFile, err)
	}
	return secret, nil
}

// getSessionSecretFile 返回会话密钥持久化路径
//
// 路径优先级：
// 1. SESSION_SECRET_FILE 环境变量
// 2. 容器内：/data/session_secret
// 3. 本地：data/session_secret
//
// 返回值：
//   - string: 密钥文件路径
func getSessionSecretFile() string {
	if secretFile := strings.TrimSpace(os.Getenv("SESSION_SECRET_FILE")); secretFile != "" {
		return secretFile
	}
	if IsRunningInContainer() {
		return defaultContainerSessionSecretFile
	}
	return defaultLocalSessionSecretFile
}

// generateSessionSecret 使用安全随机数生成会话密钥
//
// 生成 48 字节的随机数据，编码为 64 字符的 Base64URL 字符串
//
// 返回值：
//   - string: Base64URL 编码的密钥
//   - error: 生成错误
func generateSessionSecret() (string, error) {
	buffer := make([]byte, generatedSessionSecretBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate session secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// resolveSessionMaxAge 从环境变量读取登录有效期
//
// 如果环境变量未设置或无效，返回默认值
//
// 参数：
//   - defaultValue: 默认值（秒）
//
// 返回值：
//   - int: 会话最大存活时间（秒）
func resolveSessionMaxAge(defaultValue int) int {
	value := strings.TrimSpace(os.Getenv("SESSION_MAX_AGE"))
	if value == "" {
		return defaultValue
	}
	maxAge, err := strconv.Atoi(value)
	if err != nil || maxAge <= 0 {
		SysError(fmt.Sprintf("failed to parse SESSION_MAX_AGE: %s, using default value: %d", value, defaultValue))
		return defaultValue
	}
	return maxAge
}
