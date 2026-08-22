package model

import "strings"

// RoutingChannelAccountSupportsModel 判断渠道账号是否支持指定模型。
//
// 普通渠道账号保持历史兼容：账号模型为空时回退渠道模型，最终仍为空表示不限模型。
// 上游同步账号必须使用账号自己的显式模型白名单，空模型代表该同步密钥不参与任何
// Relay 路由，避免管理员清空模型后又被渠道聚合模型重新选中。
func RoutingChannelAccountSupportsModel(account *ChannelAccount, channel *Channel, modelName string) bool {
	if account == nil {
		return false
	}
	models := account.Models
	if channel != nil && channel.HasUpstreamAccountSyncMetadata() {
		if strings.TrimSpace(models) == "" {
			return false
		}
	} else if strings.TrimSpace(models) == "" && channel != nil {
		models = channel.Models
	}
	modelList := SplitCommaValues(models)
	if len(modelList) == 0 {
		return true
	}
	return MatchesModelList(modelList, modelName)
}

// RoutingChannelAccountSupportsGroup 判断渠道账号是否允许指定 NexusTok 用户组。
//
// 普通账号池中 ChannelAccount.group 表示 NexusTok 用户分组；上游同步渠道中该字段
// 表示上游平台自己的密钥分组，不能用于下游鉴权，此时必须使用 access_groups。
func RoutingChannelAccountSupportsGroup(account *ChannelAccount, channel *Channel, usingGroup string) bool {
	if account == nil {
		return false
	}
	if strings.TrimSpace(usingGroup) == "" {
		return true
	}
	group := ""
	if channel != nil && channel.HasUpstreamAccountSyncMetadata() {
		if strings.TrimSpace(account.AccessGroups) == "" {
			return false
		}
		group = account.AccessGroups
	} else {
		group = account.Group
		if strings.TrimSpace(group) == "" && channel != nil {
			group = channel.Group
		}
	}
	return routingValueAllowsGroup(group, usingGroup)
}

// RoutingPoolAccountSupportsModel 判断全局账号池账号是否支持指定模型。
// 账号级模型优先于分组级模型；两者都为空时沿用历史“不限制模型”的语义。
func RoutingPoolAccountSupportsModel(account *PoolAccount, group *AccountPoolGroup, modelName string) bool {
	if account == nil {
		return false
	}
	models := account.Models
	if strings.TrimSpace(models) == "" && group != nil {
		models = group.Models
	}
	modelList := SplitCommaValues(models)
	if len(modelList) == 0 {
		return true
	}
	return MatchesModelList(modelList, modelName)
}

// RoutingPoolAccountSupportsGroup 判断全局账号池账号是否允许指定 NexusTok 用户组。
// 账号级分组优先于分组级分组；空列表或通配符 "*" 表示允许所有分组。
func RoutingPoolAccountSupportsGroup(account *PoolAccount, group *AccountPoolGroup, usingGroup string) bool {
	if account == nil {
		return false
	}
	if strings.TrimSpace(usingGroup) == "" {
		return true
	}
	groupValue := account.Group
	if strings.TrimSpace(groupValue) == "" && group != nil {
		groupValue = group.Group
	}
	return routingValueAllowsGroup(groupValue, usingGroup)
}

func routingValueAllowsGroup(groupValue string, usingGroup string) bool {
	groups := SplitCommaValues(groupValue)
	if len(groups) == 0 {
		return true
	}
	for _, candidate := range groups {
		if candidate == "*" || candidate == usingGroup {
			return true
		}
	}
	return false
}
