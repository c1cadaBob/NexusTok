// Package controller - channel_authz.go
// 该文件承载渠道编辑字段的敏感度分类。
//
// NexusTok 暂未引入 new-api-main 的完整 Authz/Casbin 权限系统，因此本文件先把
// “渠道敏感写”原生映射为 Root 权限。等后续 AdminPermission/ChannelSensitiveWrite
// 落地后，只需要替换 channelCanSensitiveWrite 的判定，不需要重新梳理字段分类。
package controller

import (
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// channelHasSensitiveChanges 判断本次渠道更新是否触碰高风险字段。
//
// requestData 必须来自原始 JSON 请求体，用于区分“字段缺失”和“字段显式设置为零值”。
// 对高风险字段逐项比较旧值和新值；对未知字段采用 fail-closed 策略，避免未来新增
// 字段在没有明确分类前被普通管理员静默写入。
func channelHasSensitiveChanges(channel *PatchChannel, origin *model.Channel, requestData map[string]any) bool {
	if channel == nil || origin == nil {
		return true
	}
	if _, ok := requestData["type"]; ok && channel.Type != origin.Type {
		return true
	}
	if _, ok := requestData["key"]; ok && channel.Key != origin.Key {
		return true
	}
	if _, ok := requestData["base_url"]; ok && !equalOptionalStringPtr(channel.BaseURL, origin.BaseURL) {
		return true
	}
	if _, ok := requestData["openai_organization"]; ok && !equalOptionalStringPtr(channel.OpenAIOrganization, origin.OpenAIOrganization) {
		return true
	}
	if _, ok := requestData["header_override"]; ok && !equalOptionalStringPtr(channel.HeaderOverride, origin.HeaderOverride) {
		return true
	}
	if _, ok := requestData["param_override"]; ok && !equalOptionalStringPtr(channel.ParamOverride, origin.ParamOverride) {
		return true
	}
	if _, ok := requestData["setting"]; ok && !equalOptionalStringPtr(channel.Setting, origin.Setting) {
		return true
	}
	if _, ok := requestData["other"]; ok && channel.Other != origin.Other {
		return true
	}
	if _, ok := requestData["settings"]; ok && channel.OtherSettings != origin.OtherSettings {
		return true
	}
	if _, ok := requestData["channel_info"]; ok && channelInfoHasSensitiveChanges(channel.ChannelInfo, origin.ChannelInfo) {
		return true
	}
	if _, ok := requestData["key_mode"]; ok && channel.KeyMode != nil {
		return true
	}

	for field := range requestData {
		if _, ok := channelSensitiveFields[field]; ok {
			continue
		}
		if _, ok := channelNonSensitiveFields[field]; ok {
			continue
		}
		if _, ok := channelOperationalFields[field]; ok {
			continue
		}
		if _, ok := channelReadOnlyFields[field]; ok {
			continue
		}
		return true
	}
	return false
}

// channelInfoHasSensitiveChanges 只比较客户端允许编辑的 ChannelInfo 子集。
//
// ChannelInfo 同时保存凭证模式和多 Key 运行时状态。前端完整编辑表单通常只回传
// credential_mode、account_pool_*、is_multi_key、multi_key_mode 等配置字段，不会携带
// MultiKeyStatusList、MultiKeyDisabledTime、MultiKeyPollingIndex 这类运行时字段；因此这里
// 不能对整个结构体做 DeepEqual，否则会把“运行时字段缺失”误判成敏感变更。
func channelInfoHasSensitiveChanges(next, origin model.ChannelInfo) bool {
	next.CredentialMode = normalizeChannelCredentialMode(next)
	origin.CredentialMode = normalizeChannelCredentialMode(origin)
	next.MultiKeyMode = normalizeChannelMultiKeyMode(next.MultiKeyMode)
	origin.MultiKeyMode = normalizeChannelMultiKeyMode(origin.MultiKeyMode)
	return next.CredentialMode != origin.CredentialMode ||
		next.AccountPoolEnabled != origin.AccountPoolEnabled ||
		next.AccountPoolMode != origin.AccountPoolMode ||
		next.AccountPoolFallback != origin.AccountPoolFallback ||
		next.AccountPoolGroupId != origin.AccountPoolGroupId ||
		next.IsMultiKey != origin.IsMultiKey ||
		next.MultiKeyMode != origin.MultiKeyMode
}

// normalizeChannelCredentialMode 与前端表单保持相同的历史数据推断顺序。
// 旧渠道可能没有显式 credential_mode，前端会根据 account_pool_enabled/is_multi_key
// 补成 account_pool/multi_key/single_key；后端比较也要做同样归一化，避免误判。
func normalizeChannelCredentialMode(info model.ChannelInfo) string {
	if info.CredentialMode != "" {
		return info.CredentialMode
	}
	if info.AccountPoolEnabled {
		return constant.ChannelCredentialModeAccountPool
	}
	if info.IsMultiKey {
		return constant.ChannelCredentialModeMultiKey
	}
	return constant.ChannelCredentialModeSingleKey
}

// normalizeChannelMultiKeyMode 将空值归一为前端默认的 random。
func normalizeChannelMultiKeyMode(mode constant.MultiKeyMode) constant.MultiKeyMode {
	if mode == "" {
		return constant.MultiKeyModeRandom
	}
	return mode
}

// channelCanSensitiveWrite 是完整 Authz 引入前的过渡桥接。
// 当前 authz.Can 只基于 Root/Admin 系统角色基线，效果仍等价 Root 才能通过；
// 后续接入 Casbin 或用户级 override 时，字段级敏感写会自动复用同一套判定。
func channelCanSensitiveWrite(c *gin.Context) bool {
	return authz.Can(c.GetInt("id"), c.GetInt("role"), authz.ChannelSensitiveWrite)
}

// channelSensitiveFields 记录需要敏感写权限的渠道字段。
// 这些字段会影响上游凭证、目标地址、请求改写或账号池绑定，误改可能造成凭证泄露、
// SSRF、错误计费或把流量导向非预期上游。
var channelSensitiveFields = map[string]struct{}{
	"type":                {},
	"key":                 {},
	"base_url":            {},
	"openai_organization": {},
	"header_override":     {},
	"param_override":      {},
	"setting":             {},
	"other":               {},
	"settings":            {},
	"channel_info":        {},
	"key_mode":            {},
}

// channelOperationalFields 由操作类入口管理。
// status 已迁移到 /api/channel/:id/status 和 /api/channel/status/batch，
// 通用编辑接口会提前拒绝该字段；这里保留分类是为了让字段分类测试继续约束模型字段全集。
var channelOperationalFields = map[string]struct{}{
	"status": {},
}

// channelReadOnlyFields 是服务端运行时或统计字段。
// 如果客户端误带这些字段，UpdateChannel 会清零并依赖 GORM 的零值跳过语义避免写入；
// 后续若改为显式 Select 更新，也必须继续忽略这些字段。
var channelReadOnlyFields = map[string]struct{}{
	"created_time":               {},
	"test_time":                  {},
	"response_time":              {},
	"balance":                    {},
	"balance_updated_time":       {},
	"used_quota":                 {},
	"channel_account_stats":      {},
	"minimum_ratio":              {},
	"upstream_balance_usd":       {},
	"upstream_used_usd":          {},
	"upstream_used_quota":        {},
	"upstream_conversion_factor": {},
	"upstream_partial":           {},
}

// channelNonSensitiveFields 是普通管理员可调整的路由和展示字段。
// 新增 model.Channel 字段时必须在敏感、非敏感、操作或只读集合中显式分类；
// TestChannelFieldsAreClassified 会守住这个约束，让新增字段默认 fail-closed。
var channelNonSensitiveFields = map[string]struct{}{
	"id":                  {},
	"test_model":          {},
	"name":                {},
	"weight":              {},
	"models":              {},
	"group":               {},
	"model_mapping":       {},
	"status_code_mapping": {},
	"priority":            {},
	"auto_ban":            {},
	"other_info":          {},
	"tag":                 {},
	"remark":              {},
	"multi_key_mode":      {},
}

// clearChannelReadOnlyFields 清理客户端不应写入的运行时统计字段。
func clearChannelReadOnlyFields(channel *PatchChannel, requestData map[string]any) {
	if _, ok := requestData["created_time"]; ok {
		channel.CreatedTime = 0
	}
	if _, ok := requestData["test_time"]; ok {
		channel.TestTime = 0
	}
	if _, ok := requestData["response_time"]; ok {
		channel.ResponseTime = 0
	}
	if _, ok := requestData["balance"]; ok {
		channel.Balance = 0
	}
	if _, ok := requestData["balance_updated_time"]; ok {
		channel.BalanceUpdatedTime = 0
	}
	if _, ok := requestData["used_quota"]; ok {
		channel.UsedQuota = 0
	}
	channel.ChannelAccountStats = nil
	channel.MinimumRatio = nil
	channel.UpstreamBalanceUSD = nil
	channel.UpstreamUsedUSD = nil
	channel.UpstreamUsedQuota = nil
	channel.UpstreamConversionFactor = nil
	channel.UpstreamPartial = false
}

// equalOptionalStringPtr 比较可选字符串指针。
// 渠道表单常把空值规范化为 ""，数据库历史数据可能保存为 nil；对 BaseURL、
// Organization 和覆盖配置这类可选字段，nil 与空字符串在运行时都表示“未设置”，
// 因此敏感变更判断里按等价处理。
func equalOptionalStringPtr(a, b *string) bool {
	return optionalStringValue(a) == optionalStringValue(b)
}

func optionalStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
