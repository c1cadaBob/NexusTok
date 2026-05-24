// 本文件 (rankings.go) 提供模型和供应商的排行榜服务。
// 基于用户使用量（Token 消耗量）数据，生成多维度的排行榜快照，
// 包括：模型排名、供应商排名、排名变动（上升/下降最多）、
// 模型历史趋势和供应商份额历史趋势。
// 支持多种时间周期（今日、本周、本月、本年、全部），并使用内存缓存提升性能。
package service

import (
	"fmt"    // 格式化输出
	"math"   // 数学运算（四舍五入）
	"sort"   // 排序算法
	"sync"   // 互斥锁，保护缓存并发访问
	"time"   // 时间处理

	"github.com/c1cada/NexusTok/model" // 数据模型层，提供数据库查询接口
)

// 排行榜相关常量配置
const (
	rankingCacheTTL         = 5 * time.Minute // 排行榜缓存有效期：5 分钟
	rankingLeaderboardLimit = 20              // 模型排行榜最多展示条数
	rankingHistoryLimit     = 10              // 历史趋势图中单独展示的模型数量，超出的归入 "Others"
	rankingVendorLimit      = 5               // 供应商份额历史趋势中单独展示的供应商数量
	rankingMoverLimit       = 6               // 排名变动（上升/下降）最多展示条数
	rankingOthersLabel      = "Others"        // 聚合其他项的标签名称
	rankingUnknownVendor    = "Unknown"       // 未知供应商的默认标签
)

// RankingsResponse 排行榜完整响应结构体，包含所有排行榜数据。
// 作为 API 接口的返回值，序列化为 JSON。
type RankingsResponse struct {
	Models             []RankedModel      `json:"models"`              // 模型排行榜
	Vendors            []RankedVendor     `json:"vendors"`             // 供应商排行榜
	TopMovers          []RankingMover     `json:"top_movers"`          // 排名上升最多的模型
	TopDroppers        []RankingMover     `json:"top_droppers"`        // 排名下降最多的模型
	ModelsHistory      ModelHistorySeries `json:"models_history"`      // 模型使用量历史趋势
	VendorShareHistory VendorShareSeries  `json:"vendor_share_history"` // 供应商份额历史趋势
}

// RankedModel 排行榜中的单个模型条目。
type RankedModel struct {
	Rank         int     `json:"rank"`                    // 当前排名（从 1 开始）
	PreviousRank *int    `json:"previous_rank,omitempty"` // 上一周期的排名（指针类型，无历史数据时为 nil）
	ModelName    string  `json:"model_name"`              // 模型名称
	Vendor       string  `json:"vendor"`                  // 供应商名称
	VendorIcon   string  `json:"vendor_icon,omitempty"`   // 供应商图标 URL
	Category     string  `json:"category"`                // 分类（目前固定为 "all"）
	TotalTokens  int64   `json:"total_tokens"`            // 总 Token 消耗量
	Share        float64 `json:"share"`                   // 市场份额（0~1 之间）
	GrowthPct    float64 `json:"growth_pct"`              // 相比上一周期的增长百分比
}

// RankedVendor 排行榜中的单个供应商条目。
type RankedVendor struct {
	Rank        int     `json:"rank"`                    // 排名
	Vendor      string  `json:"vendor"`                  // 供应商名称
	VendorIcon  string  `json:"vendor_icon,omitempty"`   // 供应商图标 URL
	TotalTokens int64   `json:"total_tokens"`            // 总 Token 消耗量
	Share       float64 `json:"share"`                   // 市场份额
	GrowthPct   float64 `json:"growth_pct"`              // 增长百分比
	ModelsCount int     `json:"models_count"`            // 该供应商下的模型数量
	TopModel    string  `json:"top_model"`               // 该供应商下使用量最高的模型
}

// RankingMover 排名变动条目，用于展示排名上升或下降最多的模型。
type RankingMover struct {
	ModelName   string  `json:"model_name"`   // 模型名称
	Vendor      string  `json:"vendor"`       // 供应商名称
	VendorIcon  string  `json:"vendor_icon,omitempty"` // 供应商图标
	RankDelta   int     `json:"rank_delta"`   // 排名变化量（正数表示上升，负数表示下降）
	CurrentRank int     `json:"current_rank"` // 当前排名
	GrowthPct   float64 `json:"growth_pct"`   // 增长百分比
}

// --- 模型历史趋势相关结构体 ---

