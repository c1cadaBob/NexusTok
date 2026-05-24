// channel.go 实现了渠道的启用/禁用管理逻辑。
// 包括自动禁用发生错误的渠道、手动启用渠道、
// 以及判断渠道是否应该被禁用或启用的策略。
// 自动禁用功能基于错误状态码和关键词匹配来决定。
package service

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/types"
)

// formatNotifyType 格式化通知类型标识符。
// 格式：{通知类型}_{渠道ID}_{状态码}
func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

// DisableChannel 禁用发生错误的渠道。
// 如果启用了自动禁用功能（AutoBan），将渠道状态更新为自动禁用，
// 并向管理员发送通知。记录禁用原因到日志。
//
// 参数：
//   - channelError: 渠道错误信息（包含渠道 ID、名称等）
//   - reason: 禁用原因
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

// EnableChannel 启用指定的渠道。
// 将渠道状态更新为已启用，并向管理员发送通知。
//
// 参数：
//   - channelId: 渠道 ID
//   - usingKey: 渠道使用的密钥标识
//   - channelName: 渠道名称
func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

// ShouldDisableChannel 判断渠道是否应该被自动禁用。
// 判断逻辑：
// 1. 自动禁用功能必须已启用
// 2. 错误不能为 nil
// 3. 如果是渠道级错误（IsChannelError），直接禁用
// 4. 如果是跳过重试的错误（IsSkipRetryError），不禁用
// 5. 如果状态码在禁用列表中，禁用
// 6. 如果错误消息包含禁用关键词，禁用
func ShouldDisableChannel(err *types.NexusTokError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search
}

// ShouldEnableChannel 判断渠道是否应该被自动启用。
// 判断逻辑：
// 1. 自动启用功能必须已启用
// 2. 新的请求不能有错误
// 3. 渠道当前状态必须是自动禁用
func ShouldEnableChannel(newAPIError *types.NexusTokError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}
