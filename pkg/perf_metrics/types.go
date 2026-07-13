// Package perfmetrics - types.go
// 该文件定义了性能指标系统的数据类型
//
// 主要接口：
// - Store：性能指标存储接口（Record/Query）
//
// 主要类型：
// - Sample：单次请求采样数据
// - QueryParams：查询参数
// - QueryResult：查询结果
//
// 用途：
// - 在中继层记录请求性能数据
// - 支持模型广场的性能指标展示
package perfmetrics

import "sync/atomic"

// Store 定义性能指标存储的接口
// 提供记录采样数据和查询聚合结果两个核心方法
type Store interface {
	// Record 记录一次请求的采样数据
	Record(sample Sample)
	// Query 根据查询参数返回聚合后的性能指标结果
	Query(params QueryParams) (QueryResult, error)
}

// Sample 表示单次请求的采样数据
// 在中继层完成请求后记录，包含延迟、TTFT、Token 数等维度
type Sample struct {
	Model        string // 模型名称（如 "gpt-4"）
	Group        string // 用户分组（如 "default"、"vip"）
	LatencyMs    int64  // 端到端延迟（毫秒）
	TtftMs       int64  // 首 Token 延迟（毫秒，仅流式请求有效）
	HasTtft      bool   // 是否有 TTFT 数据（仅流式且已发送首个响应时为 true）
	Success      bool   // 请求是否成功
	OutputTokens int64  // 输出 Token 数（用于计算 TPS）
	GenerationMs int64  // 生成阶段耗时（毫秒，从首 Token 到完成）
}

// QueryParams 表示性能指标查询参数
type QueryParams struct {
	Model string // 模型名称（必填）
	Group string // 用户分组（可选，为空则查询所有分组）
	Hours int    // 查询时间范围（小时），默认 24，最大 720（30 天）
}

// BucketPoint 表示单个时间桶的聚合指标点
// 用于绘制时间序列图表
type BucketPoint struct {
	Ts           int64   `json:"ts"`             // 时间桶的起始时间戳（Unix 秒）
	AvgTtftMs    int64   `json:"avg_ttft_ms"`    // 平均首 Token 延迟（毫秒）
	AvgLatencyMs int64   `json:"avg_latency_ms"` // 平均端到端延迟（毫秒）
	SuccessRate  float64 `json:"success_rate"`   // 成功率（百分比，0-100）
	AvgTps       float64 `json:"avg_tps"`        // 平均 Token 生成速度（tokens/秒）
}

// GroupResult 表示按用户分组聚合的性能指标结果
type GroupResult struct {
	Group        string        `json:"group"`          // 用户分组名称
	AvgTtftMs    int64         `json:"avg_ttft_ms"`    // 该分组的平均首 Token 延迟
	AvgLatencyMs int64         `json:"avg_latency_ms"` // 该分组的平均端到端延迟
	SuccessRate  float64       `json:"success_rate"`   // 该分组的成功率
	AvgTps       float64       `json:"avg_tps"`        // 该分组的平均 TPS
	Series       []BucketPoint `json:"series"`         // 时间序列数据点
}

// QueryResult 表示性能指标查询的完整结果
type QueryResult struct {
	ModelName    string        `json:"model_name"`    // 查询的模型名称
	SeriesSchema string        `json:"series_schema"` // 序列数据的 Schema 标识（客户端缓存用）
	Groups       []GroupResult `json:"groups"`        // 按分组聚合的结果列表
}

// ModelSummary 表示单个模型的性能摘要
// 用于性能概览页面展示所有模型的汇总指标
type ModelSummary struct {
	ModelName          string    `json:"model_name"`                     // 模型名称
	AvgLatencyMs       int64     `json:"avg_latency_ms"`                 // 平均延迟
	SuccessRate        float64   `json:"success_rate"`                   // 成功率
	AvgTps             float64   `json:"avg_tps"`                        // 平均 TPS
	RecentSuccessRates []float64 `json:"recent_success_rates,omitempty"` // 最近时间桶成功率，供前端展示短趋势
	RequestCount       int64     `json:"-"`                              // 请求总数（不序列化到 JSON，仅内部排序用）
}

// SummaryAllResult 表示所有模型摘要的查询结果
type SummaryAllResult struct {
	Models []ModelSummary `json:"models"` // 模型摘要列表（按请求量降序排列）
}

