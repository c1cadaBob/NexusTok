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
	defaultContainerSessionSecretFile = "/data/session_secret"
	defaultLocalSessionSecretFile     = "data/session_secret"
	generatedSessionSecretBytes       = 48
)

var errDefaultSessionSecret = errors.New("SESSION_SECRET uses the unsafe default value")

// resolveSessionSecret 解析当前进程使用的会话密钥。
//
// 没有显式配置 SESSION_SECRET 时，密钥会落盘到持久化文件中；这样热更新或容器重启后
// cookie 签名密钥保持一致，用户不会因为进程重启而被迫重新登录。
func resolveSessionSecret() (string, error) {
	if secret := strings.TrimSpace(os.Getenv("SESSION_SECRET")); secret != "" {
		if secret == "random_string" {
			return "", errDefaultSessionSecret
		}
		return secret, nil
	}

	secretFile := getSessionSecretFile()
	if data, err := os.ReadFile(secretFile); err == nil {
		secret := strings.TrimSpace(string(data))
		if secret != "" {
			return secret, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read session secret file %q: %w", secretFile, err)
	}

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

// getSessionSecretFile 返回会话密钥持久化路径，容器内默认使用 /data 以配合 Docker volume。
func getSessionSecretFile() string {
	if secretFile := strings.TrimSpace(os.Getenv("SESSION_SECRET_FILE")); secretFile != "" {
		return secretFile
	}
	if IsRunningInContainer() {
		return defaultContainerSessionSecretFile
	}
	return defaultLocalSessionSecretFile
}

// generateSessionSecret 使用安全随机数生成足够长的 cookie 签名密钥。
func generateSessionSecret() (string, error) {
	buffer := make([]byte, generatedSessionSecretBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate session secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// resolveSessionMaxAge 从环境变量读取登录有效期，避免非法值误删浏览器 cookie。
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
