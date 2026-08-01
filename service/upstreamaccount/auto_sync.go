// auto_sync.go — 上游同步渠道的后台自动同步编排
// 职责：扫描符合条件的渠道，复用已保存凭据调用现有刷新流程，并汇总每个渠道
// 的成功、跳过和失败结果。
//
// 自动任务只负责调度，不改变 RefreshChannelFromCredential 的业务边界。刷新函数
// 仍然负责事务、余额快照、账号 upsert、缺失 key 禁用和渠道缓存重建。
package upstreamaccount

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
)

const upstreamAccountAutoSyncChannelTimeout = 5 * time.Minute

// UpstreamAccountSyncSummary 是一次全局自动同步任务的可观察结果。
type UpstreamAccountSyncSummary struct {
	ScannedChannels   int                          `json:"scanned_channels"`
	EligibleChannels  int                          `json:"eligible_channels"`
	SkippedChannels   int                          `json:"skipped_channels"`
	SucceededChannels int                          `json:"succeeded_channels"`
	FailedChannels    int                          `json:"failed_channels"`
	CreatedAccounts   int                          `json:"created_accounts"`
	UpdatedAccounts   int                          `json:"updated_accounts"`
	DisabledAccounts  int                          `json:"disabled_accounts"`
	Skipped           bool                         `json:"skipped,omitempty"`
	SkipReason        string                       `json:"skip_reason,omitempty"`
	Failures          []UpstreamAccountSyncFailure `json:"failures,omitempty"`
}

// UpstreamAccountSyncFailure 记录单个渠道的失败信息。
//
// 这里只保存渠道标识和错误文本，绝不保存解密后的密码、session 或完整 API Key。
type UpstreamAccountSyncFailure struct {
	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Error       string `json:"error"`
}

// RunUpstreamAccountSync 扫描并刷新所有符合条件的上游同步渠道。
//
// 单个渠道失败后继续处理后续渠道；只有数据库扫描失败或任务上下文取消等整体
// 错误才通过 error 返回。渠道级失败会保留在 summary.Failures 中，由 SystemTask
// handler 根据失败数量将任务标记为 failed。
func RunUpstreamAccountSync(ctx context.Context, report func(processed, total int)) (*UpstreamAccountSyncSummary, error) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return nil, err
	}

	summary := &UpstreamAccountSyncSummary{
		ScannedChannels: len(channels),
		Failures:        make([]UpstreamAccountSyncFailure, 0),
	}
	if report != nil {
		report(0, len(channels))
	}

	for index, channel := range channels {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if channel == nil {
			summary.SkippedChannels++
			if report != nil {
				report(index+1, len(channels))
			}
			continue
		}
		if channel.Status != common.ChannelStatusEnabled || !channel.HasUpstreamAccountSyncMetadata() {
			summary.SkippedChannels++
			if report != nil {
				report(index+1, len(channels))
			}
			continue
		}

		credential, ok, err := ReadChannelSyncCredential(channel.OtherSettings)
		if err != nil {
			appendUpstreamAccountSyncFailure(summary, channel, err)
			if report != nil {
				report(index+1, len(channels))
			}
			continue
		}
		if !ok {
			summary.SkippedChannels++
			if report != nil {
				report(index+1, len(channels))
			}
			continue
		}

		summary.EligibleChannels++
		accounts, err := automaticAccountConfigs(channel.Id)
		if err != nil {
			appendUpstreamAccountSyncFailure(summary, channel, err)
			if report != nil {
				report(index+1, len(channels))
			}
			continue
		}

		channelCtx, cancel := context.WithTimeout(ctx, upstreamAccountAutoSyncChannelTimeout)
		result, refreshErr := RefreshChannelFromCredential(channelCtx, RefreshRequest{
			ChannelID:         channel.Id,
			Credential:        credential,
			Accounts:          accounts,
			ApplySuggested:    true,
			DisableMissingKey: true,
		})
		cancel()
		if refreshErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return summary, ctxErr
			}
			appendUpstreamAccountSyncFailure(summary, channel, refreshErr)
		} else {
			summary.SucceededChannels++
			if result != nil {
				summary.CreatedAccounts += result.Created
				summary.UpdatedAccounts += result.Updated
				summary.DisabledAccounts += result.Disabled
			}
		}
		if report != nil {
			report(index+1, len(channels))
		}
	}

	return summary, nil
}

func appendUpstreamAccountSyncFailure(summary *UpstreamAccountSyncSummary, channel *model.Channel, err error) {
	summary.FailedChannels++
	summary.Failures = append(summary.Failures, UpstreamAccountSyncFailure{
		ChannelID:   channel.Id,
		ChannelName: channel.Name,
		Error:       strings.TrimSpace(err.Error()),
	})
}

// automaticAccountConfigs 读取当前账号池的本地配置，供刷新时覆盖上游建议值。
//
// 不传 Enabled 指针是有意为之：RefreshChannelFromSnapshot 在没有显式状态时会
// 保留账号当前的完整状态，包括手动禁用、冷却和其他运行态；priority、weight、
// model、group 则通过非 nil 配置指针明确保留本地管理员设置。
func automaticAccountConfigs(channelID int) ([]AccountCreateConfig, error) {
	var accounts []model.ChannelAccount
	if err := model.DB.Where("channel_id = ?", channelID).Find(&accounts).Error; err != nil {
		return nil, err
	}

	configs := make([]AccountCreateConfig, 0, len(accounts))
	for index := range accounts {
		account := &accounts[index]
		metadata := readAccountSyncMetadata(account.OtherSettings)
		syncID := strings.TrimSpace(metadata.ExternalID)
		if syncID == "" {
			// 历史账号可能没有 external_id；ApplySyncIDs 会使用同样的脱敏 key
			// 作为稳定标识，因此这里使用脱敏 key 继续匹配而不暴露完整 key。
			syncID = strings.TrimSpace(maskKey(account.Key))
		}
		priority := account.Priority
		weight := account.Weight
		configs = append(configs, AccountCreateConfig{
			SyncID:     syncID,
			ExternalID: strings.TrimSpace(metadata.ExternalID),
			Models:     account.Models,
			Group:      account.Group,
			Priority:   &priority,
			Weight:     &weight,
		})
	}
	return configs, nil
}

// AutomaticSyncFailureError 将部分失败转换为任务终态错误，同时保留完整 summary。
func AutomaticSyncFailureError(summary *UpstreamAccountSyncSummary) error {
	if summary == nil || summary.FailedChannels == 0 {
		return nil
	}
	return fmt.Errorf("上游账号自动同步有 %d 个渠道失败", summary.FailedChannels)
}