// ModelHistoryPoint 模型历史趋势中的单个数据点。
type ModelHistoryPoint struct {
	Ts     string `json:"ts"`     // 时间戳（RFC3339 格式）
	Label  string `json:"label"`  // 显示标签（如 "Jan 2" 或 "15:04"）
	Model  string `json:"model"`  // 模型名称
	Vendor string `json:"vendor"` // 供应商名称
	Tokens int64  `json:"tokens"` // 该时间段的 Token 消耗量
}

// ModelHistoryModel 模型历史趋势中的模型摘要信息。
type ModelHistoryModel struct {
	Name   string `json:"name"`   // 模型名称
	Vendor string `json:"vendor"` // 供应商名称
	Total  int64  `json:"total"`  // 整个周期内的总 Token 消耗量
}

// ModelHistorySeries 模型历史趋势的完整数据系列。
type ModelHistorySeries struct {
	Points  []ModelHistoryPoint `json:"points"`  // 所有数据点
	Models  []ModelHistoryModel `json:"models"`  // 参与展示的模型列表
	Buckets int                 `json:"buckets"` // 时间桶数量
}

// --- 供应商份额历史趋势相关结构体 ---

// VendorSharePoint 供应商份额历史趋势中的单个数据点。
type VendorSharePoint struct {
	Ts     string  `json:"ts"`     // 时间戳
	Label  string  `json:"label"`  // 显示标签
	Vendor string  `json:"vendor"` // 供应商名称
	Share  float64 `json:"share"`  // 该时间段的份额
	Tokens int64   `json:"tokens"` // 该时间段的 Token 消耗量
}

// VendorShareVendor 供应商份额历史趋势中的供应商摘要信息。
type VendorShareVendor struct {
	Name  string  `json:"name"`  // 供应商名称
	Total int64   `json:"total"` // 总 Token 消耗量
	Share float64 `json:"share"` // 整体份额
}

// VendorShareSeries 供应商份额历史趋势的完整数据系列。
type VendorShareSeries struct {
	Points  []VendorSharePoint  `json:"points"`  // 所有数据点
	Vendors []VendorShareVendor `json:"vendors"` // 参与展示的供应商列表
	Buckets int                 `json:"buckets"` // 时间桶数量
}

// --- 内部辅助结构体 ---

// rankingPeriodConfig 排行榜时间周期配置。
// 定义不同时间周期（今日、本周等）的参数。
type rankingPeriodConfig struct {
	id          string        // 周期标识（如 "week", "today"）
	duration    time.Duration // 时间跨度（0 表示"全部"）
	bucketSize  int64         // 时间桶大小（秒），用于历史趋势聚合
	labelLayout string        // 时间标签的 Go 格式化布局
	hasPrevious bool          // 是否有上一周期数据（用于计算排名变化和增长率）
}

// rankingCacheItem 排行榜缓存条目。
type rankingCacheItem struct {
	expiresAt time.Time         // 缓存过期时间
	data      *RankingsResponse // 缓存的排行榜数据
}

// rankingModelMeta 模型的元数据信息（供应商名称和图标）。
type rankingModelMeta struct {
	vendor     string // 供应商名称
	vendorIcon string // 供应商图标 URL
}

// vendorAggregate 供应商聚合数据，用于构建供应商排行榜。
type vendorAggregate struct {
	name           string              // 供应商名称
	icon           string              // 供应商图标
	totalTokens    int64               // 当前周期总 Token 消耗量
	previousTokens int64               // 上一周期总 Token 消耗量
	models         map[string]struct{} // 该供应商下的模型集合
	topModel       string              // 使用量最高的模型名称
	topModelTokens int64               // 使用量最高的模型的 Token 数
}

// 排行榜缓存（全局单例）
var (
	rankingCacheMu sync.Mutex                         // 缓存互斥锁
	rankingCache   = map[string]rankingCacheItem{}     // 按周期 ID 缓存的排行榜数据
)

// GetRankingsSnapshot 获取排行榜快照数据（带缓存）。
// 该函数是排行榜服务的主入口，先检查缓存是否有效，
// 缓存命中则直接返回，否则重新构建排行榜数据并缓存。
// 参数:
//   - period: 时间周期标识（"today", "week", "month", "year", "all"，空字符串默认为 "week"）
// 返回值:
//   - *RankingsResponse: 排行榜完整数据
//   - error: 构建过程中的错误
func GetRankingsSnapshot(period string) (*RankingsResponse, error) {
	// 解析时间周期配置
	config, err := rankingConfig(period)
	if err != nil {
		return nil, err
	}

	// 检查缓存是否有效
	now := time.Now()
	rankingCacheMu.Lock()
	if item, ok := rankingCache[config.id]; ok && now.Before(item.expiresAt) {
		rankingCacheMu.Unlock()
		return item.data, nil
	}
	rankingCacheMu.Unlock()

	// 缓存未命中或已过期，重新构建排行榜数据
	data, err := buildRankingsSnapshot(config, now)
	if err != nil {
		return nil, err
	}

	// 更新缓存
	rankingCacheMu.Lock()
	rankingCache[config.id] = rankingCacheItem{
		expiresAt: now.Add(rankingCacheTTL),
		data:      data,
	}
	rankingCacheMu.Unlock()

	return data, nil
}

