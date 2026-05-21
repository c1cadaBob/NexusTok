package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/accountauth"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	accountPoolRefreshTickInterval = 10 * time.Minute
	accountPoolRefreshBatchSize    = 200
	accountPoolRefreshTimeout      = 30 * time.Second
	accountPoolRefreshRetryDelay   = 5 * time.Minute
)

var (
	accountPoolRefreshOnce    sync.Once
	accountPoolRefreshRunning atomic.Bool
)

func StartAccountPoolCredentialAutoRefreshTask() {
	accountPoolRefreshOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("account pool credential auto-refresh task started: tick=%s", accountPoolRefreshTickInterval))
			ticker := time.NewTicker(accountPoolRefreshTickInterval)
			defer ticker.Stop()
			runAccountPoolCredentialAutoRefreshOnce()
			for range ticker.C {
				runAccountPoolCredentialAutoRefreshOnce()
			}
		})
	})
}

func runAccountPoolCredentialAutoRefreshOnce() {
	if !accountPoolRefreshRunning.CompareAndSwap(false, true) {
		return
	}
	defer accountPoolRefreshRunning.Store(false)

	ctx := context.Background()
	now := common.GetTimestamp()
	offset := 0
	var scanned int
	var refreshed int

	for {
		var accounts []*model.PoolAccount
		err := model.DB.
			Where("auth_type = ? AND (status = ? OR status = ?)",
				model.AccountPoolAuthTypeOfficialOAuth,
				common.ChannelStatusEnabled,
				common.ChannelStatusAutoDisabled,
			).
			Order("id asc").
			Limit(accountPoolRefreshBatchSize).
			Offset(offset).
			Find(&accounts).Error
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("account pool credential auto-refresh: query accounts failed: %v", err))
			return
		}
		if len(accounts) == 0 {
			break
		}
		offset += accountPoolRefreshBatchSize

		for _, account := range accounts {
			if account == nil {
				continue
			}
			scanned++
			if !shouldRefreshPoolAccountCredential(account, now) {
				continue
			}
			provider, ok := accountauth.DefaultManager().Provider(account.GetCredentialProvider())
			if !ok || provider.RefreshLead() == nil {
				continue
			}
			refreshCtx, cancel := context.WithTimeout(ctx, accountPoolRefreshTimeout)
			credential, err := provider.Refresh(refreshCtx, account)
			cancel()
			if err != nil {
				markPoolAccountRefreshFailed(account, err)
				continue
			}
			if err := updatePoolAccountCredentialFromAuth(account, credential); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("account pool credential auto-refresh: account_id=%d update failed: %v", account.Id, err))
				continue
			}
			refreshed++
		}
	}

	if common.DebugEnabled {
		logger.LogDebug(ctx, "account pool credential auto-refresh: scanned=%d refreshed=%d", scanned, refreshed)
	}
}

func shouldRefreshPoolAccountCredential(account *model.PoolAccount, now int64) bool {
	if account == nil || strings.TrimSpace(account.Credentials) == "" {
		return false
	}
	if account.NextRetryTime > now {
		return false
	}
	if account.NextRefreshTime <= 0 {
		return true
	}
	return account.NextRefreshTime <= now
}

func updatePoolAccountCredentialFromAuth(account *model.PoolAccount, credential *accountauth.AccountCredential) error {
	if account == nil || credential == nil {
		return fmt.Errorf("account and credential are required")
	}
	encrypted, err := common.EncryptSensitiveString(strings.TrimSpace(credential.Credentials))
	if err != nil {
		return err
	}
	summary := credential.Summary
	if strings.TrimSpace(summary) == "" {
		summary = model.NormalizeAccountPoolCredentialSummary(credential.Credentials)
	}
	updates := map[string]interface{}{
		"credentials":           encrypted,
		"credential_summary":    summary,
		"credential_provider":   credential.Provider,
		"credential_label":      credential.Label,
		"credential_metadata":   accountauth.MetadataToJSON(credential.Metadata),
		"credential_attributes": accountauth.AttributesToJSON(credential.Attributes),
		"last_refreshed_time":   timestampOrZero(credential.LastRefreshedAt),
		"next_refresh_time":     timestampOrZero(credential.NextRefreshAt),
		"next_retry_time":       0,
		"unavailable":           false,
		"schedulable":           true,
		"last_error":            "",
		"status_message":        "",
	}
	if strings.TrimSpace(credential.Provider) != "" {
		updates["platform"] = credential.Provider
	}
	if strings.TrimSpace(credential.AuthType) != "" {
		updates["auth_type"] = credential.AuthType
	}
	if strings.TrimSpace(credential.Label) != "" {
		updates["name"] = credential.Label
	}
	return model.DB.Model(account).Updates(updates).Error
}

func markPoolAccountRefreshFailed(account *model.PoolAccount, err error) {
	if account == nil || err == nil {
		return
	}
	reason := err.Error()
	nextRetry := common.GetTimestamp() + int64(accountPoolRefreshRetryDelay.Seconds())
	if updateErr := model.UpdatePoolAccountErrorState(account.Id, map[string]interface{}{
		"unavailable":     true,
		"status_message":  reason,
		"last_error":      reason,
		"next_retry_time": nextRetry,
	}); updateErr != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("account pool credential auto-refresh: account_id=%d mark failed: %v", account.Id, updateErr))
	}
}

func timestampOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
