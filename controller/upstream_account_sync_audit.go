package controller

import (
	"regexp"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/upstreamaccount"

	"github.com/gin-gonic/gin"
)

const (
	upstreamAccountSyncAuditAction                      = "channel.upstream_account_sync_refresh"
	systemTaskUpstreamAccountSyncAuditAction            = "system_task.upstream_account_sync"
	systemTaskUpstreamAccountScheduleRefreshAuditAction = "system_task.upstream_account_schedule_refresh"
	upstreamAccountSyncAuditErrorMaxRunes               = 512
	upstreamAccountSyncAuditFailureLimit                = 5
)

var upstreamAccountSyncAuditSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(access[_-]?token|refresh[_-]?token|password|session|cookie|api[_-]?key|key)\s*[:=]\s*["']?[^"',\s}]+`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{12,}`),
}

// recordManualUpstreamAccountSyncAudit 记录管理员手动刷新上游同步渠道的审计摘要。
//
// 审计只保存渠道、平台、结果计数和脱敏错误，不记录密码、登录态或完整 API Key。
// 写入后立即标记当前 Gin 上下文，避免中间件再追加 generic 路由审计。
func recordManualUpstreamAccountSyncAudit(c *gin.Context, req upstreamaccount.RefreshRequest, result *upstreamaccount.RefreshResult, runErr error) {
	if c == nil {
		return
	}
	params := upstreamAccountSyncAuditParams(req.ChannelID, req.Platform, "manual", result, runErr)
	model.RecordOperationAuditLog(model.OperationAuditLogParams{
		UserId:    c.GetInt("id"),
		Content:   auditContentEN(upstreamAccountSyncAuditAction, params),
		Ip:        c.ClientIP(),
		Action:    upstreamAccountSyncAuditAction,
		Params:    params,
		AdminInfo: auditOperatorInfo(c),
		AuditInfo: upstreamAccountSyncAuditInfoFromParams(params),
	})
	markAuditLogged(c)
}

// recordSystemUpstreamAccountSyncAudit 记录 upstream_account_sync 系统任务的汇总审计。
//
// 系统任务没有管理员操作者，日志用户名固定为 system；任务摘要同样只包含汇总计数
// 和截断后的失败原因，避免把任何上游凭据写入管理日志。
func recordSystemUpstreamAccountSyncAudit(task *model.SystemTask, runnerID string, summary *upstreamaccount.UpstreamAccountSyncSummary, runErr error) {
	params := systemUpstreamAccountSyncAuditParams(task, runnerID, summary, runErr)
	model.RecordOperationAuditLog(model.OperationAuditLogParams{
		UserId:   0,
		Username: "system",
		Content:  auditContentEN(systemTaskUpstreamAccountSyncAuditAction, params),
		Ip:       common.GetIp(),
		Action:   systemTaskUpstreamAccountSyncAuditAction,
		Params:   params,
		AdminInfo: map[string]interface{}{
			"admin_username": "system",
			"auth_method":    "system_task",
			"node_name":      common.NodeName,
		},
		AuditInfo: upstreamAccountSyncAuditInfoFromParams(params),
	})
}

// recordSystemUpstreamAccountScheduleRefreshAudit 记录同步密钥调度建议统一刷新的审计摘要。
//
// 维护任务只写入扫描数量、更新数量和受影响渠道 ID，不记录账号 key、digest 或上游登录凭据。
func recordSystemUpstreamAccountScheduleRefreshAudit(task *model.SystemTask, runnerID string, summary *upstreamaccount.ScheduleRefreshSummary, runErr error) {
	params := systemUpstreamAccountScheduleRefreshAuditParams(task, runnerID, summary, runErr)
	model.RecordOperationAuditLog(model.OperationAuditLogParams{
		UserId:   0,
		Username: "system",
		Content:  auditContentEN(systemTaskUpstreamAccountScheduleRefreshAuditAction, params),
		Ip:       common.GetIp(),
		Action:   systemTaskUpstreamAccountScheduleRefreshAuditAction,
		Params:   params,
		AdminInfo: map[string]interface{}{
			"admin_username": "system",
			"auth_method":    "system_task",
			"node_name":      common.NodeName,
		},
		AuditInfo: upstreamAccountScheduleRefreshAuditInfoFromParams(params),
	})
}

// upstreamAccountSyncAuditParams 构造手动同步审计的非敏感结构化参数。
func upstreamAccountSyncAuditParams(channelID int, requestedPlatform string, source string, result *upstreamaccount.RefreshResult, runErr error) map[string]interface{} {
	name, platform := upstreamAccountSyncAuditChannel(channelID)
	if strings.TrimSpace(requestedPlatform) != "" {
		platform = upstreamaccount.NormalizePlatform(requestedPlatform)
	}
	params := map[string]interface{}{
		"id":       channelID,
		"name":     name,
		"platform": platform,
		"source":   source,
		"success":  runErr == nil,
	}
	if result != nil {
		params["created"] = result.Created
		params["updated"] = result.Updated
		params["disabled"] = result.Disabled
	}
	if runErr != nil {
		params["error"] = sanitizeUpstreamAccountSyncAuditText(runErr.Error(), upstreamAccountSyncAuditErrorMaxRunes)
	}
	return params
}