// rankingConfig 根据周期标识返回对应的排行榜配置。
// 支持的周期：today（今日）、week（本周，默认）、month（本月）、year（本年）、all（全部）。
// 参数:
//   - period: 时间周期标识字符串
// 返回值:
//   - rankingPeriodConfig: 周期配置
//   - error: 无效周期时返回错误
func rankingConfig(period string) (rankingPeriodConfig, error) {
	switch period {
	case "", "week":
		return rankingPeriodConfig{id: "week", duration: 7 * 24 * time.Hour, bucketSize: 24 * 3600, labelLayout: "Jan 2", hasPrevious: true}, nil
	case "today":
		return rankingPeriodConfig{id: "today", duration: 24 * time.Hour, bucketSize: 3600, labelLayout: "15:04", hasPrevious: true}, nil
	case "month":
		return rankingPeriodConfig{id: "month", duration: 30 * 24 * time.Hour, bucketSize: 24 * 3600, labelLayout: "Jan 2", hasPrevious: true}, nil
	case "year":
		return rankingPeriodConfig{id: "year", duration: 365 * 24 * time.Hour, bucketSize: 7 * 24 * 3600, labelLayout: "Jan 2", hasPrevious: true}, nil
	case "all":
		return rankingPeriodConfig{id: "all", bucketSize: 30 * 24 * 3600, labelLayout: "Jan 2006"}, nil
	default:
		return rankingPeriodConfig{}, fmt.Errorf("invalid ranking period: %s", period)
	}
}

// buildRankingsSnapshot 构建排行榜快照数据的核心函数。
// 处理流程：
// 1. 计算当前周期的时间范围，查询数据库获取当前周期的 Token 消耗总量和分桶数据
// 2. 如有上一周期，查询上一周期的总量数据用于计算排名变化和增长率
// 3. 构建模型元数据映射（供应商名称、图标等）
// 4. 分别构建模型排行榜、供应商排行榜、模型历史趋势、供应商份额历史、排名变动
// 5. 组装并返回完整的排行榜响应
// 参数:
//   - config: 时间周期配置
//   - now: 当前时间
// 返回值:
//   - *RankingsResponse: 排行榜完整数据
//   - error: 数据库查询或构建过程中的错误
func buildRankingsSnapshot(config rankingPeriodConfig, now time.Time) (*RankingsResponse, error) {
	// 计算当前周期的时间范围
	startTime, endTime := rankingTimeRange(config, now)
	// 查询当前周期的模型 Token 消耗总量（按消耗量降序排列）
	currentTotals, err := model.GetRankingQuotaTotals(startTime, endTime)
	if err != nil {
		return nil, err
	}
	// 查询当前周期的分桶数据（按时间桶聚合），用于绘制历史趋势图
	currentBuckets, err := model.GetRankingQuotaBuckets(startTime, endTime, config.bucketSize)
	if err != nil {
		return nil, err
	}

	// 查询上一周期的总量数据（用于计算排名变化和增长率）
	var previousTotals []model.RankingQuotaTotal
	if config.hasPrevious {
		previousStart, previousEnd := previousRankingTimeRange(config, startTime)
		previousTotals, err = model.GetRankingQuotaTotals(previousStart, previousEnd)
		if err != nil {
			return nil, err
		}
	}

	// 构建辅助数据结构
	meta := buildRankingModelMeta()                          // 模型 -> 供应商元数据映射
	totalTokens := sumRankingTokens(currentTotals)           // 当前周期总 Token 数
	previousRankByModel := rankingRankMap(previousTotals)    // 上一周期模型排名映射
	previousTokensByModel := rankingTokenMap(previousTotals) // 上一周期模型 Token 数映射

	// 构建各个排行榜数据
	rankedModels := buildRankedModels(currentTotals, totalTokens, previousRankByModel, previousTokensByModel, meta, config.hasPrevious)
	vendors := buildRankedVendors(currentTotals, previousTotals, totalTokens, meta, config.hasPrevious)
	modelHistory := buildModelHistory(currentBuckets, currentTotals, meta, config)
	vendorHistory := buildVendorShareHistory(currentBuckets, vendors, totalTokens, meta, config)
	movers, droppers := buildRankingMovers(rankedModels)

	// 组装最终响应，模型排行榜限制最多展示 rankingLeaderboardLimit 条
	return &RankingsResponse{
		Models:             limitRankedModels(rankedModels, rankingLeaderboardLimit),
		Vendors:            vendors,
		TopMovers:          movers,
		TopDroppers:        droppers,
		ModelsHistory:      modelHistory,
		VendorShareHistory: vendorHistory,
	}, nil
}

