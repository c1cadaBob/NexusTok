package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
)

const (
	accountPoolHealthDefaultAbnormalLimit = 10
	accountPoolHealthMaxAbnormalLimit     = 50
	accountPoolHealthDefaultAuditLimit    = 10
	accountPoolHealthMaxAuditLimit        = 50
)

// AccountPoolHealthSummary 是账号池健康看板的聚合响应。
// 它只读取原生账号池组、账号运行态、使用日志和状态审计日志，不写入任何热路径状态。
type AccountPoolHealthSummary struct {
	GeneratedAt            int64                         `json:"generated_at"`
	WindowStart            int64                         `json:"window_start"`
	WindowEnd              int64                         `json:"window_end"`
	Totals                 AccountPoolHealthTotals       `json:"totals"`
	Groups                 []*AccountPoolGroupHealth     `json:"groups"`
	RecentAbnormalAccounts []*AccountPoolAbnormalAccount `json:"recent_abnormal_accounts"`
	RecentStateLogs        []*PoolAccountStateLog        `json:"recent_state_logs"`
}

// AccountPoolHealthTotals 描述账号池整体健康度。
// SuccessRate 和 AvailabilityRate 使用 0..1 的小数，前端可统一格式化为百分比。
type AccountPoolHealthTotals struct {
	GroupCount          int     `json:"group_count"`
	LimitedGroupCount   int     `json:"limited_group_count"`
	TotalAccounts       int64   `json:"total_accounts"`
	AvailableAccounts   int64   `json:"available_accounts"`
	DisabledAccounts    int64   `json:"disabled_accounts"`
	CooldownAccounts    int64   `json:"cooldown_accounts"`
	UnavailableAccounts int64   `json:"unavailable_accounts"`
	TodayRequests       int64   `json:"today_requests"`
	TodaySuccesses      int64   `json:"today_successes"`
	TodayFailures       int64   `json:"today_failures"`
	SuccessRate         float64 `json:"success_rate"`
	AvailabilityRate    float64 `json:"availability_rate"`
}

// AccountPoolGroupHealth 描述单个账号池分组的健康快照。
// Stats 复用分组列表已有统计口径，便于页面在列表和看板间保持一致。
type AccountPoolGroupHealth struct {
	Id                             int                        `json:"id"`
	Name                           string                     `json:"name"`
	Platform                       string                     `json:"platform"`
	AuthType                       string                     `json:"auth_type"`
	Status                         int                        `json:"status"`
	Strategy                       string                     `json:"strategy"`
	Stats                          map[string]int64           `json:"stats"`
	DailyLimitState                AccountPoolDailyLimitState `json:"daily_limit_state"`
	AutoCheckEnabled               bool                       `json:"auto_check_enabled"`
	AutoCheckIntervalMinutes       int                        `json:"auto_check_interval_minutes"`
	AutoCheckNextTime              int64                      `json:"auto_check_next_time"`
	AutoCheckLastTaskId            int                        `json:"auto_check_last_task_id"`
	PreflightCheckMode             string                     `json:"preflight_check_mode"`
	PreflightCheckFreshnessMinutes int                        `json:"preflight_check_freshness_minutes"`
	TodayRequests                  int64                      `json:"today_requests"`
	TodaySuccesses                 int64                      `json:"today_successes"`
	TodayFailures                  int64                      `json:"today_failures"`
	SuccessRate                    float64                    `json:"success_rate"`
	AvailabilityRate               float64                    `json:"availability_rate"`
}

