// Package common - totp.go
// 该文件实现了基于时间的一次性密码（TOTP）双因素认证功能
//
// TOTP（Time-based One-Time Password）是 RFC 6238 定义的标准双因素认证协议
// 用户使用认证器应用（如 Google Authenticator）扫描二维码后，可生成 6 位验证码
//
// 功能模块：
// - TOTP 密钥生成和验证
// - 备用恢复码生成和验证
// - 二维码数据生成
// - 暴力破解防护（失败次数限制和锁定）
//
// 安全设计：
// - 使用 crypto/rand 生成安全随机数
// - 备用码使用 bcrypt 哈希存储
// - 支持失败次数限制和临时锁定
package common

import (
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	// 备用码配置

	// BackupCodeLength 备用码长度（字符数）
	BackupCodeLength = 8
	// BackupCodeCount 生成的备用码数量
	BackupCodeCount = 4

	// 限制配置

	// MaxFailAttempts 最大失败尝试次数，超过后锁定账户
	MaxFailAttempts = 5
	// LockoutDuration 锁定持续时间（秒）
	LockoutDuration = 300
)

// GenerateTOTPSecret 生成 TOTP 密钥和配置
//
// 生成的密钥用于用户设置双因素认证时，生成二维码供认证器应用扫描
//
// TOTP 参数配置：
// - 发行者：系统名称（SystemName）
// - 有效期：30 秒
// - 位数：6 位
// - 算法：SHA-1（RFC 6238 标准）
//
// 参数：
//   - accountName: 用户账户名（通常是邮箱）
//
// 返回值：
//   - *otp.Key: TOTP 密钥对象，包含 Secret 和 URL
//   - error: 生成错误
func GenerateTOTPSecret(accountName string) (*otp.Key, error) {
	issuer := Get2FAIssuer()
	return totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
}

// ValidateTOTPCode 验证 TOTP 验证码
//
// 验证流程：
// 1. 清理验证码格式（移除空格）
// 2. 检查长度是否为 6 位
// 3. 使用 totp.Validate 验证验证码是否有效
//
// totp.Validate 会自动验证当前时间窗口和前后一个时间窗口的验证码
// 这允许客户端和服务端存在少量时间偏差
//
// 参数：
//   - secret: TOTP 密钥（Base32 编码）
//   - code: 用户输入的 6 位验证码
//
// 返回值：
//   - bool: 验证码是否有效
func ValidateTOTPCode(secret, code string) bool {
	// 清理验证码格式（移除空格）
	cleanCode := strings.ReplaceAll(code, " ", "")
	if len(cleanCode) != 6 {
		return false
	}

	// 验证验证码
	return totp.Validate(cleanCode, secret)
}

// GenerateBackupCodes 生成备用恢复码
//
// 备用码用于用户丢失认证器设备时恢复账户访问
// 每个备用码只能使用一次，使用后应标记为已使用
//
// 返回值：
//   - []string: 备用码列表（格式：XXXX-XXXX）
//   - error: 生成错误
func GenerateBackupCodes() ([]string, error) {
	codes := make([]string, BackupCodeCount)

	for i := 0; i < BackupCodeCount; i++ {
		code, err := generateRandomBackupCode()
		if err != nil {
			return nil, err
		}
		codes[i] = code
	}

	return codes, nil
}

// generateRandomBackupCode 生成单个随机备用码
//
// 生成流程：
// 1. 使用 crypto/rand 生成密码学安全的随机字节
// 2. 从字符集（A-Z, 0-9）中选取字符
// 3. 格式化为 XXXX-XXXX 格式
//
// 返回值：
//   - string: 备用码（格式：XXXX-XXXX）
//   - error: 随机数生成错误
func generateRandomBackupCode() (string, error) {
	// 字符集：大写字母 + 数字
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, BackupCodeLength)

	for i := range code {
		randomBytes := make([]byte, 1)
		_, err := rand.Read(randomBytes)
		if err != nil {
			return "", err
		}
		code[i] = charset[int(randomBytes[0])%len(charset)]
	}

	// 格式化为 XXXX-XXXX 格式
	return fmt.Sprintf("%s-%s", string(code[:4]), string(code[4:])), nil
}