// rankingTimeRange 根据周期配置计算查询的时间范围。
// 对于 "all" 周期（duration <= 0），起始时间为 0（即不限制）。
// 参数:
//   - config: 时间周期配置
//   - now: 当前时间
// 返回值:
//   - int64: 起始时间（Unix 时间戳，秒）
//   - int64: 结束时间（Unix 时间戳，秒）
func rankingTimeRange(config rankingPeriodConfig, now time.Time) (int64, int64) {
	endTime := now.Unix()
	if config.duration <= 0 {
		return 0, endTime // "all" 周期不限制起始时间
	}
	return now.Add(-config.duration).Unix(), endTime
}

// previousRankingTimeRange 计算上一周期的时间范围。
// 上一周期的结束时间为当前周期起始时间减 1 秒，
// 起始时间为当前周期起始时间再往前推一个 duration。
// 参数:
//   - config: 时间周期配置
//   - currentStart: 当前周期的起始时间戳
// 返回值:
//   - int64: 上一周期起始时间戳
//   - int64: 上一周期结束时间戳
func previousRankingTimeRange(config rankingPeriodConfig, currentStart int64) (int64, int64) {
	previousEnd := currentStart - 1
	previousStart := time.Unix(currentStart, 0).Add(-config.duration).Unix()
	return previousStart, previousEnd
}

// buildRankingModelMeta 构建模型名称到供应商元数据的映射表。
// 数据来源：先加载所有供应商信息，再遍历定价记录关联供应商。
// 如果定价记录没有关联供应商但有 OwnerBy 字段，则使用 OwnerBy 作为供应商名称。
// 返回值:
//   - map[string]rankingModelMeta: 模型名称 -> 供应商元数据的映射
func buildRankingModelMeta() map[string]rankingModelMeta {
	// 构建供应商 ID -> 供应商信息的映射
	vendorByID := make(map[int]model.PricingVendor)
	for _, vendor := range model.GetVendors() {
		vendorByID[vendor.ID] = vendor
	}

	// 遍历定价记录，关联模型与供应商
	meta := make(map[string]rankingModelMeta)
	for _, pricing := range model.GetPricing() {
		item := rankingModelMeta{vendor: rankingUnknownVendor}
		if vendor, ok := vendorByID[pricing.VendorID]; ok {
			item.vendor = vendor.Name
			item.vendorIcon = vendor.Icon
		} else if pricing.OwnerBy != "" {
			item.vendor = pricing.OwnerBy
		}
		meta[pricing.ModelName] = item
	}
	return meta
}

// modelMeta 从元数据映射中获取模型的供应商信息。
// 如果模型不在映射中或供应商为空，返回默认的 "Unknown" 供应商。
// 参数:
//   - modelName: 模型名称
//   - meta: 模型元数据映射表
// 返回值:
//   - rankingModelMeta: 模型的供应商元数据
func modelMeta(modelName string, meta map[string]rankingModelMeta) rankingModelMeta {
	if item, ok := meta[modelName]; ok && item.vendor != "" {
		return item
	}
	return rankingModelMeta{vendor: rankingUnknownVendor}
}

