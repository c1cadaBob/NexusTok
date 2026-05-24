// codex_credential_refresh_task.go
// 本文件实现了 Codex 渠道凭据的自动刷新定时任务。
// 该任务定期扫描所有启用的 Codex 渠道，检查其 OAuth 凭据是否即将过期，
// 如果在刷新阈值内则自动调用刷新接口更新凭据，确保渠道持续可用。
// 仅在主节点（master node）上运行，避免多实例重复刷新。

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
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"

	// 第三方库：字节跳动的轻量级协程池
	"github.com/bytedance/gopkg/util/gopool"
)

// 自动刷新任务的配置常量
const (
	codexCredentialRefreshTickInterval = 10 * time.Minute // 定时任务执行间隔
	codexCredentialRefreshThreshold    = 24 * time.Hour   // 凭据过期阈值：过期前 24 小时内触发刷新
	codexCredentialRefreshBatchSize    = 200              // 每批查询的渠道数量
	codexCredentialRefreshTimeout      = 15 * time.Second // 单个渠道刷新操作的超时时间
)

// 使用 sync.Once 和 atomic.Bool 确保任务只启动一次且不会并发执行
var (
	codexCredentialRefreshOnce    sync.Once   // 确保定时任务只初始化一次
	codexCredentialRefreshRunning atomic.Bool // 任务运行状态标记，防止并发执行
)

// shouldAutoRefreshCodexChannelStatus 判断渠道状态是否允许自动刷新凭据
// 只有启用状态或自动禁用状态的渠道才参与自动刷新
// 参数:
//   - status: 渠道状态码
// 返回值:
//   - bool: 是否允许自动刷新
func shouldAutoRefreshCodexChannelStatus(status int) bool {
	return status == common.ChannelStatusEnabled || status == common.ChannelStatusAutoDisabled
}

// StartCodexCredentialAutoRefreshTask 启动 Codex 凭据自动刷新定时任务
// 仅在主节点上运行，使用 sync.Once 确保只启动一次
// 启动后立即执行一次刷新，之后按配置的间隔定期执行
func StartCodexCredentialAutoRefreshTask() {
	codexCredentialRefreshOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}

		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("codex credential auto-refresh task started: tick=%s threshold=%s", codexCredentialRefreshTickInterval, codexCredentialRefreshThreshold))

			ticker := time.NewTicker(codexCredentialRefreshTickInterval)
			defer ticker.Stop()

			runCodexCredentialAutoRefreshOnce()
			for range ticker.C {
				runCodexCredentialAutoRefreshOnce()
			}
		})
	})
}

// runCodexCredentialAutoRefreshOnce 执行一次 Codex 凭据自动刷新
// 使用 atomic.Bool 的 CAS 操作防止并发执行
// 执行流程：
// 1. 分批查询所有启用的 Codex 渠道（跳过多 Key 渠道）
// 2. 解析每个渠道的 OAuth 凭据
// 3. 检查凭据是否在刷新阈值内即将过期
// 4. 对需要刷新的渠道调用 RefreshCodexChannelCredential
// 5. 刷新完成后重置渠道缓存
func runCodexCredentialAutoRefreshOnce() {
	if !codexCredentialRefreshRunning.CompareAndSwap(false, true) {
		return
	}
	defer codexCredentialRefreshRunning.Store(false)

	ctx := context.Background()
	now := time.Now()

	var refreshed int
	var scanned int

	offset := 0
	for {
		// 分批查询启用和自动禁用状态的 Codex 渠道，按 ID 升序排列
		var channels []*model.Channel
		err := model.DB.
			Select("id", "name", "key", "status", "channel_info").
			Where("type = ? AND (status = ? OR status = ?)",
				constant.ChannelTypeCodex,
				common.ChannelStatusEnabled,
				common.ChannelStatusAutoDisabled,
			).
			Order("id asc").
			Limit(codexCredentialRefreshBatchSize).
			Offset(offset).
			Find(&channels).Error
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("codex credential auto-refresh: query channels failed: %v", err))
			return
		}
		if len(channels) == 0 {
			break
		}
		offset += codexCredentialRefreshBatchSize

		for _, ch := range channels {
			if ch == nil {
				continue
			}
			scanned++
			if ch.ChannelInfo.IsMultiKey {
				continue // 跳过多 Key 渠道，多 Key 渠道有独立的凭据管理机制
			}

			rawKey := strings.TrimSpace(ch.Key)
			if rawKey == "" {
				continue // 跳过没有 Key 的渠道
			}

			oauthKey, err := parseCodexOAuthKey(rawKey)
			if err != nil {
				continue // 跳过 JSON 解析失败的渠道
			}

			refreshToken := strings.TrimSpace(oauthKey.RefreshToken)
			if refreshToken == "" {
				continue // 跳过没有 refresh_token 的渠道
			}

			// 检查凭据是否在刷新阈值内即将过期
			// 如果过期时间距今超过阈值（24小时），则无需刷新
			expiredAtRaw := strings.TrimSpace(oauthKey.Expired)
			expiredAt, err := time.Parse(time.RFC3339, expiredAtRaw)
			if err == nil && !expiredAt.IsZero() && expiredAt.Sub(now) > codexCredentialRefreshThreshold {
				continue // 凭据尚未接近过期，跳过
			}

			// 设置单个渠道刷新的超时上下文
			refreshCtx, cancel := context.WithTimeout(ctx, codexCredentialRefreshTimeout)
			newKey, _, err := RefreshCodexChannelCredential(refreshCtx, ch.Id, CodexCredentialRefreshOptions{ResetCaches: false})
			cancel()
			if err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("codex credential auto-refresh: channel_id=%d name=%s refresh failed: %v", ch.Id, ch.Name, err))
				continue
			}

			refreshed++
			logger.LogInfo(ctx, fmt.Sprintf("codex credential auto-refresh: channel_id=%d name=%s refreshed, expires_at=%s", ch.Id, ch.Name, newKey.Expired))
		}
	}

	// 如果有渠道被刷新，重置缓存以使新凭据生效
	if refreshed > 0 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.LogWarn(ctx, fmt.Sprintf("codex credential auto-refresh: InitChannelCache panic: %v", r))
				}
			}()
			model.InitChannelCache() // 重置渠道缓存（带 panic 保护）
		}()
		ResetProxyClientCache() // 重置代理客户端缓存
	}

	if common.DebugEnabled {
		logger.LogDebug(ctx, "codex credential auto-refresh: scanned=%d refreshed=%d", scanned, refreshed)
	}
}
