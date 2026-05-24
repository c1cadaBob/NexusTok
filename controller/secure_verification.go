// Package controller - secure_verification.go
// 该文件实现了通用安全验证的 API 控制器
//
// 安全验证用于敏感操作（如查看密钥、修改安全设置）的身份确认
// 支持两种验证方式：
// - 2FA（TOTP 验证码）
// - Passkey（WebAuthn 生物识别/安全密钥）
//
// 验证状态通过会话管理，有效期为 5 分钟
//
// 主要 API：
// - UniversalVerify：通用验证接口
package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// 会话键常量
const (
	SecureVerificationSessionKey       = "secure_verified_at"      // 安全验证完成时间戳
	secureVerificationMethodSessionKey = "secure_verified_method"  // 验证方式
	secureVerificationMethod2FA        = "2fa"                     // 2FA 验证方式
	secureVerificationMethodPasskey    = "passkey"                 // Passkey 验证方式
	PasskeyReadySessionKey             = "secure_passkey_ready_at" // Passkey 验证就绪标记
	SecureVerificationTimeout          = 300                       // 验证有效期（秒）
	PasskeyReadyTimeout                = 60                        // Passkey 就绪标记有效期（秒）
)

// UniversalVerifyRequest 通用验证请求结构体
type UniversalVerifyRequest struct {
	Method string `json:"method"`              // 验证方式："2fa" 或 "passkey"
	Code   string `json:"code,omitempty"`      // TOTP 验证码（2FA 方式时必填）
}

// VerificationStatusResponse 验证状态响应结构体
type VerificationStatusResponse struct {
	Verified  bool  `json:"verified"`           // 是否已验证
	ExpiresAt int64 `json:"expires_at,omitempty"` // 过期时间戳
}

// UniversalVerify 通用验证接口
//
// 支持 2FA 和 Passkey 验证，验证成功后在 session 中记录时间戳
//
// 请求参数：
//   - method: 验证方式（"2fa" 或 "passkey"）
//   - code: TOTP 验证码（2FA 方式时必填）
func UniversalVerify(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未登录",
		})
		return
	}

	var req UniversalVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, fmt.Errorf("参数错误: %v", err))
		return
	}

	// 获取用户信息
	user := &model.User{Id: userId}
	if err := user.FillUserById(); err != nil {
		common.ApiError(c, fmt.Errorf("获取用户信息失败: %v", err))
		return
	}

	if user.Status != common.UserStatusEnabled {
		common.ApiError(c, fmt.Errorf("该用户已被禁用"))
		return
	}

	// 检查用户的验证方式
	twoFA, _ := model.GetTwoFAByUserId(userId)
	has2FA := twoFA != nil && twoFA.IsEnabled

	passkey, passkeyErr := model.GetPasskeyByUserID(userId)
	hasPasskey := passkeyErr == nil && passkey != nil

	if !has2FA && !hasPasskey {
		common.ApiError(c, fmt.Errorf("用户未启用2FA或Passkey"))
		return
	}

	// 根据验证方式进行验证
	var verified bool
	var verifyMethod string
	var err error

	switch req.Method {
	case "2fa":
		if !has2FA {
			common.ApiError(c, fmt.Errorf("用户未启用2FA"))
			return
		}
		if req.Code == "" {
			common.ApiError(c, fmt.Errorf("验证码不能为空"))
			return
		}
		verified = validateTwoFactorAuth(twoFA, req.Code)
		verifyMethod = "2FA"

	case "passkey":
		if !hasPasskey {
			common.ApiError(c, fmt.Errorf("用户未启用Passkey"))
			return
		}
		// Passkey branch only trusts the short-lived marker written by PasskeyVerifyFinish.
		verified, err = consumePasskeyReady(c)
		if err != nil {
			common.ApiError(c, fmt.Errorf("Passkey 验证状态异常: %v", err))
			return
		}
		if !verified {
			common.ApiError(c, fmt.Errorf("请先完成 Passkey 验证"))
			return
		}
		verifyMethod = "Passkey"

	default:
		common.ApiError(c, fmt.Errorf("不支持的验证方式: %s", req.Method))
		return
	}

	if !verified {
		common.ApiError(c, fmt.Errorf("验证失败，请检查验证码"))
		return
	}

	// 验证成功，在 session 中记录时间戳
	now, err := setSecureVerificationSession(c, req.Method)
	if err != nil {
		common.ApiError(c, fmt.Errorf("保存验证状态失败: %v", err))
		return
	}

	// 记录日志
	model.RecordLog(userId, model.LogTypeSystem, fmt.Sprintf("通用安全验证成功 (验证方式: %s)", verifyMethod))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "验证成功",
		"data": gin.H{
			"verified":   true,
			"expires_at": now + SecureVerificationTimeout,
		},
	})
}

// setSecureVerificationSession 设置安全验证会话状态
//
// 清除 Passkey 就绪标记，记录验证时间和方式
func setSecureVerificationSession(c *gin.Context, method string) (int64, error) {
	session := sessions.Default(c)
	session.Delete(PasskeyReadySessionKey)
	now := time.Now().Unix()
	session.Set(SecureVerificationSessionKey, now)
	session.Set(secureVerificationMethodSessionKey, method)
	if err := session.Save(); err != nil {
		return 0, err
	}
	return now, nil
}

// consumePasskeyReady 消费 Passkey 就绪标记
//
// 检查并消费 PasskeyVerifyFinish 设置的就绪标记
// 标记有效期为 60 秒，过期后不能使用
func consumePasskeyReady(c *gin.Context) (bool, error) {
	session := sessions.Default(c)
	readyAtRaw := session.Get(PasskeyReadySessionKey)
	if readyAtRaw == nil {
		return false, nil
	}

	readyAt, ok := readyAtRaw.(int64)
	if !ok {
		session.Delete(PasskeyReadySessionKey)
		_ = session.Save()
		return false, fmt.Errorf("无效的 Passkey 验证状态")
	}
	session.Delete(PasskeyReadySessionKey)
	if err := session.Save(); err != nil {
		return false, err
	}
	// Expired ready markers cannot be reused.
	if time.Now().Unix()-readyAt >= PasskeyReadyTimeout {
		return false, nil
	}
	return true, nil
}