// buildRankedModels 构建模型排行榜数据。
// 遍历当前周期的 Token 消耗总量数据，为每个模型计算排名、市场份额和增长率。
// 参数:
//   - totals: 当前周期的模型 Token 消耗总量列表（已按消耗量降序排列）
//   - totalTokens: 当前周期所有模型的总 Token 数
//   - previousRanks: 上一周期的模型排名映射（模型名 -> 排名）
//   - previousTokens: 上一周期的模型 Token 数映射
//   - meta: 模型元数据映射
//   - showGrowth: 是否计算增长率
// 返回值:
//   - []RankedModel: 模型排行榜数据列表
func buildRankedModels(totals []model.RankingQuotaTotal, totalTokens int64, previousRanks map[string]int, previousTokens map[string]int64, meta map[string]rankingModelMeta, showGrowth bool) []RankedModel {
	rows := make([]RankedModel, 0, len(totals))
	for idx, item := range totals {
		modelMeta := modelMeta(item.ModelName, meta)
		// 获取上一周期的排名（如有）
		var previousRank *int
		if rank, ok := previousRanks[item.ModelName]; ok {
			rankCopy := rank
			previousRank = &rankCopy
		}
		// 计算增长率
		growth := 0.0
		if showGrowth {
			growth = rankingGrowthPct(item.TotalTokens, previousTokens[item.ModelName])
		}
		rows = append(rows, RankedModel{
			Rank:         idx + 1,
			PreviousRank: previousRank,
			ModelName:    item.ModelName,
			Vendor:       modelMeta.vendor,
			VendorIcon:   modelMeta.vendorIcon,
			Category:     "all",
			TotalTokens:  item.TotalTokens,
			Share:        rankingShare(item.TotalTokens, totalTokens),
			GrowthPct:    growth,
		})
	}
	return rows
}

// buildRankedVendors 构建供应商排行榜数据。
// 将模型级别的数据聚合到供应商级别，计算每个供应商的总消耗量、
// 市场份额、增长率、模型数量和最热门模型。
// 参数:
//   - currentTotals: 当前周期的模型 Token 消耗总量
//   - previousTotals: 上一周期的模型 Token 消耗总量
//   - totalTokens: 当前周期总 Token 数
//   - meta: 模型元数据映射
//   - showGrowth: 是否计算增长率
// 返回值:
//   - []RankedVendor: 供应商排行榜数据列表（已按消耗量降序排列并编号）
func buildRankedVendors(currentTotals []model.RankingQuotaTotal, previousTotals []model.RankingQuotaTotal, totalTokens int64, meta map[string]rankingModelMeta, showGrowth bool) []RankedVendor {
	// 按供应商聚合数据
	aggregates := make(map[string]*vendorAggregate)
	for _, item := range currentTotals {
		modelMeta := modelMeta(item.ModelName, meta)
		agg := ensureVendorAggregate(aggregates, modelMeta)
		agg.totalTokens += item.TotalTokens
		agg.models[item.ModelName] = struct{}{}
		// 记录使用量最高的模型
		if item.TotalTokens > agg.topModelTokens {
			agg.topModel = item.ModelName
			agg.topModelTokens = item.TotalTokens
		}
	}
	// 聚合上一周期数据（用于计算增长率）
	for _, item := range previousTotals {
		modelMeta := modelMeta(item.ModelName, meta)
		agg := ensureVendorAggregate(aggregates, modelMeta)
		agg.previousTokens += item.TotalTokens
	}

	// 构建排行榜行数据
	rows := make([]RankedVendor, 0, len(aggregates))
	for _, agg := range aggregates {
		if agg.totalTokens <= 0 {
			continue
		}
		growth := 0.0
		if showGrowth {
			growth = rankingGrowthPct(agg.totalTokens, agg.previousTokens)
		}
		rows = append(rows, RankedVendor{
			Vendor:      agg.name,
			VendorIcon:  agg.icon,
			TotalTokens: agg.totalTokens,
			Share:       rankingShare(agg.totalTokens, totalTokens),
			GrowthPct:   growth,
			ModelsCount: len(agg.models),
			TopModel:    agg.topModel,
		})
	}
	// 按 Token 消耗量降序排序（相同消耗量按供应商名称字母序）
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalTokens == rows[j].TotalTokens {
			return rows[i].Vendor < rows[j].Vendor
		}
		return rows[i].TotalTokens > rows[j].TotalTokens
	})
	// 分配排名
	for idx := range rows {
		rows[idx].Rank = idx + 1
	}
	return rows
}

// ensureVendorAggregate 确保供应商聚合数据存在于映射中。
// 如果不存在则创建新的聚合数据；如果已存在但缺少图标则补充。
// 参数:
//   - aggregates: 供应商聚合数据映射
//   - meta: 模型的供应商元数据
// 返回值:
//   - *vendorAggregate: 供应商聚合数据指针
func ensureVendorAggregate(aggregates map[string]*vendorAggregate, meta rankingModelMeta) *vendorAggregate {
	name := meta.vendor
	if name == "" {
		name = rankingUnknownVendor
	}
	agg, ok := aggregates[name]
	if !ok {
		agg = &vendorAggregate{
			name:   name,
			icon:   meta.vendorIcon,
			models: make(map[string]struct{}),
		}
		aggregates[name] = agg
	}
	// 补充图标（可能在后续模型数据中出现）
	if agg.icon == "" && meta.vendorIcon != "" {
		agg.icon = meta.vendorIcon
	}
	return agg
}

