// Package perfmetrics - flush.go
// 该文件实现了性能指标的定期刷新功能
//
// 核心功能：
// - flushLoop：后台循环刷新性能指标到数据库
// - 根据配置的刷新间隔定期执行
// - 聚合内存中的指标数据并批量写入
package perfmetrics

import (
	"fmt"
	"strconv"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/perf_metrics_setting"
)

// flushLoop 后台循环刷新性能指标到数据库
// 按照配置的刷新间隔定期执行：
// 1. 将已完成的时间桶数据从内存刷新到数据库
// 2. 清理过期的历史指标数据
// 此函数在 Init() 中作为 goroutine 启动，永不停止
func flushLoop() {
	for {
		interval := perf_metrics_setting.GetFlushIntervalMinutes()
		time.Sleep(time.Duration(interval) * time.Minute)
		setting := perf_metrics_setting.GetSetting()
		if !setting.Enabled {
			continue
		}
		flushCompletedBuckets()
		cleanupExpiredMetrics(setting.RetentionDays)
	}
}

// flushCompletedBuckets 将已完成的时间桶数据刷新到数据库
// 遍历所有内存中的时间桶，跳过当前仍在写入的桶
// 对于已完成的桶：排空计数器 -> 写入数据库 -> 清理旧的空桶
// 如果数据库写入失败，将排空的数据重新加回计数器（保证不丢数据）
func flushCompletedBuckets() {
	currentBucket := bucketStart(time.Now().Unix())
	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if k.bucketTs >= currentBucket {
			return true
		}

		bucket := value.(*atomicBucket)
		drained := bucket.drain()
		if drained.requestCount == 0 {
			deleteOldEmptyBucket(k, key)
			return true
		}

		err := model.UpsertPerfMetric(&model.PerfMetric{
			ModelName:      k.model,
			Group:          k.group,
			BucketTs:       k.bucketTs,
			RequestCount:   drained.requestCount,
			SuccessCount:   drained.successCount,
			TotalLatencyMs: drained.totalLatencyMs,
			TtftSumMs:      drained.ttftSumMs,
			TtftCount:      drained.ttftCount,
			OutputTokens:   drained.outputTokens,
			GenerationMs:   drained.generationMs,
		})
		if err != nil {
			bucket.addCounters(drained)
			common.SysError(fmt.Sprintf("failed to flush perf metric bucket model=%s group=%s bucket=%d: %s", k.model, k.group, k.bucketTs, err.Error()))
			return true
		}

		deleteOldEmptyBucket(k, key)
		return true
	})
}

// deleteOldEmptyBucket 删除超过 24 小时的空桶
// 防止长时间无请求的桶一直占用内存
func deleteOldEmptyBucket(k bucketKey, rawKey any) {
	if k.bucketTs < bucketStart(time.Now().Add(-24*time.Hour).Unix()) {
		hotBuckets.Delete(rawKey)
	}
}

// cleanupExpiredMetrics 清理过期的历史性能指标数据
// 根据配置的保留天数，删除数据库中超过保留期的指标记录
func cleanupExpiredMetrics(retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
	if err := model.DeletePerfMetricsBefore(cutoff); err != nil {
		common.SysError("failed to cleanup expired perf metrics: " + err.Error())
	}
}

// redisCounters 从 Redis Hash 的键值对中解析出计数器数据
// Redis 中的键名映射：req -> requestCount, ok -> successCount, lat -> totalLatencyMs,
// ttft -> ttftSumMs, ttft_n -> ttftCount, out -> outputTokens, gen_ms -> generationMs
func redisCounters(values map[string]string) counters {
	return counters{
		requestCount:   parseRedisInt(values["req"]),
		successCount:   parseRedisInt(values["ok"]),
		totalLatencyMs: parseRedisInt(values["lat"]),
		ttftSumMs:      parseRedisInt(values["ttft"]),
		ttftCount:      parseRedisInt(values["ttft_n"]),
		outputTokens:   parseRedisInt(values["out"]),
		generationMs:   parseRedisInt(values["gen_ms"]),
	}
}

// parseRedisInt 从 Redis 字符串值解析 int64
// 空字符串返回 0，解析失败也返回 0（不报错）
func parseRedisInt(value string) int64 {
	if value == "" {
		return 0
	}
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}
