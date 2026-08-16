package upstreamaccount

import (
	"context"
	"errors"
	"sort"

	"github.com/c1cada/NexusTok/model"
	"gorm.io/gorm"
)

// ScheduleRefreshSummary 是一次同步密钥调度建议统一刷新的结果。
type ScheduleRefreshSummary struct {
	ScannedAccounts  int   `json:"scanned_accounts"`
	UpdatedAccounts  int   `json:"updated_accounts"`
	AffectedChannels int   `json:"affected_channels"`
	SkippedAccounts  int   `json:"skipped_accounts"`
	ChannelIDs       []int `json:"channel_ids,omitempty"`
}

// RefreshSyncedAccountSchedulingSuggestions 统一刷新历史同步密钥的调度建议。
//
// 该维护动作只扫描带 upstream_account_sync metadata 的 ChannelAccount：
//   - priority 统一写入 0，完成本次管理员确认的口径迁移；
//   - weight 按 metadata 中保存的 ratio_conversion / effective_ratio / group_ratio /
//     model_ratios 重新计算，无有效倍率时回落 100；
//   - 普通账号池、多 Key 普通渠道不受影响。
//
// 函数只返回受影响渠道 ID，不直接刷新渠道缓存或代理客户端缓存，避免 service 子包反向依赖
// 顶层 service。调用方应在事务提交后按这些渠道重建能力并刷新缓存。
func RefreshSyncedAccountSchedulingSuggestions(ctx context.Context, report func(processed, total int)) (*ScheduleRefreshSummary, map[int]struct{}, error) {
	var accounts []model.ChannelAccount
	if err := model.DB.WithContext(ctx).
		Where("settings LIKE ?", "%"+upstreamAccountSyncMetadataKey+"%").
		Order("id ASC").
		Find(&accounts).Error; err != nil {
		return nil, nil, err
	}

	summary := &ScheduleRefreshSummary{ScannedAccounts: len(accounts)}
	affected := map[int]struct{}{}
	total := len(accounts)
	if report != nil {
		report(0, total)
	}

	for index := range accounts {
		if err := ctx.Err(); err != nil {
			return summary, affected, err
		}
		account := accounts[index]
		if !HasAccountSyncMetadata(account.OtherSettings) {
			summary.SkippedAccounts++
			if report != nil {
				report(index+1, total)
			}
			continue
		}

		weight := ManagedWeightForAccountSettings(account.OtherSettings)
		if err := model.DB.WithContext(ctx).Model(&model.ChannelAccount{}).
			Where("id = ?", account.Id).
			Updates(map[string]any{
				"priority": int64(0),
				"weight":   weight,
			}).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				summary.SkippedAccounts++
				if report != nil {
					report(index+1, total)
				}
				continue
			}
			return summary, affected, err
		}
		summary.UpdatedAccounts++
		if account.ChannelId > 0 {
			affected[account.ChannelId] = struct{}{}
		}
		if report != nil {
			report(index+1, total)
		}
	}

	summary.ChannelIDs = sortedChannelIDSet(affected)
	summary.AffectedChannels = len(summary.ChannelIDs)
	return summary, affected, nil
}

// ManagedWeightForAccountSettings 从账号同步 metadata 计算同步托管权重。
//
// 历史账号没有快照倍率时返回 100。该行为与新同步密钥的缺省权重一致，既不会把未知成本
// 当成低价 key，也不会因为空 metadata 把账号权重降到不可用。
func ManagedWeightForAccountSettings(settings string) int {
	ratio, ok := accountMinimumRatioFromSettings(settings)
	if !ok {
		return 100
	}
	return SuggestedWeightForRatio(ratio)
}

func sortedChannelIDSet(values map[int]struct{}) []int {
	if len(values) == 0 {
		return nil
	}
	ids := make([]int, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}
