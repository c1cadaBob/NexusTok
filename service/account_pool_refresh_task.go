// account_pool_refresh_task.go
// 本文件实现了账号池凭据自动刷新的定时任务。
// 账号池（Account Pool）中的账户可能使用 OAuth 等需要定期刷新的凭据。
// 该任务定期扫描所有启用的官方 OAuth 类型账户，检查其凭据是否需要刷新，
// 并调用对应的凭据提供者（Provider）进行刷新操作。
// 仅在主节点上运行，避免多实例重复刷新。

package service

import (
	// 标准库
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/accountauth"

	// 第三方库：字节跳动的轻量级协程池
	"github.com/bytedance/gopkg/util/gopool"
)

// 账号池凭据刷新任务的配置常量
const (
	accountPoolRefreshTickInterval = 10 * time.Minute // 定时任务执行间隔
	accountPoolRefreshBatchSize    = 200              // 每批查询的账户数量
	accountPoolRefreshTimeout      = 30 * time.Second // 单个账户刷新操作的超时时间
	accountPoolRefreshRetryDelay   = 5 * time.Minute  // 刷新失败后的重试延迟
)

// 使用 sync.Once 和 atomic.Bool 确保任务只启动一次且不会并发执行
var (
	accountPoolRefreshOnce    sync.Once   // 确保定时任务只初始化一次
	accountPoolRefreshRunning atomic.Bool // 任务运行状态标记，防止并发执行
)

// StartAccountPoolCredentialAutoRefreshTask 启动账号池凭据自动刷新定时任务
// 仅在主节点上运行，使用 sync.Once 确保只启动一次
// 启动后立即执行一次刷新，之后按配置的间隔定期执行
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

// runAccountPoolCredentialAutoRefreshOnce 执行一次账号池凭据自动刷新
// 使用 atomic.Bool 的 CAS 操作防止并发执行
// 执行流程：
// 1. 分批查询所有启用的官方 OAuth 类型账户
// 2. 检查每个账户是否需要刷新（根据 NextRefreshTime 和 NextRetryTime）
// 3. 获取对应的凭据提供者并调用刷新接口
// 4. 更新账户凭据信息或标记刷新失败
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
		// 分批查询启用和自动禁用状态的官方 OAuth 账户，按 ID 升序排列
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
				continue // 凭据尚未到达刷新时间，跳过
			}
			// 获取该账户类型的凭据提供者
			provider, ok := accountauth.DefaultManager().Provider(account.GetCredentialProvider())
			if !ok || provider.RefreshLead() == nil {
				continue // 未找到对应的凭据提供者，跳过
			}
			// 设置单个账户刷新的超时上下文
			refreshCtx, cancel := context.WithTimeout(ctx, accountPoolRefreshTimeout)
			credential, err := provider.Refresh(refreshCtx, account) // 调用提供者刷新凭据
			cancel()
			if err != nil {
				markPoolAccountRefreshFailed(account, err) // 标记刷新失败，设置重试延迟
				continue
			}
			before := *account
			if err := updatePoolAccountCredentialFromAuth(account, credential); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("account pool credential auto-refresh: account_id=%d update failed: %v", account.Id, err))
				continue
			}
			model.RecordPoolAccountStateLog(model.PoolAccountStateLogRecord{
				PoolAccountId: account.Id,
				Action:        model.PoolAccountStateActionRefreshSucceeded,
				Source:        "auto_refresh",
				Reason:        "账号凭据自动刷新成功",
				Before:        &before,
			})
			refreshed++
		}
	}

	if common.DebugEnabled {
		logger.LogDebug(ctx, "account pool credential auto-refresh: scanned=%d refreshed=%d", scanned, refreshed)
	}
}

// shouldRefreshPoolAccountCredential 判断账号池账户是否需要刷新凭据
// 判断逻辑：
// 1. 账户必须存在且有凭据
// 2. 未在重试延迟期间（NextRetryTime > now 则跳过）
// 3. 未设置下次刷新时间（NextRefreshTime <= 0）则立即刷新
// 4. 已到达下次刷新时间（NextRefreshTime <= now）则刷新
// 参数:
//   - account: 账号池账户
//   - now: 当前时间戳（Unix 秒）
//
// 返回值:
//   - bool: 是否需要刷新
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

// updatePoolAccountCredentialFromAuth 使用凭据提供者返回的凭据更新账号池账户
// 更新内容包括：加密凭据、凭据摘要、提供者信息、元数据、属性、刷新时间等
// 同时重置失败状态（unavailable、last_error 等）
// 参数:
//   - account: 账号池账户
//   - credential: 凭据提供者返回的凭据信息
//
// 返回值:
//   - error: 更新失败时返回错误
func updatePoolAccountCredentialFromAuth(account *model.PoolAccount, credential *accountauth.AccountCredential) error {
	if account == nil || credential == nil {
		return fmt.Errorf("account and credential are required")
	}
	// 加密凭据字符串后存储，保障安全性
	encrypted, err := common.EncryptSensitiveString(strings.TrimSpace(credential.Credentials))
	if err != nil {
		return err
	}
	// 如果凭据摘要为空，从凭据内容中提取摘要
	summary := credential.Summary
	if strings.TrimSpace(summary) == "" {
		summary = model.NormalizeAccountPoolCredentialSummary(credential.Credentials)
	}
	// 构建更新字段映射
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

// markPoolAccountRefreshFailed 标记账号池账户刷新失败
// 设置账户为不可用状态，记录错误信息，并设置下次重试时间
// 参数:
//   - account: 账号池账户
//   - err: 刷新失败的错误信息
func markPoolAccountRefreshFailed(account *model.PoolAccount, err error) {
	if account == nil || err == nil {
		return
	}
	reason := err.Error()
	before, _ := model.GetPoolAccountById(account.Id)
	// 计算下次重试时间：当前时间 + 重试延迟
	nextRetry := common.GetTimestamp() + int64(accountPoolRefreshRetryDelay.Seconds())
	// 更新账户错误状态
	if updateErr := model.UpdatePoolAccountErrorState(account.Id, map[string]interface{}{
		"unavailable":     true,
		"status_message":  reason,
		"last_error":      reason,
		"next_retry_time": nextRetry,
	}); updateErr != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("account pool credential auto-refresh: account_id=%d mark failed: %v", account.Id, updateErr))
		return
	}
	model.RecordPoolAccountStateLog(model.PoolAccountStateLogRecord{
		PoolAccountId: account.Id,
		Action:        model.PoolAccountStateActionRefreshFailed,
		Source:        "auto_refresh",
		Reason:        reason,
		Before:        before,
	})
}

// timestampOrZero 将 time.Time 转换为 Unix 时间戳
// 如果时间为零值则返回 0
// 参数:
//   - t: 时间
//
// 返回值:
//   - int64: Unix 时间戳（秒），零值时间返回 0
func timestampOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