// systemUpstreamAccountSyncAuditParams 构造系统自动同步审计的非敏感结构化参数。
func systemUpstreamAccountSyncAuditParams(task *model.SystemTask, runnerID string, summary *upstreamaccount.UpstreamAccountSyncSummary, runErr error) map[string]interface{} {
	params := map[string]interface{}{
		"source":    "system_task",
		"task_id":   taskIDForAudit(task),
		"runner_id": runnerID,
		"success":   runErr == nil,
	}
	if summary != nil {
		params["scanned"] = summary.ScannedChannels
		params["eligible"] = summary.EligibleChannels
		params["skipped"] = summary.SkippedChannels
		params["succeeded"] = summary.SucceededChannels
		params["failed"] = summary.FailedChannels
		params["created"] = summary.CreatedAccounts
		params["updated"] = summary.UpdatedAccounts
		params["disabled"] = summary.DisabledAccounts
		if summary.Skipped {
			params["task_skipped"] = true
			params["skip_reason"] = sanitizeUpstreamAccountSyncAuditText(summary.SkipReason, upstreamAccountSyncAuditErrorMaxRunes)
		}
		if len(summary.Failures) > 0 {
			params["failures"] = upstreamAccountSyncFailureAuditItems(summary.Failures)
		}
	}
	if runErr != nil {
		params["error"] = sanitizeUpstreamAccountSyncAuditText(runErr.Error(), upstreamAccountSyncAuditErrorMaxRunes)
	}
	return params
}

func systemUpstreamAccountScheduleRefreshAuditParams(task *model.SystemTask, runnerID string, summary *upstreamaccount.ScheduleRefreshSummary, runErr error) map[string]interface{} {
	params := map[string]interface{}{
		"source":    "system_task",
		"task_id":   taskIDForAudit(task),
		"runner_id": runnerID,
		"success":   runErr == nil,
	}
	if summary != nil {
		params["scanned_accounts"] = summary.ScannedAccounts
		params["updated_accounts"] = summary.UpdatedAccounts
		params["affected_channels"] = summary.AffectedChannels
		params["skipped_accounts"] = summary.SkippedAccounts
		params["channel_ids"] = summary.ChannelIDs
	}
	if runErr != nil {
		params["error"] = sanitizeUpstreamAccountSyncAuditText(runErr.Error(), upstreamAccountSyncAuditErrorMaxRunes)
	}
	return params
}

// upstreamAccountSyncAuditInfoFromParams 复用已脱敏 params 生成 audit_info。
//
// 前端详情弹窗会同时展示 op.params 和 audit_info；这里仅复制白名单字段，保持两个
// 结构都能独立表达同步结果，同时避免未来 params 扩展时意外带入敏感信息。
func upstreamAccountSyncAuditInfoFromParams(params map[string]interface{}) map[string]interface{} {
	info := map[string]interface{}{}
	for _, key := range []string{
		"id",
		"name",
		"platform",
		"source",
		"task_id",
		"runner_id",
		"success",
		"scanned",
		"eligible",
		"skipped",
		"succeeded",
		"failed",
		"created",
		"updated",
		"disabled",
		"task_skipped",
		"skip_reason",
		"failures",
		"error",
	} {
		if value, ok := params[key]; ok {
			info[key] = value
		}
	}
	return info
}

func upstreamAccountScheduleRefreshAuditInfoFromParams(params map[string]interface{}) map[string]interface{} {
	info := map[string]interface{}{}
	for _, key := range []string{
		"source",
		"task_id",
		"runner_id",
		"success",
		"scanned_accounts",
		"updated_accounts",
		"affected_channels",
		"skipped_accounts",
		"channel_ids",
		"error",
	} {
		if value, ok := params[key]; ok {
			info[key] = value
		}
	}
	return info
}

// upstreamAccountSyncAuditChannel 读取渠道名称和已配置的同步平台用于审计展示。
func upstreamAccountSyncAuditChannel(channelID int) (string, string) {
	if channelID <= 0 {
		return "", ""
	}
	var channel model.Channel
	if err := model.DB.Where("id = ?", channelID).First(&channel).Error; err != nil {
		return "", ""
	}
	return channel.Name, channel.UpstreamAccountSyncPlatform()
}

// upstreamAccountSyncFailureAuditItems 截断系统任务失败列表并脱敏错误文本。
func upstreamAccountSyncFailureAuditItems(failures []upstreamaccount.UpstreamAccountSyncFailure) []map[string]interface{} {
	limit := len(failures)
	if limit > upstreamAccountSyncAuditFailureLimit {
		limit = upstreamAccountSyncAuditFailureLimit
	}
	items := make([]map[string]interface{}, 0, limit)
	for index := 0; index < limit; index++ {
		failure := failures[index]
		items = append(items, map[string]interface{}{
			"channel_id":   failure.ChannelID,
			"channel_name": failure.ChannelName,
			"error":        sanitizeUpstreamAccountSyncAuditText(failure.Error, upstreamAccountSyncAuditErrorMaxRunes),
		})
	}
	return items
}

// sanitizeUpstreamAccountSyncAuditText 清理同步审计中的错误文本。
//
// 上游站点错误可能会回显 password、access_token、refresh_token、session cookie
// 或完整 API Key；这些内容只能用于当前请求排错，不能进入持久化审计日志。
func sanitizeUpstreamAccountSyncAuditText(value string, maxRunes int) string {
	text := strings.TrimSpace(value)
	for _, pattern := range upstreamAccountSyncAuditSecretPatterns {
		text = pattern.ReplaceAllStringFunc(text, func(match string) string {
			if strings.HasPrefix(strings.ToLower(match), "sk-") {
				return "[redacted-key]"
			}
			key, _, ok := strings.Cut(match, "=")
			if !ok {
				key, _, ok = strings.Cut(match, ":")
			}
			if !ok {
				return "[redacted-secret]"
			}
			return strings.TrimSpace(key) + "=[redacted]"
		})
	}
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

// taskIDForAudit 安全读取系统任务 ID，兼容测试或异常路径里的 nil task。
func taskIDForAudit(task *model.SystemTask) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.TaskID)
}