// buildModelHistory 构建模型使用量历史趋势数据。
// 将分桶数据按模型聚合，展示 Top N 模型的时间序列数据，
// 超出的模型归入 "Others" 聚合展示。
// 参数:
//   - buckets: 按时间桶聚合的 Token 消耗数据
//   - totals: 模型 Token 消耗总量（用于确定 Top N 模型）
//   - meta: 模型元数据映射
//   - config: 时间周期配置（决定标签格式）
// 返回值:
//   - ModelHistorySeries: 模型历史趋势数据系列
func buildModelHistory(buckets []model.RankingQuotaBucket, totals []model.RankingQuotaTotal, meta map[string]rankingModelMeta, config rankingPeriodConfig) ModelHistorySeries {
	// 确定 Top N 模型，其余归入 "Others"
	topModels := make(map[string]struct{})
	models := make([]ModelHistoryModel, 0, minInt(len(totals), rankingHistoryLimit)+1)
	otherTotal := int64(0)
	for idx, item := range totals {
		if idx < rankingHistoryLimit {
			topModels[item.ModelName] = struct{}{}
			modelMeta := modelMeta(item.ModelName, meta)
			models = append(models, ModelHistoryModel{Name: item.ModelName, Vendor: modelMeta.vendor, Total: item.TotalTokens})
			continue
		}
		otherTotal += item.TotalTokens
	}
	// 如果有不属于 Top N 的模型，添加 "Others" 聚合项
	if otherTotal > 0 {
		models = append(models, ModelHistoryModel{Name: rankingOthersLabel, Vendor: "Various", Total: otherTotal})
	}

	// 按时间桶和模型聚合 Token 数据
	bucketSet := make(map[int64]struct{})
	tokensByBucketAndModel := make(map[int64]map[string]int64)
	for _, item := range buckets {
		modelName := item.ModelName
		// 非 Top N 模型归入 "Others"
		if _, ok := topModels[modelName]; !ok {
			modelName = rankingOthersLabel
		}
		bucketSet[item.Bucket] = struct{}{}
		if _, ok := tokensByBucketAndModel[item.Bucket]; !ok {
			tokensByBucketAndModel[item.Bucket] = make(map[string]int64)
		}
		tokensByBucketAndModel[item.Bucket][modelName] += item.Tokens
	}

	// 按时间顺序排列时间桶，生成数据点
	sortedBuckets := sortedRankingBuckets(bucketSet)
	points := make([]ModelHistoryPoint, 0, len(sortedBuckets)*len(models))
	for _, bucket := range sortedBuckets {
		for _, historyModel := range models {
			tokens := tokensByBucketAndModel[bucket][historyModel.Name]
			if tokens <= 0 {
				continue
			}
			points = append(points, ModelHistoryPoint{
				Ts:     rankingBucketTs(bucket),
				Label:  rankingBucketLabel(bucket, config),
				Model:  historyModel.Name,
				Vendor: historyModel.Vendor,
				Tokens: tokens,
			})
		}
	}

	return ModelHistorySeries{
		Points:  points,
		Models:  models,
		Buckets: len(sortedBuckets),
	}
}