// bucketKey 表示时间桶的唯一标识
// 由模型名称、用户分组和时间桶起始时间戳组成
type bucketKey struct {
	model    string // 模型名称
	group    string // 用户分组
	bucketTs int64  // 时间桶起始时间戳（Unix 秒）
}

// counters 表示聚合的计数器数据
// 用于在内存和数据库之间传递聚合结果
type counters struct {
	requestCount   int64 // 请求总数
	successCount   int64 // 成功请求数
	totalLatencyMs int64 // 延迟总和（毫秒）
	ttftSumMs      int64 // TTFT 总和（毫秒）
	ttftCount      int64 // 有 TTFT 数据的请求数
	outputTokens   int64 // 输出 Token 总数
	generationMs   int64 // 生成阶段耗时总和（毫秒）
}

// atomicBucket 表示原子化的时间桶计数器
// 使用 atomic.Int64 保证高并发下的数据一致性
// 适用于写多读少的场景（大量请求并发写入，定期批量读取）
type atomicBucket struct {
	requestCount   atomic.Int64 // 请求总数
	successCount   atomic.Int64 // 成功请求数
	totalLatencyMs atomic.Int64 // 延迟总和（毫秒）
	ttftSumMs      atomic.Int64 // TTFT 总和（毫秒）
	ttftCount      atomic.Int64 // 有 TTFT 数据的请求数
	outputTokens   atomic.Int64 // 输出 Token 总数
	generationMs   atomic.Int64 // 生成阶段耗时总和（毫秒）
}

// add 向时间桶中添加一次采样数据
// 使用原子操作保证并发安全
func (b *atomicBucket) add(sample Sample) {
	b.requestCount.Add(1)
	if sample.Success {
		b.successCount.Add(1)
	}
	if sample.LatencyMs > 0 {
		b.totalLatencyMs.Add(sample.LatencyMs)
	}
	if sample.HasTtft && sample.TtftMs >= 0 {
		b.ttftSumMs.Add(sample.TtftMs)
		b.ttftCount.Add(1)
	}
	if sample.OutputTokens > 0 && sample.GenerationMs > 0 {
		b.outputTokens.Add(sample.OutputTokens)
		b.generationMs.Add(sample.GenerationMs)
	}
}

// snapshot 获取当前计数器的快照（非破坏性读取）
// 返回当前所有计数器的值，不影响后续写入
func (b *atomicBucket) snapshot() counters {
	return counters{
		requestCount:   b.requestCount.Load(),
		successCount:   b.successCount.Load(),
		totalLatencyMs: b.totalLatencyMs.Load(),
		ttftSumMs:      b.ttftSumMs.Load(),
		ttftCount:      b.ttftCount.Load(),
		outputTokens:   b.outputTokens.Load(),
		generationMs:   b.generationMs.Load(),
	}
}

// drain 排空并重置所有计数器（破坏性读取）
// 使用 Swap(0) 原子地读取当前值并重置为 0
// 用于将内存中的累积数据刷新到数据库后清零
func (b *atomicBucket) drain() counters {
	return counters{
		requestCount:   b.requestCount.Swap(0),
		successCount:   b.successCount.Swap(0),
		totalLatencyMs: b.totalLatencyMs.Swap(0),
		ttftSumMs:      b.ttftSumMs.Swap(0),
		ttftCount:      b.ttftCount.Swap(0),
		outputTokens:   b.outputTokens.Swap(0),
		generationMs:   b.generationMs.Swap(0),
	}
}

// addCounters 将一组计数器的值累加到当前桶中
// 用于刷新失败时恢复数据（将排空的数据重新加回）
// 只累加非零值，避免不必要的原子操作
func (b *atomicBucket) addCounters(c counters) {
	if c.requestCount != 0 {
		b.requestCount.Add(c.requestCount)
	}
	if c.successCount != 0 {
		b.successCount.Add(c.successCount)
	}
	if c.totalLatencyMs != 0 {
		b.totalLatencyMs.Add(c.totalLatencyMs)
	}
	if c.ttftSumMs != 0 {
		b.ttftSumMs.Add(c.ttftSumMs)
	}
	if c.ttftCount != 0 {
		b.ttftCount.Add(c.ttftCount)
	}
	if c.outputTokens != 0 {
		b.outputTokens.Add(c.outputTokens)
	}
	if c.generationMs != 0 {
		b.generationMs.Add(c.generationMs)
	}
}