// AccountPoolAbnormalAccount 描述最近需要管理员关注的异常账号。
// 响应只包含状态、原因、调度和统计字段，不返回凭证明文、OAuth metadata 或其它敏感配置。
type AccountPoolAbnormalAccount struct {
	Id                 int     `json:"id"`
	PoolGroupId        int     `json:"pool_group_id"`
	PoolGroupName      string  `json:"pool_group_name"`
	Name               string  `json:"name"`
	Platform           string  `json:"platform"`
	AuthType           string  `json:"auth_type"`
	CredentialProvider string  `json:"credential_provider"`
	Status             int     `json:"status"`
	Schedulable        bool    `json:"schedulable"`
	Unavailable        bool    `json:"unavailable"`
	CoolingUntil       int64   `json:"cooling_until"`
	Reason             string  `json:"reason"`
	StatusMessage      string  `json:"status_message"`
	DisabledReason     string  `json:"disabled_reason"`
	LastError          string  `json:"last_error"`
	LastCheckedTime    int64   `json:"last_checked_time"`
	LastUsedTime       int64   `json:"last_used_time"`
	NextRetryTime      int64   `json:"next_retry_time"`
	SuccessCount       int64   `json:"success_count"`
	FailedCount        int64   `json:"failed_count"`
	FailureRate        float64 `json:"failure_rate"`
}

// AccountPoolHealthOptions 描述健康看板查询条件。
// PoolGroupID 为 0 时返回全部原生账号池分组；非 0 时只返回指定分组的健康快照。
type AccountPoolHealthOptions struct {
	PoolGroupID   int
	AbnormalLimit int
	AuditLimit    int
}

type accountPoolHealthUsageAggregate struct {
	PoolGroupId int   `gorm:"column:pool_group_id"`
	Total       int64 `gorm:"column:total"`
	Failed      int64 `gorm:"column:failed"`
}

type accountPoolHealthUsage struct {
	Total   int64
	Failed  int64
	Success int64
}

// GetAccountPoolHealthSummary 返回原生账号池整体或单分组健康概览。
// 今日请求窗口沿用账号池每日额度窗口，保证看板与调度层“今日”口径一致。
func GetAccountPoolHealthSummary(opts AccountPoolHealthOptions) (*AccountPoolHealthSummary, error) {
	now := time.Now()
	windowStart := AccountPoolDailyWindowStart(now)
	windowEnd := common.GetTimestamp()
	groups, err := loadAccountPoolHealthGroups(opts.PoolGroupID)
	if err != nil {
		return nil, err
	}
	groupIDs := accountPoolHealthGroupIDs(groups)
	usageByGroup, err := loadAccountPoolHealthUsageByGroup(groupIDs, windowStart, windowEnd)
	if err != nil {
		return nil, err
	}
	groupNameByID := accountPoolHealthGroupNameMap(groups)
	groupViews := make([]*AccountPoolGroupHealth, 0, len(groups))
	totals := AccountPoolHealthTotals{GroupCount: len(groups)}
	for _, group := range groups {
		if group == nil {
			continue
		}
		usage := usageByGroup[group.Id]
		stats := group.Stats
		if stats == nil {
			stats = newPoolAccountStats()
		}
		dailyLimitState := AccountPoolGroupDailyLimitState(group, now)
		if dailyLimitState.Limited {
			totals.LimitedGroupCount++
		}
		totals.TotalAccounts += stats["total"]
		totals.AvailableAccounts += stats["enabled"]
		totals.DisabledAccounts += stats["disabled"]
		totals.CooldownAccounts += stats["cooldown"]
		totals.UnavailableAccounts += stats["unavailable"]
		totals.TodayRequests += usage.Total
		totals.TodaySuccesses += usage.Success
		totals.TodayFailures += usage.Failed
		groupViews = append(groupViews, &AccountPoolGroupHealth{
			Id:                             group.Id,
			Name:                           group.Name,
			Platform:                       group.Platform,
			AuthType:                       group.AuthType,
			Status:                         group.Status,
			Strategy:                       group.Strategy,
			Stats:                          stats,
			DailyLimitState:                dailyLimitState,
			AutoCheckEnabled:               group.AutoCheckEnabled,
			AutoCheckIntervalMinutes:       group.GetAutoCheckIntervalMinutes(),
			AutoCheckNextTime:              group.AutoCheckNextTime,
			AutoCheckLastTaskId:            group.AutoCheckLastTaskId,
			PreflightCheckMode:             group.GetPreflightCheckMode(),
			PreflightCheckFreshnessMinutes: group.GetPreflightCheckFreshnessMinutes(),
			TodayRequests:                  usage.Total,
			TodaySuccesses:                 usage.Success,
			TodayFailures:                  usage.Failed,
			SuccessRate:                    ratio64(usage.Success, usage.Total),
			AvailabilityRate:               ratio64(stats["enabled"], stats["total"]),
		})
	}
	totals.SuccessRate = ratio64(totals.TodaySuccesses, totals.TodayRequests)
	totals.AvailabilityRate = ratio64(totals.AvailableAccounts, totals.TotalAccounts)
	abnormalAccounts, err := loadAccountPoolAbnormalAccounts(groupIDs, groupNameByID, opts.AbnormalLimit)
	if err != nil {
		return nil, err
	}
	stateLogs := []*PoolAccountStateLog{}
	if opts.PoolGroupID <= 0 || len(groupIDs) > 0 {
		var logsErr error
		stateLogs, _, logsErr = GetPoolAccountStateLogs(PoolAccountStateLogFilter{
			PoolGroupId: opts.PoolGroupID,
			StartIdx:    0,
			Limit:       normalizeAccountPoolHealthAuditLimit(opts.AuditLimit),
		})
		if logsErr != nil {
			return nil, logsErr
		}
	}
	return &AccountPoolHealthSummary{
		GeneratedAt:            windowEnd,
		WindowStart:            windowStart,
		WindowEnd:              windowEnd,
		Totals:                 totals,
		Groups:                 groupViews,
		RecentAbnormalAccounts: abnormalAccounts,
		RecentStateLogs:        stateLogs,
	}, nil
}