// buildVendorShareHistory 构建供应商份额历史趋势数据。
// 将分桶数据按供应商聚合，展示 Top N 供应商在各时间段的份额变化。
// 参数:
//   - buckets: 按时间桶聚合的 Token 消耗数据
//   - vendors: 供应商排行榜（用于确定 Top N 供应商）
//   - totalTokens: 总 Token 数
//   - meta: 模型元数据映射
//   - config: 时间周期配置
// 返回值:
//   - VendorShareSeries: 供应商份额历史趋势数据系列
func buildVendorShareHistory(buckets []model.RankingQuotaBucket, vendors []RankedVendor, totalTokens int64, meta map[string]rankingModelMeta, config rankingPeriodConfig) VendorShareSeries {
	// 确定 Top N 供应商，其余归入 "Others"
	topVendors := make(map[string]struct{})
	vendorRows := make([]VendorShareVendor, 0, minInt(len(vendors), rankingVendorLimit)+1)
	otherTotal := int64(0)
	for idx, vendor := range vendors {
		if idx < rankingVendorLimit {
			topVendors[vendor.Vendor] = struct{}{}
			vendorRows = append(vendorRows, VendorShareVendor{Name: vendor.Vendor, Total: vendor.TotalTokens, Share: vendor.Share})
			continue
		}
		otherTotal += vendor.TotalTokens
	}
	if otherTotal > 0 {
		vendorRows = append(vendorRows, VendorShareVendor{Name: rankingOthersLabel, Total: otherTotal, Share: rankingShare(otherTotal, totalTokens)})
	}

	// 按时间桶和供应商聚合数据
	bucketSet := make(map[int64]struct{})
	tokensByBucketAndVendor := make(map[int64]map[string]int64)
	totalsByBucket := make(map[int64]int64) // 每个时间桶的总 Token 数，用于计算份额
	for _, item := range buckets {
		modelMeta := modelMeta(item.ModelName, meta)
		vendorName := modelMeta.vendor
		// 非 Top N 供应商归入 "Others"
		if _, ok := topVendors[vendorName]; !ok {
			vendorName = rankingOthersLabel
		}
		bucketSet[item.Bucket] = struct{}{}
		if _, ok := tokensByBucketAndVendor[item.Bucket]; !ok {
			tokensByBucketAndVendor[item.Bucket] = make(map[string]int64)
		}
		tokensByBucketAndVendor[item.Bucket][vendorName] += item.Tokens
		totalsByBucket[item.Bucket] += item.Tokens
	}

	// 按时间顺序排列时间桶，生成份额数据点
	sortedBuckets := sortedRankingBuckets(bucketSet)
	points := make([]VendorSharePoint, 0, len(sortedBuckets)*len(vendorRows))
	for _, bucket := range sortedBuckets {
		for _, vendor := range vendorRows {
			tokens := tokensByBucketAndVendor[bucket][vendor.Name]
			if tokens <= 0 {
				continue
			}
			points = append(points, VendorSharePoint{
				Ts:     rankingBucketTs(bucket),
				Label:  rankingBucketLabel(bucket, config),
				Vendor: vendor.Name,
				Share:  rankingShare(tokens, totalsByBucket[bucket]),
				Tokens: tokens,
			})
		}
	}

	return VendorShareSeries{
		Points:  points,
		Vendors: vendorRows,
		Buckets: len(sortedBuckets),
	}
}

// buildRankingMovers 构建排名变动数据（上升最多和下降最多的模型）。
// 遍历模型排行榜，根据当前排名与上一周期排名的差异，
// 分别收集上升和下降的模型，按排名变化量排序后返回 Top N。
// 参数:
//   - models: 模型排行榜数据
// 返回值:
//   - []RankingMover: 排名上升最多的模型列表
//   - []RankingMover: 排名下降最多的模型列表
func buildRankingMovers(models []RankedModel) ([]RankingMover, []RankingMover) {
	movers := make([]RankingMover, 0)    // 排名上升的模型
	droppers := make([]RankingMover, 0)  // 排名下降的模型
	for _, item := range models {
		// 没有上一周期排名数据的模型跳过
		if item.PreviousRank == nil {
			continue
		}
		// 计算排名变化量（正数表示上升，负数表示下降）
		delta := *item.PreviousRank - item.Rank
		if delta == 0 {
			continue
		}
		row := RankingMover{
			ModelName:   item.ModelName,
			Vendor:      item.Vendor,
			VendorIcon:  item.VendorIcon,
			RankDelta:   delta,
			CurrentRank: item.Rank,
			GrowthPct:   item.GrowthPct,
		}
		if delta > 0 {
			movers = append(movers, row)
		} else {
			droppers = append(droppers, row)
		}
	}
	// 上升的按排名变化量降序排列（变化最大的排前面）
	sort.Slice(movers, func(i, j int) bool {
		if movers[i].RankDelta == movers[j].RankDelta {
			return movers[i].GrowthPct > movers[j].GrowthPct
		}
		return movers[i].RankDelta > movers[j].RankDelta
	})
	// 下降的按排名变化量升序排列（下降最多的排前面）
	sort.Slice(droppers, func(i, j int) bool {
		if droppers[i].RankDelta == droppers[j].RankDelta {
			return droppers[i].GrowthPct < droppers[j].GrowthPct
		}
		return droppers[i].RankDelta < droppers[j].RankDelta
	})
	return limitRankingMovers(movers, rankingMoverLimit), limitRankingMovers(droppers, rankingMoverLimit)
}

// sortedRankingBuckets 将时间桶集合转换为按时间顺序排列的切片。
// 参数:
//   - bucketSet: 时间桶的集合（去重）
// 返回值:
//   - []int64: 按升序排列的时间桶切片
func sortedRankingBuckets(bucketSet map[int64]struct{}) []int64 {
	buckets := make([]int64, 0, len(bucketSet))
	for bucket := range bucketSet {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i] < buckets[j]
	})
	return buckets
}

