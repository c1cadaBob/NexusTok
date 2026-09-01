package controller

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/service/upstreamaccount"
	"github.com/c1cada/NexusTok/service/upstreammodel"
	"github.com/c1cada/NexusTok/setting/operation_setting"
)

// upstreamAccountKeyCheckHandler 执行同步密钥级自动连接测试。
type upstreamAccountKeyCheckHandler struct{}

type upstreamAccountKeyCheckSummary struct {
	ScannedAccounts   int                              `json:"scanned_accounts"`
	EligibleAccounts  int                              `json:"eligible_accounts"`
	SkippedAccounts   int                              `json:"skipped_accounts"`
	SucceededAccounts int                              `json:"succeeded_accounts"`
	FailedAccounts    int                              `json:"failed_accounts"`
	DisabledAccounts  int                              `json:"disabled_accounts"`
	RecoveredAccounts int                              `json:"recovered_accounts"`
	Cancelled         bool                             `json:"cancelled,omitempty"`
	Failures          []upstreamAccountKeyCheckFailure `json:"failures,omitempty"`
}

type upstreamAccountKeyCheckFailure struct {
	ChannelID    int    `json:"channel_id"`
	ChannelName  string `json:"channel_name"`
	AccountID    int    `json:"account_id"`
	AccountName  string `json:"account_name"`
	FailureCount int    `json:"failure_count"`
	AutoDisabled bool   `json:"auto_disabled,omitempty"`
	Error        string `json:"error"`
}

func init() {
	service.RegisterSystemTaskHandler(upstreamAccountKeyCheckHandler{})
}

func (upstreamAccountKeyCheckHandler) Type() string {
	return model.SystemTaskTypeUpstreamAccountKeyCheck
}

func (upstreamAccountKeyCheckHandler) Enabled() bool {
	return operation_setting.GetUpstreamAccountKeyCheckSetting().Enabled
}

func (upstreamAccountKeyCheckHandler) Interval() time.Duration {
	return operation_setting.GetUpstreamAccountKeyCheckSetting().Interval()
}

func (upstreamAccountKeyCheckHandler) NewPayload() any {
	return nil
}

func (upstreamAccountKeyCheckHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := runUpstreamAccountKeyCheckTask(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	if summary != nil && summary.Cancelled {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, context.Canceled)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

func runUpstreamAccountKeyCheckTask(ctx context.Context, report func(processed, total int)) (*upstreamAccountKeyCheckSummary, error) {
	setting := operation_setting.GetUpstreamAccountKeyCheckSetting()
	accounts, err := queryUpstreamAccountKeyCheckAccounts()
	if err != nil {
		return nil, err
	}
	summary := &upstreamAccountKeyCheckSummary{
		ScannedAccounts: len(accounts),
		Failures:        make([]upstreamAccountKeyCheckFailure, 0),
	}
	if report != nil {
		report(0, len(accounts))
	}
	channelMap, err := loadChannelsForAccounts(accounts)
	if err != nil {
		return nil, err
	}
	changedChannelIDs := map[int]struct{}{}

	for index := range accounts {
		if err := ctx.Err(); err != nil {
			summary.Cancelled = true
			return summary, nil
		}
		account := &accounts[index]
		channel := channelMap[account.ChannelId]
		if !upstreamAccountKeyCheckEligible(setting, channel, account) {
			summary.SkippedAccounts++
			if report != nil {
				report(index+1, len(accounts))
			}
			continue
		}
		summary.EligibleAccounts++
		recovered, disabled, checkErr := checkSingleUpstreamAccountKey(setting, channel, account)
		if checkErr != nil {
			summary.FailedAccounts++
			if disabled {
				summary.DisabledAccounts++
				changedChannelIDs[account.ChannelId] = struct{}{}
			}
			autoMetadata := upstreamaccount.ReadAccountAutoCheckMetadata(account.OtherSettings)
			summary.Failures = append(summary.Failures, upstreamAccountKeyCheckFailure{
				ChannelID:    account.ChannelId,
				ChannelName:  channelName(channel),
				AccountID:    account.Id,
				AccountName:  account.Name,
				FailureCount: autoMetadata.FailureCount,
				AutoDisabled: disabled,
				Error:        common.MaskSensitiveInfo(checkErr.Error()),
			})
		} else {
			summary.SucceededAccounts++
			if recovered {
				summary.RecoveredAccounts++
				changedChannelIDs[account.ChannelId] = struct{}{}
			}
		}
		if report != nil {
			report(index+1, len(accounts))
		}
	}
	if err := refreshChangedAccountChannels(changedChannelIDs); err != nil {
		return summary, err
	}
	return summary, nil
}

func queryUpstreamAccountKeyCheckAccounts() ([]model.ChannelAccount, error) {
	var accounts []model.ChannelAccount
	err := model.DB.
		Where("settings LIKE ?", "%upstream_account_sync%").
		Order("channel_id asc").
		Order("id asc").
		Find(&accounts).Error
	return accounts, err
}

func loadChannelsForAccounts(accounts []model.ChannelAccount) (map[int]*model.Channel, error) {
	ids := make([]int, 0, len(accounts))
	seen := map[int]struct{}{}
	for _, account := range accounts {
		if account.ChannelId <= 0 {
			continue
		}
		if _, ok := seen[account.ChannelId]; ok {
			continue
		}
		seen[account.ChannelId] = struct{}{}
		ids = append(ids, account.ChannelId)
	}
	if len(ids) == 0 {
		return map[int]*model.Channel{}, nil
	}
	sort.Ints(ids)
	var channels []*model.Channel
	if err := model.DB.Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return nil, err
	}
	result := make(map[int]*model.Channel, len(channels))
	for _, channel := range channels {
		result[channel.Id] = channel
	}
	return result, nil
}

func upstreamAccountKeyCheckEligible(
	setting *operation_setting.UpstreamAccountKeyCheckSetting,
	channel *model.Channel,
	account *model.ChannelAccount,
) bool {
	if setting == nil || !setting.Enabled || channel == nil || account == nil {
		return false
	}
	if !upstreamaccount.HasAccountSyncMetadata(account.OtherSettings) {
		return false
	}
	if channel.Status != common.ChannelStatusEnabled {
		return false
	}
	autoMetadata := upstreamaccount.ReadAccountAutoCheckMetadata(account.OtherSettings)
	switch account.Status {
	case common.ChannelStatusEnabled:
	case common.ChannelStatusAutoDisabled:
		if !autoMetadata.DisabledByAutoCheck {
			return false
		}
	default:
		return false
	}
	if strings.TrimSpace(account.Key) == "" {
		return false
	}
	if setting.RatioThreshold > 0 {
		ratio, ok := accountAutoCheckConvertedRatio(autoMetadata)
		if !ok || ratio >= setting.RatioThreshold {
			return false
		}
	}
	return true
}

func accountAutoCheckConvertedRatio(metadata upstreamaccount.AccountAutoCheckMetadata) (float64, bool) {
	if metadata.RatioConversion > 0 {
		return metadata.RatioConversion, true
	}
	return 0, false
}

func checkSingleUpstreamAccountKey(
	setting *operation_setting.UpstreamAccountKeyCheckSetting,
	channel *model.Channel,
	account *model.ChannelAccount,
) (recovered bool, disabled bool, err error) {
	tempChannel := upstreammodel.ChannelWithAccountCredential(channel, account)
	startedAt := time.Now()
	_, fetchErr := upstreammodel.FetchChannelModelIDs(tempChannel)
	durationMs := time.Since(startedAt).Milliseconds()
	if fetchErr == nil {
		return applyUpstreamAccountKeyCheckSuccess(setting, account, durationMs)
	}
	return applyUpstreamAccountKeyCheckFailure(setting, account, fetchErr)
}

func applyUpstreamAccountKeyCheckSuccess(
	setting *operation_setting.UpstreamAccountKeyCheckSetting,
	account *model.ChannelAccount,
	durationMs int64,
) (bool, bool, error) {
	settings := upstreamaccount.ApplyAccountAutoCheckAutomaticSuccess(account.OtherSettings, durationMs)
	updates := map[string]any{
		"settings": settings,
	}
	recovered := false
	autoMetadata := upstreamaccount.ReadAccountAutoCheckMetadata(account.OtherSettings)
	updatedMetadata := upstreamaccount.ReadAccountAutoCheckMetadata(settings)
	if account.Status == common.ChannelStatusAutoDisabled &&
		autoMetadata.DisabledByAutoCheck &&
		setting != nil &&
		setting.AutoRecoverEnabled &&
		updatedMetadata.FastSuccessStreak >= 2 {
		settings = upstreamaccount.ApplyAccountAutoCheckRecoveryMarker(settings)
		recovered = true
		updates["status"] = common.ChannelStatusEnabled
		updates["settings"] = settings
		updates["disabled_reason"] = ""
		updates["last_error"] = ""
		updates["rate_limited_until"] = 0
		updates["overload_until"] = 0
		updates["temp_disabled_until"] = 0
	}
	if err := model.DB.Model(&model.ChannelAccount{}).Where("id = ?", account.Id).Updates(updates).Error; err != nil {
		return false, false, err
	}
	account.OtherSettings = settings
	if recovered {
		account.Status = common.ChannelStatusEnabled
	}
	return recovered, false, nil
}

func applyUpstreamAccountKeyCheckFailure(
	setting *operation_setting.UpstreamAccountKeyCheckSetting,
	account *model.ChannelAccount,
	checkErr error,
) (bool, bool, error) {
	autoMetadata := upstreamaccount.ReadAccountAutoCheckMetadata(account.OtherSettings)
	failureCount := autoMetadata.FailureCount + 1
	threshold := setting.NormalizedFailureThreshold()
	shouldDisable := account.Status == common.ChannelStatusEnabled && failureCount >= threshold
	disabledByAutoCheck := autoMetadata.DisabledByAutoCheck || shouldDisable
	errText := sanitizeUpstreamAccountKeyCheckError(checkErr, account)
	settings := upstreamaccount.ApplyAccountAutoCheckFailure(account.OtherSettings, failureCount, errText, disabledByAutoCheck)
	updates := map[string]any{
		"settings":   settings,
		"last_error": errText,
	}
	if shouldDisable {
		updates["status"] = common.ChannelStatusAutoDisabled
		updates["disabled_reason"] = errText
		updates["rate_limited_until"] = 0
		updates["overload_until"] = 0
		updates["temp_disabled_until"] = 0
	}
	if err := model.DB.Model(&model.ChannelAccount{}).Where("id = ?", account.Id).Updates(updates).Error; err != nil {
		return false, false, err
	}
	account.OtherSettings = settings
	if shouldDisable {
		account.Status = common.ChannelStatusAutoDisabled
	}
	return false, shouldDisable, errors.New(errText)
}

func sanitizeUpstreamAccountKeyCheckError(checkErr error, account *model.ChannelAccount) string {
	errText := "同步密钥连接测试失败"
	if checkErr != nil {
		errText = checkErr.Error()
	}
	errText = common.MaskSensitiveInfo(errText)
	if account != nil {
		if key := strings.TrimSpace(account.Key); key != "" {
			errText = strings.ReplaceAll(errText, key, "[redacted-key]")
		}
	}
	return errText
}

func refreshChangedAccountChannels(channelIDs map[int]struct{}) error {
	if len(channelIDs) == 0 {
		return nil
	}
	ids := make([]int, 0, len(channelIDs))
	for channelID := range channelIDs {
		ids = append(ids, channelID)
	}
	sort.Ints(ids)
	for _, channelID := range ids {
		if err := model.SyncChannelAccountPoolCapabilities(channelID, nil); err != nil {
			return err
		}
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()
	return nil
}

func channelName(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	return channel.Name
}
