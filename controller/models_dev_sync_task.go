// Package controller - models_dev_sync_task.go
// 该文件实现 models.dev 模型目录的每日自动同步任务。
//
// 任务目标：
// - 每天凌晨从 https://models.dev/catalog.json 拉取公开模型目录；
// - 只创建本地尚不存在的模型和供应商，不覆盖管理员手动编辑；
// - 同步 provider 价格到模型级定价配置，但默认不覆盖管理员手动确认过的价格；
// - 不修改渠道能力、用户配置等业务数据；
// - 仅在主节点运行，避免多实例部署时重复写库。
package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	modelsDevSyncDefaultTime    = "02:00"
	modelsDevSyncDefaultTimeout = 2 * time.Minute
)

var (
	modelsDevSyncTaskOnce    sync.Once
	modelsDevSyncTaskRunning atomic.Bool
)

// StartModelsDevSyncTask 启动 models.dev 每日模型目录同步任务。
//
// 环境变量：
// - MODELS_DEV_AUTO_SYNC_ENABLED：是否启用，默认 true；
// - MODELS_DEV_AUTO_SYNC_TIME：每天运行时间，格式 HH:mm，默认 02:00；
// - MODELS_DEV_SYNC_BASE：models.dev 基础 URL，默认 https://models.dev。
func StartModelsDevSyncTask() {
	modelsDevSyncTaskOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		if !common.GetEnvOrDefaultBool("MODELS_DEV_AUTO_SYNC_ENABLED", true) {
			logger.LogInfo(context.Background(), "models.dev model sync task disabled by MODELS_DEV_AUTO_SYNC_ENABLED")
			return
		}

		scheduleTime := common.GetEnvOrDefaultString("MODELS_DEV_AUTO_SYNC_TIME", modelsDevSyncDefaultTime)
		hour, minute, ok := parseDailyScheduleTime(scheduleTime)
		if !ok {
			logger.LogWarn(context.Background(), fmt.Sprintf("models.dev model sync task got invalid MODELS_DEV_AUTO_SYNC_TIME=%q, fallback to %s", scheduleTime, modelsDevSyncDefaultTime))
			hour, minute, _ = parseDailyScheduleTime(modelsDevSyncDefaultTime)
			scheduleTime = modelsDevSyncDefaultTime
		}

		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("models.dev model sync task started: time=%s source=%s", scheduleTime, getModelsDevCatalogURL()))
			for {
				nextRun := nextDailyScheduleTime(time.Now(), hour, minute)
				timer := time.NewTimer(time.Until(nextRun))
				<-timer.C
				runModelsDevSyncTaskOnce()
			}
		})
	})
}

// runModelsDevSyncTaskOnce 执行一次 models.dev 自动同步。
func runModelsDevSyncTaskOnce() {
	if !modelsDevSyncTaskRunning.CompareAndSwap(false, true) {
		return
	}
	defer modelsDevSyncTaskRunning.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), modelsDevSyncDefaultTimeout)
	defer cancel()

	result, err := syncUpstreamModelsCore(ctx, syncRequest{
		Source: syncSourceModelsDev,
		Pricing: syncPricingPolicyRequest{
			Enabled:         true,
			OverwriteManual: false,
			ProviderOrder:   parseModelsDevProviderOrderEnv(),
		},
	}, syncUpstreamOptions{CreateAllUpstream: true})
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("models.dev model sync failed: %v", err))
		return
	}
	logger.LogInfo(ctx, fmt.Sprintf(
		"models.dev model sync completed: created_models=%d created_vendors=%d updated_models=%d pricing_updated=%d pricing_skipped=%d skipped_models=%d source=%s",
		result.CreatedModels,
		result.CreatedVendors,
		result.UpdatedModels,
		result.PricingUpdated,
		result.PricingSkipped,
		len(result.SkippedModels),
		result.Source.CatalogURL,
	))
}

// parseModelsDevProviderOrderEnv 读取自动同步的价格 provider 降级链。
// 例如 MODELS_DEV_PRICING_PROVIDER_ORDER=openai,azure,openrouter。
func parseModelsDevProviderOrderEnv() []string {
	raw := common.GetEnvOrDefaultString("MODELS_DEV_PRICING_PROVIDER_ORDER", "")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	order := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			order = append(order, trimmed)
		}
	}
	return order
}

// parseDailyScheduleTime 解析每日任务时间，格式为 HH:mm。
func parseDailyScheduleTime(value string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}

// nextDailyScheduleTime 计算下一次每日任务时间。
//
// 使用当前进程时区，容器部署时可通过 TZ 控制业务期望的凌晨时区。
func nextDailyScheduleTime(now time.Time, hour int, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}