// rankingBucketTs 将时间桶的 Unix 时间戳转换为 RFC3339 格式字符串。
// 参数:
//   - bucket: Unix 时间戳（秒）
// 返回值:
//   - string: RFC3339 格式的时间字符串
func rankingBucketTs(bucket int64) string {
	return time.Unix(bucket, 0).UTC().Format(time.RFC3339)
}

// rankingBucketLabel 根据周期配置生成时间桶的显示标签。
// 参数:
//   - bucket: Unix 时间戳（秒）
//   - config: 时间周期配置（决定标签格式）
// 返回值:
//   - string: 格式化后的时间标签（如 "Jan 2" 或 "15:04"）
func rankingBucketLabel(bucket int64, config rankingPeriodConfig) string {
	return time.Unix(bucket, 0).Format(config.labelLayout)
}

// rankingRankMap 构建模型排名映射表。
// 根据列表中的位置（索引 + 1）确定排名。
// 参数:
//   - totals: 模型 Token 消耗总量列表（已排序）
// 返回值:
//   - map[string]int: 模型名称 -> 排名的映射
func rankingRankMap(totals []model.RankingQuotaTotal) map[string]int {
	ranks := make(map[string]int, len(totals))
	for idx, item := range totals {
		ranks[item.ModelName] = idx + 1
	}
	return ranks
}

// rankingTokenMap 构建模型 Token 数映射表。
// 参数:
//   - totals: 模型 Token 消耗总量列表
// 返回值:
//   - map[string]int64: 模型名称 -> Token 数的映射
func rankingTokenMap(totals []model.RankingQuotaTotal) map[string]int64 {
	tokens := make(map[string]int64, len(totals))
	for _, item := range totals {
		tokens[item.ModelName] = item.TotalTokens
	}
	return tokens
}

// sumRankingTokens 计算所有模型的 Token 消耗总量。
// 参数:
//   - totals: 模型 Token 消耗总量列表
// 返回值:
//   - int64: 总 Token 数
func sumRankingTokens(totals []model.RankingQuotaTotal) int64 {
	total := int64(0)
	for _, item := range totals {
		total += item.TotalTokens
	}
	return total
}

// rankingShare 计算市场份额（值占总量的比例）。
// 结果保留 4 位小数。总量或值为 0 时返回 0。
// 参数:
//   - value: 当前值
//   - total: 总量
// 返回值:
//   - float64: 市场份额（0~1 之间）
func rankingShare(value int64, total int64) float64 {
	if total <= 0 || value <= 0 {
		return 0
	}
	return roundRankingFloat(float64(value) / float64(total))
}

// rankingGrowthPct 计算增长率百分比。
// 公式：(当前值 - 上期值) / 上期值 * 100
// 上期值为 0 时：当前值 > 0 则返回 100%，否则返回 0%。
// 参数:
//   - current: 当前周期的值
//   - previous: 上一周期的值
// 返回值:
//   - float64: 增长率百分比（保留 4 位小数）
func rankingGrowthPct(current int64, previous int64) float64 {
	if previous <= 0 {
		if current > 0 {
			return 100
		}
		return 0
	}
	return roundRankingFloat((float64(current-previous) / float64(previous)) * 100)
}

// roundRankingFloat 将浮点数四舍五入到 4 位小数。
// 参数:
//   - value: 原始浮点数
// 返回值:
//   - float64: 四舍五入后的浮点数
func roundRankingFloat(value float64) float64 {
	return math.Round(value*10000) / 10000
}

// limitRankedModels 限制模型排行榜的展示条数。
// 如果 limit <= 0 或列表长度未超过限制，返回原列表。
// 参数:
//   - rows: 模型排行榜数据列表
//   - limit: 最大展示条数
// 返回值:
//   - []RankedModel: 截断后的列表
func limitRankedModels(rows []RankedModel, limit int) []RankedModel {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

// limitRankingMovers 限制排名变动列表的展示条数。
// 参数:
//   - rows: 排名变动数据列表
//   - limit: 最大展示条数
// 返回值:
//   - []RankingMover: 截断后的列表
func limitRankingMovers(rows []RankingMover, limit int) []RankingMover {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

// minInt 返回两个整数中较小的一个。
// 参数:
//   - a: 第一个整数
//   - b: 第二个整数
// 返回值:
//   - int: 较小的整数
func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
