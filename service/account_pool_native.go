package service

import (
	"fmt"

	"github.com/c1cada/NexusTok/model"
)

// ensureNativeAccountPoolGroup 确认账号池分组属于 NexusTok 原生实现。
//
// 旧版本曾经把外部账号池镜像组写入 account_pool_groups。外部系统已经移除后，
// 所有运行入口都必须只处理 native 或历史空来源分组；迁移会把拥有本地账号的旧组转为
// native，仍残留的非原生来源代表不可运行的历史数据，不能继续参与检测、限流或 Relay 调度。
func ensureNativeAccountPoolGroup(group *model.AccountPoolGroup) error {
	if group == nil || model.IsNativeAccountPoolGroupSource(group.Source) {
		return nil
	}
	return fmt.Errorf("账号池分组不是原生来源")
}