func loadAccountPoolHealthGroups(groupID int) ([]*AccountPoolGroup, error) {
	query := DB.Where("(source = ? OR source = '')", AccountPoolGroupSourceNative)
	if groupID > 0 {
		query = query.Where("id = ?", groupID)
	}
	var groups []*AccountPoolGroup
	if err := query.Order("id DESC").Find(&groups).Error; err != nil {
		return nil, err
	}
	AttachAccountPoolGroupStats(groups)
	return groups, nil
}

func accountPoolHealthGroupIDs(groups []*AccountPoolGroup) []int {
	ids := make([]int, 0, len(groups))
	for _, group := range groups {
		if group == nil || group.Id <= 0 {
			continue
		}
		ids = append(ids, group.Id)
	}
	return ids
}

func accountPoolHealthGroupNameMap(groups []*AccountPoolGroup) map[int]string {
	result := make(map[int]string, len(groups))
	for _, group := range groups {
		if group == nil || group.Id <= 0 {
			continue
		}
		result[group.Id] = group.Name
	}
	return result
}

func loadAccountPoolHealthUsageByGroup(groupIDs []int, start int64, end int64) (map[int]accountPoolHealthUsage, error) {
	result := map[int]accountPoolHealthUsage{}
	if len(groupIDs) == 0 {
		return result, nil
	}
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if logDB == nil {
		return result, fmt.Errorf("log db is not initialized")
	}
	rows := []accountPoolHealthUsageAggregate{}
	err := logDB.Model(&PoolAccountUsageLog{}).
		Select("pool_group_id, COUNT(*) AS total, SUM(CASE WHEN success = ? THEN 1 ELSE 0 END) AS failed", false).
		Where("pool_group_id IN ? AND created_at >= ? AND created_at <= ?", groupIDs, start, end).
		Group("pool_group_id").
		Scan(&rows).Error
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		success := row.Total - row.Failed
		if success < 0 {
			success = 0
		}
		result[row.PoolGroupId] = accountPoolHealthUsage{
			Total:   row.Total,
			Failed:  row.Failed,
			Success: success,
		}
	}
	return result, nil
}

