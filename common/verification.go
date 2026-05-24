// Package common - verification.go
// 该文件实现了验证码管理功能
//
// 用于邮箱验证、密码重置等场景的验证码生成、存储和验证
//
// 验证码生命周期：
// 1. 生成：GenerateVerificationCode 生成随机验证码
// 2. 注册：RegisterVerificationCodeWithKey 存储验证码（带用途标识）
// 3. 验证：VerifyCodeWithKey 验证用户输入的验证码
// 4. 删除：DeleteKey 验证成功后删除验证码
//
// 安全设计：
// - 验证码有效期限制（默认 10 分钟）
// - 存储容量限制（默认最多 10 个），超过时自动清理过期验证码
// - 使用互斥锁保护并发访问
// - 基于 UUID 生成随机验证码
package common

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// verificationValue 存储验证码及其创建时间
type verificationValue struct {
	// code 验证码内容
	code string
	// time 验证码创建时间，用于判断是否过期
	time time.Time
}

const (
	// EmailVerificationPurpose 邮箱验证用途标识
	EmailVerificationPurpose = "v"
	// PasswordResetPurpose 密码重置用途标识
	PasswordResetPurpose = "r"
)

// verificationMutex 保护 verificationMap 的并发访问
var verificationMutex sync.Mutex

// verificationMap 存储验证码映射
// 键格式：{purpose}{key}（如 "vuser@example.com"）
var verificationMap map[string]verificationValue

// verificationMapMaxSize 验证码存储的最大容量
// 超过此限制时会自动清理过期验证码
var verificationMapMaxSize = 10

// VerificationValidMinutes 验证码有效时间（分钟）
var VerificationValidMinutes = 10

// GenerateVerificationCode 生成指定长度的随机验证码
//
// 使用 UUID v4 生成随机字符串，移除连字符后截取指定长度
// 如果 length 为 0，返回完整的 32 位 UUID
//
// 参数：
//   - length: 验证码长度（0 表示完整 UUID）
//
// 返回值：
//   - string: 随机验证码
func GenerateVerificationCode(length int) string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	if length == 0 {
		return code
	}
	return code[:length]
}

// RegisterVerificationCodeWithKey 注册验证码到存储中
//
// 使用 {purpose}{key} 作为存储键，支持不同用途的验证码
// 如果存储超过最大容量，会自动清理过期验证码
//
// 参数：
//   - key: 关联的键（如邮箱地址）
//   - code: 验证码
//   - purpose: 用途标识（EmailVerificationPurpose 或 PasswordResetPurpose）
func RegisterVerificationCodeWithKey(key string, code string, purpose string) {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap[purpose+key] = verificationValue{
		code: code,
		time: time.Now(),
	}
	// 超过容量限制时清理过期验证码
	if len(verificationMap) > verificationMapMaxSize {
		removeExpiredPairs()
	}
}

// VerifyCodeWithKey 验证用户输入的验证码是否正确
//
// 验证流程：
// 1. 检查验证码是否存在
// 2. 检查验证码是否在有效期内
// 3. 比较验证码是否匹配
//
// 参数：
//   - key: 关联的键（如邮箱地址）
//   - code: 用户输入的验证码
//   - purpose: 用途标识
//
// 返回值：
//   - bool: 验证码是否有效
func VerifyCodeWithKey(key string, code string, purpose string) bool {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	value, okay := verificationMap[purpose+key]
	now := time.Now()
	// 检查是否存在且未过期
	if !okay || int(now.Sub(value.time).Seconds()) >= VerificationValidMinutes*60 {
		return false
	}
	return code == value.code
}

// DeleteKey 从存储中删除指定的验证码
//
// 验证成功后应调用此方法删除验证码，防止重复使用
//
// 参数：
//   - key: 关联的键
//   - purpose: 用途标识
func DeleteKey(key string, purpose string) {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	delete(verificationMap, purpose+key)
}

// removeExpiredPairs 清理过期的验证码
//
// 注意：此方法不获取锁，调用者必须在调用前持有 verificationMutex 锁
// 遍历整个存储，删除所有超过有效期的验证码
func removeExpiredPairs() {
	now := time.Now()
	for key := range verificationMap {
		if int(now.Sub(verificationMap[key].time).Seconds()) >= VerificationValidMinutes*60 {
			delete(verificationMap, key)
		}
	}
}

// init 初始化验证码存储映射
func init() {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap = make(map[string]verificationValue)
}
