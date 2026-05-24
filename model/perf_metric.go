// Package model - perf_metric.go
// 该文件定义了性能指标（PerfMetric）数据模型及相关操作
//
// 主要结构体：
// - PerfMetric：模型广场展示的聚合性能指标
//
// 核心功能：
// - 性能指标的批量写入和更新
// - 按模型和分组查询性能指标
// - 支持时间桶（bucket）聚合
package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PerfMetric 性能指标数据模型
// 存储模型广场展示的聚合性能指标，按时间桶（bucket）聚合
type PerfMetric struct {
	Id             int    `json:"id" gorm:"primaryKey"`                                                          // 指标 ID
	ModelName      string `json:"model_name" gorm:"size:128;uniqueIndex:idx_perf_model_group_bucket,priority:1"` // 模型名称
	Group          string `json:"group" gorm:"column:group;size:64;uniqueIndex:idx_perf_model_group_bucket,priority:2"` // 分组
	BucketTs       int64  `json:"bucket_ts" gorm:"uniqueIndex:idx_perf_model_group_bucket,priority:3;index:idx_perf_bucket_ts"` // 时间桶时间戳
	RequestCount   int64  `json:"-" gorm:"default:0"`   // 请求数量
	SuccessCount   int64  `json:"-" gorm:"default:0"`   // 成功数量
	TotalLatencyMs int64  `json:"-" gorm:"default:0"`   // 总延迟（毫秒）
	TtftSumMs      int64  `json:"-" gorm:"default:0"`   // TTFT 总和（毫秒，Time To First Token）
	TtftCount      int64  `json:"-" gorm:"default:0"`   // TTFT 计数
	OutputTokens   int64  `json:"-" gorm:"default:0"`   // 输出 Token 数量
	GenerationMs   int64  `json:"-" gorm:"default:0"`   // 生成时间（毫秒）
}

// TableName 指定性能指标表名
func (PerfMetric) TableName() string {
	return "perf_metrics"
}

// UpsertPerfMetric 插入或更新性能指标
// 使用 ON CONFLICT DO UPDATE 策略，累加各项指标值
//
// 参数：
//   - metric: 性能指标对象
//
// 返回值：
//   - error: 操作失败时返回错误
func UpsertPerfMetric(metric *PerfMetric) error {
	if metric == nil || metric.RequestCount == 0 {
		return nil
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "model_name"},
			{Name: "group"},
			{Name: "bucket_ts"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"request_count":    gorm.Expr("request_count + ?", metric.RequestCount),
			"success_count":    gorm.Expr("success_count + ?", metric.SuccessCount),
			"total_latency_ms": gorm.Expr("total_latency_ms + ?", metric.TotalLatencyMs),
			"ttft_sum_ms":      gorm.Expr("ttft_sum_ms + ?", metric.TtftSumMs),
			"ttft_count":       gorm.Expr("ttft_count + ?", metric.TtftCount),
			"output_tokens":    gorm.Expr("output_tokens + ?", metric.OutputTokens),
			"generation_ms":    gorm.Expr("generation_ms + ?", metric.GenerationMs),
		}),
	}).Create(metric).Error
}

// GetPerfMetrics 获取指定模型和时间范围的性能指标
//
// 参数：
//   - modelName: 模型名称
//   - group: 分组（可选，为空则查询所有分组）
//   - startTs: 开始时间戳
//   - endTs: 结束时间戳
//
// 返回值：
//   - []PerfMetric: 性能指标列表
//   - error: 查询失败时返回错误
func GetPerfMetrics(modelName string, group string, startTs int64, endTs int64) ([]PerfMetric, error) {
	var metrics []PerfMetric
	query := DB.Model(&PerfMetric{}).
		Where("model_name = ? AND bucket_ts >= ? AND bucket_ts <= ?", modelName, startTs, endTs)
	if group != "" {
		query = query.Where(commonGroupCol+" = ?", group)
	}
	err := query.Order("bucket_ts ASC").Find(&metrics).Error
	return metrics, err
}

// PerfMetricSummary 性能指标汇总
// 用于展示模型的聚合性能数据
type PerfMetricSummary struct {
	ModelName      string `json:"model_name"`      // 模型名称
	RequestCount   int64  `json:"request_count"`   // 请求数量
	SuccessCount   int64  `json:"success_count"`   // 成功数量
	TotalLatencyMs int64  `json:"total_latency_ms"` // 总延迟（毫秒）
	OutputTokens   int64  `json:"output_tokens"`   // 输出 Token 数量
	GenerationMs   int64  `json:"generation_ms"`   // 生成时间（毫秒）
}

// GetPerfMetricsSummaryAll 获取所有模型的性能指标汇总
//
// 参数：
//   - startTs: 开始时间戳
//   - endTs: 结束时间戳
//
// 返回值：
//   - []PerfMetricSummary: 性能指标汇总列表
//   - error: 查询失败时返回错误
func GetPerfMetricsSummaryAll(startTs int64, endTs int64) ([]PerfMetricSummary, error) {
	var summaries []PerfMetricSummary
	err := DB.Model(&PerfMetric{}).
		Select("model_name, SUM(request_count) as request_count, SUM(success_count) as success_count, SUM(total_latency_ms) as total_latency_ms, SUM(output_tokens) as output_tokens, SUM(generation_ms) as generation_ms").
		Where("bucket_ts >= ? AND bucket_ts <= ?", startTs, endTs).
		Group("model_name").
		Having("SUM(request_count) > 0").
		Find(&summaries).Error
	return summaries, err
}

// DeletePerfMetricsBefore 删除指定时间之前的性能指标
// 用于清理过期的历史数据
//
// 参数：
//   - cutoffTs: 截止时间戳
//
// 返回值：
//   - error: 删除失败时返回错误
func DeletePerfMetricsBefore(cutoffTs int64) error {
	if cutoffTs <= 0 {
		return nil
	}
	return DB.Where("bucket_ts < ?", cutoffTs).Delete(&PerfMetric{}).Error
}

// PerfMetricStartTime 计算性能指标的开始时间
//
// 参数：
//   - hours: 回溯小时数（默认 24 小时）
//
// 返回值：
//   - int64: 开始时间的 UNIX 时间戳
func PerfMetricStartTime(hours int) int64 {
	if hours <= 0 {
		hours = 24
	}
	return time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
}
