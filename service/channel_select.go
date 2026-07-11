// Package service - channel_select.go
// 该文件实现了渠道选择相关服务
// 负责根据模型、分组、重试策略等选择合适的渠道
//
// 渠道选择策略：
// 1. 普通模式：根据 token 分组和模型名称选择渠道
// 2. Auto 模式：自动在多个分组间切换，支持跨分组重试
package service

import (
	"errors"

	"github.com/c1cada/NexusTok/common"   // 公共工具包
	"github.com/c1cada/NexusTok/constant" // 常量定义
	"github.com/c1cada/NexusTok/logger"   // 日志
	"github.com/c1cada/NexusTok/model"    // 数据模型
	"github.com/c1cada/NexusTok/setting"  // 设置

	"github.com/gin-gonic/gin" // Gin 框架
)

// RetryParam 重试参数结构体
// 用于跟踪重试状态和控制重试逻辑
type RetryParam struct {
	Ctx          *gin.Context // Gin 上下文
	TokenGroup   string       // Token 分组
	ModelName    string       // 模型名称
	RequestPath  string       // 当前请求路径，用于 Advanced Custom 按入口路径过滤候选渠道
	Retry        *int         // 当前重试次数
	resetNextTry bool         // 是否在下次重试时重置计数
}

// GetRetry 获取当前重试次数
//
// 返回值：
//   - int: 当前重试次数，如果为 nil 返回 0
func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

// SetRetry 设置重试次数
//
// 参数：
//   - retry: 新的重试次数
func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

// IncreaseRetry 增加重试次数
// 如果设置了 resetNextTry 标志，则不增加（用于跨分组重试场景）
func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

// ResetRetryNextTry 设置在下次重试时重置计数
// 用于跨分组重试场景：切换分组时重置重试计数
func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

// CacheGetRandomSatisfiedChannel 获取满足要求的随机渠道
// 尝试获取一个满足要求的随机渠道。
//
// 对于 "auto" tokenGroup 且启用了跨分组重试：
//   - 每个分组会用完所有优先级后才会切换到下一个分组
//   - 使用 ContextKeyAutoGroupIndex 跟踪当前分组索引
//   - 使用 ContextKeyAutoGroupRetryIndex 跟踪当前分组开始时的全局重试次数
//   - priorityRetry = Retry - startRetryIndex，表示当前分组内的优先级级别
//   - 当 GetRandomSatisfiedChannel 返回 nil（优先级用完）时，切换到下一个分组
//
// 示例流程（2个分组，每个有2个优先级，RetryTimes=3）：
//
//	Retry=0: GroupA, priority0 (startRetryIndex=0, priorityRetry=0)
//	         分组A, 优先级0
//
//	Retry=1: GroupA, priority1 (startRetryIndex=0, priorityRetry=1)
//	         分组A, 优先级1
//
//	Retry=2: GroupA exhausted → GroupB, priority0 (startRetryIndex=2, priorityRetry=0)
//	         分组A用完 → 分组B, 优先级0
//
//	Retry=3: GroupB, priority1 (startRetryIndex=2, priorityRetry=1)
//	         分组B, 优先级1
//
// 参数：
//   - param: 重试参数
//
// 返回值：
//   - *model.Channel: 选中的渠道
//   - string: 选中的分组
//   - error: 错误信息
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	// Auto 模式：自动在多个分组间切换
	if param.TokenGroup == "auto" {
		// 检查是否配置了 auto 分组
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}

		// 获取用户的 auto 分组列表
		autoGroups := GetUserAutoGroup(userGroup)

		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		// 获取上次的分组索引
		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}

		// 遍历分组寻找可用渠道
		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]

			// 计算当前分组的 priorityRetry
			priorityRetry := param.GetRetry()

			// 如果切换到新分组，重置 priorityRetry
			if i > startGroupIndex {
				priorityRetry = 0
			}

			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

			// 获取满足要求的随机渠道
			channel, _ = model.GetRandomSatisfiedChannel(autoGroup, param.ModelName, priorityRetry, param.RequestPath)

			if channel == nil {
				// 当前分组没有该模型的可用渠道，尝试下一个分组
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)

				// 重置状态以尝试下一个分组
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)

				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				continue
			}

			// 找到可用渠道
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			// 为下一次重试准备状态
			if crossGroupRetry && priorityRetry >= common.RetryTimes {
				// 当前分组已用完所有重试次数，准备切换到下一个分组
				// 本次请求仍使用当前分组，但下次重试将使用下一个分组
				logger.LogDebug(param.Ctx, "Current group %s retries exhausted (priorityRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", autoGroup, priorityRetry, common.RetryTimes)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)

				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				// 保持在当前分组，保存当前状态
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}

			break
		}
	} else {
		// 普通模式：直接根据 token 分组和模型名称获取渠道
		channel, err = model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, param.GetRetry(), param.RequestPath)
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}

	return channel, selectGroup, nil
}