// ValidateBackupCode 验证备用码格式是否合法
//
// 验证规则：
// - 移除分隔符后长度必须为 BackupCodeLength（8 位）
// - 只能包含大写字母和数字
//
// 参数：
//   - code: 用户输入的备用码
//
// 返回值：
//   - bool: 格式是否合法
func ValidateBackupCode(code string) bool {
	// 移除所有分隔符并转为大写
	cleanCode := strings.ToUpper(strings.ReplaceAll(code, "-", ""))
	if len(cleanCode) != BackupCodeLength {
		return false
	}

	// 检查字符是否合法（只允许 A-Z 和 0-9）
	for _, char := range cleanCode {
		if !((char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9')) {
			return false
		}
	}

	return true
}

// NormalizeBackupCode 标准化备用码格式
//
// 将用户输入的备用码统一为 XXXX-XXXX 格式
// 便于后续哈希比较
//
// 参数：
//   - code: 用户输入的备用码
//
// 返回值：
//   - string: 标准化后的备用码
func NormalizeBackupCode(code string) string {
	cleanCode := strings.ToUpper(strings.ReplaceAll(code, "-", ""))
	if len(cleanCode) == BackupCodeLength {
		return fmt.Sprintf("%s-%s", cleanCode[:4], cleanCode[4:])
	}
	return code
}

// HashBackupCode 对备用码进行哈希处理
//
// 使用 bcrypt 算法哈希，安全存储到数据库
// 验证时使用 bcrypt.CompareHashAndPassword 进行比较
//
// 参数：
//   - code: 备用码
//
// 返回值：
//   - string: 哈希后的备用码
//   - error: 哈希错误
func HashBackupCode(code string) (string, error) {
	normalizedCode := NormalizeBackupCode(code)
	return Password2Hash(normalizedCode)
}

// Get2FAIssuer 获取双因素认证的发行者名称
//
// 发行者名称显示在认证器应用中，用于区分不同服务的 TOTP
//
// 返回值：
//   - string: 发行者名称（系统名称）
func Get2FAIssuer() string {
	return SystemName
}

// getEnvOrDefault 获取环境变量值，如果不存在则返回默认值
//
// 参数：
//   - key: 环境变量名
//   - defaultValue: 默认值
//
// 返回值：
//   - string: 环境变量值或默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// ValidateNumericCode 验证数字验证码格式
//
// 验证规则：
// - 移除空格后长度必须为 6 位
// - 必须是纯数字
//
// 参数：
//   - code: 用户输入的验证码
//
// 返回值：
//   - string: 清理后的验证码
//   - error: 验证错误
func ValidateNumericCode(code string) (string, error) {
	// 移除空格
	code = strings.ReplaceAll(code, " ", "")

	if len(code) != 6 {
		return "", fmt.Errorf("验证码必须是6位数字")
	}

	// 检查是否为纯数字
	if _, err := strconv.Atoi(code); err != nil {
		return "", fmt.Errorf("验证码只能包含数字")
	}

	return code, nil
}

// GenerateQRCodeData 生成 TOTP 二维码数据
//
// 生成符合 otpauth:// URI 格式的二维码数据
// 用户使用认证器应用扫描此二维码即可添加 TOTP 账户
//
// URI 格式：otpauth://totp/{issuer}:{account}?secret={secret}&issuer={issuer}&digits=6&period=30
//
// 参数：
//   - secret: TOTP 密钥（Base32 编码）
//   - username: 用户名
//
// 返回值：
//   - string: otpauth:// 格式的 URI
func GenerateQRCodeData(secret, username string) string {
	issuer := Get2FAIssuer()
	accountName := fmt.Sprintf("%s (%s)", username, issuer)
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&digits=6&period=30",
		issuer, accountName, secret, issuer)
}
