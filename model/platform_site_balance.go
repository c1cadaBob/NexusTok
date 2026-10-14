/*
Copyright (C) 2023-2026 c1cadaBob

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cadabob.dev
*/
package model

import (
	"fmt"
	"math"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/shopspring/decimal"
)

// PlatformSiteBalanceSummary 是管理员可见的平台站点本地余额快照汇总。
type PlatformSiteBalanceSummary struct {
	Quota        int64 `json:"quota"`
	AccountCount int   `json:"account_count"`
	LastSyncAt   int64 `json:"last_sync_at"`
}

// GetPlatformSiteBalanceSummary 按平台账号各统计一次 NewAPI/Sub2API 余额。
// 查询刻意包含已禁用账号，因为其本地保存的最近余额仍属于管理员账号视图。
func GetPlatformSiteBalanceSummary() (*PlatformSiteBalanceSummary, error) {
	if !isValidQuotaPerUnit() {
		return nil, fmt.Errorf("系统额度单位无效")
	}
	if DB == nil {
		return nil, fmt.Errorf("主数据库未初始化")
	}

	var accounts []PlatformSiteAccount
	if err := DB.Where("platform IN ?", []string{
		PlatformNewAPI,
		PlatformSub2API,
	}).Order("channel_id ASC").Find(&accounts).Error; err != nil {
		return nil, err
	}

	summary := &PlatformSiteBalanceSummary{
		AccountCount: len(accounts),
	}
	totalQuota := decimal.Zero
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	for i := range accounts {
		account := &accounts[i]
		if account.LastSyncAt > summary.LastSyncAt {
			summary.LastSyncAt = account.LastSyncAt
		}
		if math.IsNaN(account.Balance) ||
			math.IsInf(account.Balance, 0) ||
			account.Balance < 0 {
			common.SysError(fmt.Sprintf(
				"平台站点余额无效，已跳过: channel_id=%d",
				account.ChannelID,
			))
			continue
		}
		totalQuota = totalQuota.Add(
			decimal.NewFromFloat(account.Balance).Mul(quotaPerUnit),
		)
	}

	quota, err := common.WalletQuotaFromDecimalStrict(totalQuota)
	if err != nil {
		return nil, err
	}
	summary.Quota = int64(quota)
	return summary, nil
}

func isValidQuotaPerUnit() bool {
	return common.QuotaPerUnit > 0 &&
		!math.IsNaN(common.QuotaPerUnit) &&
		!math.IsInf(common.QuotaPerUnit, 0)
}
