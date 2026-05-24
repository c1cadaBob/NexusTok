// Package controller - billing.go
// 该文件实现了计费相关的 API 控制器
//
// 提供 OpenAI 兼容的计费查询接口：
// - GetSubscription：查询用户订阅信息和配额限制
// - GetUsage：查询用户已使用的额度
//
// 接口格式与 OpenAI 官方计费 API 保持一致
// 支持多种配额显示类型：USD、CNY、Tokens
package controller

import (
	“github.com/c1cada/NexusTok/common”
	“github.com/c1cada/NexusTok/model”
	“github.com/c1cada/NexusTok/setting/operation_setting”
	“github.com/c1cada/NexusTok/types”
	“github.com/gin-gonic/gin”
)

// GetSubscription 获取用户订阅信息
//
// 返回 OpenAI 兼容的订阅信息，包括：
// - 软限制（SoftLimitUSD）：建议的使用上限
// - 硬限制（HardLimitUSD）：强制的使用上限
// - 系统硬限制（SystemHardLimitUSD）：系统级限制
// - 访问有效期（AccessUntil）：Token 过期时间
//
// 根据配置，配额可能显示为：
// - USD：直接除以 QuotaPerUnit
// - CNY：先转 USD 再乘汇率
// - Tokens：直接使用 Token 数量
//
// 参数：
//   - c: Gin 上下文
func GetSubscription(c *gin.Context) {
	var remainQuota int
	var usedQuota int
	var err error
	var token *model.Token
	var expiredTime int64

	// 根据配置决定查询 Token 级别还是用户级别的配额
	if common.DisplayTokenStatEnabled {
		// Token 级别：查询指定 Token 的配额
		tokenId := c.GetInt(“token_id”)
		token, err = model.GetTokenById(tokenId)
		expiredTime = token.ExpiredTime
		remainQuota = token.RemainQuota
		usedQuota = token.UsedQuota
	} else {
		// 用户级别：查询用户的总配额
		userId := c.GetInt(“id”)
		remainQuota, err = model.GetUserQuota(userId, false)
		usedQuota, err = model.GetUserUsedQuota(userId)
	}

	if expiredTime <= 0 {
		expiredTime = 0
	}

	// 查询失败返回错误
	if err != nil {
		openAIError := types.OpenAIError{
			Message: err.Error(),
			Type:    “upstream_error”,
		}
		c.JSON(200, gin.H{
			“error”: openAIError,
		})
		return
	}

	// 计算总配额（剩余额度 + 已使用额度）
	quota := remainQuota + usedQuota
	amount := float64(quota)

	// 根据配额显示类型进行转换
	// OpenAI 兼容接口中的 *_USD 字段含义保持”额度单位”对应值：
	// - USD: 直接除以 QuotaPerUnit
	// - CNY: 先转 USD 再乘汇率
	// - TOKENS: 直接使用 tokens 数量
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		amount = amount / common.QuotaPerUnit * operation_setting.USDExchangeRate
	case operation_setting.QuotaDisplayTypeTokens:
		// amount 保持 tokens 数值
	default:
		amount = amount / common.QuotaPerUnit
	}

	// 无限配额的 Token 显示为固定大数值
	if token != nil && token.UnlimitedQuota {
		amount = 100000000
	}

	// 构建 OpenAI 兼容的订阅响应
	subscription := OpenAISubscriptionResponse{
		Object:             “billing_subscription”,
		HasPaymentMethod:   true,
		SoftLimitUSD:       amount,
		HardLimitUSD:       amount,
		SystemHardLimitUSD: amount,
		AccessUntil:        expiredTime,
	}
	c.JSON(200, subscription)
	return
}

// GetUsage 获取用户已使用的额度
//
// 返回 OpenAI 兼容的使用量信息
// TotalUsage 的单位为”分”（乘以 100）
//
// 参数：
//   - c: Gin 上下文
func GetUsage(c *gin.Context) {
	var quota int
	var err error
	var token *model.Token

	// 根据配置决定查询 Token 级别还是用户级别的使用量
	if common.DisplayTokenStatEnabled {
		tokenId := c.GetInt(“token_id”)
		token, err = model.GetTokenById(tokenId)
		quota = token.UsedQuota
	} else {
		userId := c.GetInt(“id”)
		quota, err = model.GetUserUsedQuota(userId)
	}

	// 查询失败返回错误
	if err != nil {
		openAIError := types.OpenAIError{
			Message: err.Error(),
			Type:    “nexustok_error”,
		}
		c.JSON(200, gin.H{
			“error”: openAIError,
		})
		return
	}

	// 根据配额显示类型进行转换
	amount := float64(quota)
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		amount = amount / common.QuotaPerUnit * operation_setting.USDExchangeRate
	case operation_setting.QuotaDisplayTypeTokens:
		// tokens 保持原值
	default:
		amount = amount / common.QuotaPerUnit
	}

	// 构建 OpenAI 兼容的使用量响应
	// 注意：TotalUsage 单位为”分”，需要乘以 100
	usage := OpenAIUsageResponse{
		Object:     “list”,
		TotalUsage: amount * 100,
	}
	c.JSON(200, usage)
	return
}
