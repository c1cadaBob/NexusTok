package controller

import (
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/i18n"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// requireAccountPoolGroupSensitiveWriteIfNeeded 对账号池分组更新做字段级敏感写校验。
//
// 账号池分组更新接口是一个混合 payload：名称、模型、分组和限流阈值属于普通写；
// platform、auth_type、model_mapping 和开放 settings JSON 会改变凭证解释、上游模型映射
// 或未来扩展行为，必须额外要求 account_pool.sensitive_write。未知字段默认敏感，确保后续
// 新增字段在完成分类前不会被普通管理员静默写入。
func requireAccountPoolGroupSensitiveWriteIfNeeded(c *gin.Context, req accountPoolGroupUpsertRequest, origin *model.AccountPoolGroup, requestData map[string]any) bool {
	if !accountPoolGroupHasSensitiveChanges(req, origin, requestData) {
		return true
	}
	if accountPoolCanSensitiveWrite(c) {
		return true
	}
	common.ApiErrorI18n(c, i18n.MsgAuthInsufficientPrivilege)
	return false
}

// completeAccountPoolGroupUpdateRequest 用原始记录补齐局部更新里缺失的文本字段。
//
// 历史 update map 会无条件写入 models、group 和 settings。前端通常提交完整表单不受影响，
// 但外部客户端如果只提交 name，旧逻辑会把这些字段清空。这里用 requestData 判断字段是否
// 出现，缺失时继承原值；字段显式传空字符串仍表示用户希望清空。
func completeAccountPoolGroupUpdateRequest(req *accountPoolGroupUpsertRequest, origin *model.AccountPoolGroup, requestData map[string]any) {
	if req == nil || origin == nil {
		return
	}
	if _, ok := requestData["models"]; !ok {
		req.Models = origin.Models
	}
	if _, ok := requestData["group"]; !ok {
		req.Group = origin.Group
	}
	if _, ok := requestData["settings"]; !ok {
		req.Settings = origin.Settings
	}
}

// accountPoolGroupHasSensitiveChanges 判断分组更新是否触碰高风险字段。
//
// requestData 来自原始 JSON，用于区分“字段缺失”和“字段显式设置为空值”。敏感字段只有在
// 实际值发生变化时才要求敏感写权限；未知字段无论值如何都按敏感处理，避免新增字段失守。
func accountPoolGroupHasSensitiveChanges(req accountPoolGroupUpsertRequest, origin *model.AccountPoolGroup, requestData map[string]any) bool {
	if origin == nil {
		return true
	}
	if _, ok := requestData["platform"]; ok {
		platform := strings.ToLower(strings.TrimSpace(req.Platform))
		if platform != "" && platform != strings.ToLower(strings.TrimSpace(origin.Platform)) {
			return true
		}
	}
	if _, ok := requestData["auth_type"]; ok {
		authType := strings.ToLower(strings.TrimSpace(req.AuthType))
		if authType != "" && authType != strings.ToLower(strings.TrimSpace(origin.AuthType)) {
			return true
		}
	}
	if _, ok := requestData["model_mapping"]; ok && req.ModelMapping != nil {
		if accountPoolOptionalStringValue(req.ModelMapping) != accountPoolOptionalStringValue(origin.ModelMapping) {
			return true
		}
	}
	if _, ok := requestData["settings"]; ok && !accountPoolGroupSettingsEqualForAuthz(req, origin) {
		return true
	}

	for field := range requestData {
		if _, ok := accountPoolGroupSensitiveFields[field]; ok {
			continue
		}
		if _, ok := accountPoolGroupNonSensitiveFields[field]; ok {
			continue
		}
		if _, ok := accountPoolGroupReadOnlyFields[field]; ok {
			continue
		}
		return true
	}
	return false
}

// accountPoolGroupSettingsEqualForAuthz 比较 settings 是否真的发生敏感变化。
//
// max_concurrency 已迁出 settings 成为独立列。普通管理员显式修改 max_concurrency 时，
// accountPoolGroupRequestSettings 会顺手移除历史 settings.max_concurrency；这属于限流字段
// 兼容清理，不应被误判为 settings 敏感变更。
func accountPoolGroupSettingsEqualForAuthz(req accountPoolGroupUpsertRequest, origin *model.AccountPoolGroup) bool {
	if origin == nil {
		return false
	}
	current := strings.TrimSpace(origin.Settings)
	if req.MaxConcurrency != nil {
		current = strings.TrimSpace(removeAccountPoolGroupSetting(current, "max_concurrency"))
	}
	next := strings.TrimSpace(accountPoolGroupRequestSettings(req))
	return next == current
}

func accountPoolCanSensitiveWrite(c *gin.Context) bool {
	return authz.Can(c.GetInt("id"), c.GetInt("role"), authz.AccountPoolSensitiveWrite)
}

func accountPoolOptionalStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

// accountPoolGroupSensitiveFields 记录更新既有分组时需要敏感写权限的字段。
//
// 这些字段会改变账号池如何解释凭证、映射模型或应用开放 JSON 扩展。settings 是未来扩展
// 入口，因此必须按 fail-closed 处理，不能让普通写权限绕过字段级评审。
var accountPoolGroupSensitiveFields = map[string]struct{}{
	"platform":      {},
	"auth_type":     {},
	"model_mapping": {},
	"settings":      {},
}

// accountPoolGroupNonSensitiveFields 是具备 account_pool.write 的管理员可维护的分组字段。
var accountPoolGroupNonSensitiveFields = map[string]struct{}{
	"name":                              {},
	"status":                            {},
	"strategy":                          {},
	"models":                            {},
	"group":                             {},
	"max_concurrency":                   {},
	"rate_limit_rpm":                    {},
	"daily_request_limit":               {},
	"daily_quota_limit":                 {},
	"daily_limit_action":                {},
	"auto_check_enabled":                {},
	"auto_check_interval_minutes":       {},
	"auto_check_limit":                  {},
	"preflight_check_mode":              {},
	"preflight_check_freshness_minutes": {},
	"preflight_check_limit":             {},
	"no_available_action":               {},
	"no_available_wait_seconds":         {},
	"task_max_concurrency":              {},
	"task_rate_limit_rpm":               {},
	"task_limit_action":                 {},
	"task_limit_wait_seconds":           {},
}

// accountPoolGroupReadOnlyFields 是服务端响应或运行时统计字段。
//
// 更新接口不会写入这些字段；如果客户端误带，分类为只读可以避免因为响应对象回提交而触发
// 敏感写，但真正的更新 map 仍会忽略它们。
var accountPoolGroupReadOnlyFields = map[string]struct{}{
	"id":                      {},
	"source":                  {},
	"external_group_key":      {},
	"daily_request_count":     {},
	"used_quota":              {},
	"daily_used_quota":        {},
	"daily_reset_time":        {},
	"auto_check_last_time":    {},
	"auto_check_next_time":    {},
	"auto_check_last_task_id": {},
	"created_time":            {},
	"updated_time":            {},
	"stats":                   {},
	"daily_limit_state":       {},
}