func loadAccountPoolAbnormalAccounts(groupIDs []int, groupNameByID map[int]string, limit int) ([]*AccountPoolAbnormalAccount, error) {
	limit = normalizeAccountPoolHealthAbnormalLimit(limit)
	if len(groupIDs) == 0 || limit <= 0 {
		return []*AccountPoolAbnormalAccount{}, nil
	}
	now := common.GetTimestamp()
	var accounts []*PoolAccount
	err := DB.Where("pool_group_id IN ?", groupIDs).
		Where(
			"status <> ? OR schedulable = ? OR unavailable = ? OR rate_limited_until > ? OR overload_until > ? OR temp_disabled_until > ? OR next_retry_time > ? OR last_error <> '' OR status_message <> '' OR disabled_reason <> ''",
			common.ChannelStatusEnabled,
			false,
			true,
			now,
			now,
			now,
			now,
		).
		Order("updated_time DESC").
		Order("id DESC").
		Limit(limit).
		Find(&accounts).Error
	if err != nil {
		return nil, err
	}
	result := make([]*AccountPoolAbnormalAccount, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		result = append(result, accountPoolAbnormalAccountView(account, groupNameByID[account.PoolGroupId], now))
	}
	return result, nil
}

func accountPoolAbnormalAccountView(account *PoolAccount, groupName string, now int64) *AccountPoolAbnormalAccount {
	coolingUntil := accountPoolAccountCoolingUntil(account)
	reason := accountPoolAbnormalReason(account, coolingUntil, now)
	totalRequests := account.SuccessCount + account.FailedCount
	return &AccountPoolAbnormalAccount{
		Id:                 account.Id,
		PoolGroupId:        account.PoolGroupId,
		PoolGroupName:      groupName,
		Name:               account.Name,
		Platform:           account.Platform,
		AuthType:           account.AuthType,
		CredentialProvider: account.CredentialProvider,
		Status:             account.Status,
		Schedulable:        account.Schedulable,
		Unavailable:        account.Unavailable,
		CoolingUntil:       coolingUntil,
		Reason:             reason,
		StatusMessage:      account.StatusMessage,
		DisabledReason:     account.DisabledReason,
		LastError:          account.LastError,
		LastCheckedTime:    account.LastCheckedTime,
		LastUsedTime:       account.LastUsedTime,
		NextRetryTime:      account.NextRetryTime,
		SuccessCount:       account.SuccessCount,
		FailedCount:        account.FailedCount,
		FailureRate:        ratio64(account.FailedCount, totalRequests),
	}
}

func accountPoolAccountCoolingUntil(account *PoolAccount) int64 {
	if account == nil {
		return 0
	}
	result := account.RateLimitedUntil
	if account.OverloadUntil > result {
		result = account.OverloadUntil
	}
	if account.TempDisabledUntil > result {
		result = account.TempDisabledUntil
	}
	if account.NextRetryTime > result {
		result = account.NextRetryTime
	}
	return result
}

func accountPoolAbnormalReason(account *PoolAccount, coolingUntil int64, now int64) string {
	if account == nil {
		return ""
	}
	for _, value := range []string{account.LastError, account.StatusMessage, account.DisabledReason} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if coolingUntil > now {
		return "账号正在冷却中"
	}
	if account.Status != common.ChannelStatusEnabled || !account.Schedulable {
		return "账号已禁用或不可调度"
	}
	if account.Unavailable {
		return "账号标记为不可用"
	}
	return "账号状态异常"
}

func normalizeAccountPoolHealthAbnormalLimit(limit int) int {
	if limit <= 0 {
		return accountPoolHealthDefaultAbnormalLimit
	}
	if limit > accountPoolHealthMaxAbnormalLimit {
		return accountPoolHealthMaxAbnormalLimit
	}
	return limit
}

func normalizeAccountPoolHealthAuditLimit(limit int) int {
	if limit <= 0 {
		return accountPoolHealthDefaultAuditLimit
	}
	if limit > accountPoolHealthMaxAuditLimit {
		return accountPoolHealthMaxAuditLimit
	}
	return limit
}

func ratio64(numerator int64, denominator int64) float64 {
	if numerator <= 0 || denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
