package model

import (
	"fmt"

	"github.com/c1cada/NexusTok/common"
)

const legacyCLIProxyAccountPoolGroupSource = "cliproxyapi"

// migrateLegacyCLIProxyAccountPoolGroups 将旧外部账号池镜像分组安全收敛为原生账号池数据。
//
// 迁移规则：
//  1. 如果旧分组下已经存在本地 PoolAccount，说明该分组具备主服务可调度的数据，直接转为
//     native 并清空 external_group_key，后续 Relay、检测和用量统计都走原生账号池路径。
//  2. 如果旧分组只是外部账号池镜像且没有本地账号，则禁用保留。这样不会误删历史记录，
//     也不会让已移除的外部系统入口继续出现在常规列表或运行路径里。
func migrateLegacyCLIProxyAccountPoolGroups() error {
	if DB == nil {
		return nil
	}
	var groups []AccountPoolGroup
	if err := DB.Select("id", "source").Where("source = ?", legacyCLIProxyAccountPoolGroupSource).Find(&groups).Error; err != nil {
		return err
	}
	if len(groups) == 0 {
		return nil
	}

	now := common.GetTimestamp()
	converted := 0
	disabled := 0
	for _, group := range groups {
		var accountCount int64
		if err := DB.Model(&PoolAccount{}).Where("pool_group_id = ?", group.Id).Count(&accountCount).Error; err != nil {
			return err
		}
		if accountCount > 0 {
			tx := DB.Model(&AccountPoolGroup{}).
				Where("id = ? AND source = ?", group.Id, legacyCLIProxyAccountPoolGroupSource).
				UpdateColumns(map[string]interface{}{
					"source":             AccountPoolGroupSourceNative,
					"external_group_key": "",
					"updated_time":       now,
				})
			if tx.Error != nil {
				return tx.Error
			}
			converted += int(tx.RowsAffected)
			continue
		}
		tx := DB.Model(&AccountPoolGroup{}).
			Where("id = ? AND source = ?", group.Id, legacyCLIProxyAccountPoolGroupSource).
			UpdateColumns(map[string]interface{}{
				"status":       common.ChannelStatusManuallyDisabled,
				"updated_time": now,
			})
		if tx.Error != nil {
			return tx.Error
		}
		disabled += int(tx.RowsAffected)
	}
	if converted > 0 || disabled > 0 {
		common.SysLog(fmt.Sprintf("migrated legacy external account pool groups: converted=%d disabled=%d", converted, disabled))
	}
	return nil
}
