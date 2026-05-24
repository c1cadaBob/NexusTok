// 本文件 (tool_billing.go) 提供工具调用的计费功能。
// 当 AI 请求中包含工具调用（如 Web 搜索、文件搜索、图片生成等）时，
// 需要在 Token 消耗之外额外收取工具调用费用。
// 费用计算基于工具单价、调用次数和用户分组比率。
package service

import (
	"math" // 数学运算（四舍五入）

	"github.com/c1cada/NexusTok/common"                        // 项目公共工具包（QuotaPerUnit 常量）
	"github.com/c1cada/NexusTok/setting/operation_setting"      // 运营设置（工具定价）
)

// ToolCallUsage 单次请求中所有工具调用的使用量记录。
// 用于捕获请求中包含的各种工具调用信息。
type ToolCallUsage struct {
	ModelName              string // 模型名称（用于查找模型特定的工具定价）
	WebSearchCalls         int    // Web 搜索调用次数
	WebSearchToolName      string // Web 搜索工具名称（如 "web_search_preview", "web_search" 等）
	FileSearchCalls        int    // 文件搜索调用次数
	ImageGenerationCall    bool   // 是否包含图片生成调用
	ImageGenerationQuality string // 图片生成质量（如 "low", "medium", "high"）
	ImageGenerationSize    string // 图片生成尺寸（如 "1024x1024"）
}

// ToolCallItem 单个工具调用的计费明细行。
type ToolCallItem struct {
	Name       string  `json:"name"`         // 工具名称
	CallCount  int     `json:"call_count"`   // 调用次数
	PricePer1K float64 `json:"price_per_1k"` // 每千次调用的单价（美元）
	TotalPrice float64 `json:"total_price"`  // 总价（美元）
	Quota      int     `json:"quota"`        // 折算后的配额数量
}

// ToolCallResult 工具调用计费的聚合结果。
type ToolCallResult struct {
	TotalQuota int            `json:"total_quota"`       // 所有工具调用的总配额消耗
	Items      []ToolCallItem `json:"items,omitempty"`   // 各工具的计费明细列表
}

// ComputeToolCallQuota 计算请求中所有工具调用的总配额消耗。
// 支持的工具类型：Web 搜索、文件搜索、图片生成。
// 计价逻辑：
// - Web 搜索和文件搜索：按每千次调用单价 * 调用次数 / 1000 计算
// - 图片生成：按单次调用价格计算（通过 GetGPTImage1PriceOnceCall 获取）
// - 所有费用再乘以 QuotaPerUnit（美元到配额的转换系数）和 groupRatio（分组比率）
//
// 工具价格通过 GetToolPriceForModel 解析，支持按模型前缀覆盖定价。
// 参数:
//   - usage: 请求中的工具调用使用量
//   - groupRatio: 用户分组的配额比率
// 返回值:
//   - ToolCallResult: 工具调用计费结果（包含总配额和明细）
func ComputeToolCallQuota(usage ToolCallUsage, groupRatio float64) ToolCallResult {
	var items []ToolCallItem
	totalQuota := 0

	// 内部辅助函数：添加单个工具的计费明细
	addItem := func(toolName string, count int) {
		if count <= 0 {
			return
		}
		// 获取该工具的每千次调用单价（支持按模型覆盖）
		pricePer1K := operation_setting.GetToolPriceForModel(toolName, usage.ModelName)
		if pricePer1K <= 0 {
			return // 未配置价格或价格为 0，不计费
		}
		// 计算总价：单价 * 次数 / 1000
		totalPrice := pricePer1K * float64(count) / 1000
		// 折算为配额：总价 * 每单位配额数 * 分组比率
		quota := int(math.Round(totalPrice * common.QuotaPerUnit * groupRatio))
		items = append(items, ToolCallItem{
			Name:       toolName,
			CallCount:  count,
			PricePer1K: pricePer1K,
			TotalPrice: totalPrice,
			Quota:      quota,
		})
		totalQuota += quota
	}

	// Web 搜索工具计费
	if usage.WebSearchCalls > 0 && usage.WebSearchToolName != "" {
		addItem(usage.WebSearchToolName, usage.WebSearchCalls)
	}

	// 文件搜索工具计费
	if usage.FileSearchCalls > 0 {
		addItem("file_search", usage.FileSearchCalls)
	}

	// 图片生成工具计费（按单次调用计价，不同于搜索工具的按千次计价）
	if usage.ImageGenerationCall {
		price := operation_setting.GetGPTImage1PriceOnceCall(usage.ImageGenerationQuality, usage.ImageGenerationSize)
		quota := int(math.Round(price * common.QuotaPerUnit * groupRatio))
		items = append(items, ToolCallItem{
			Name:       "image_generation",
			CallCount:  1,
			PricePer1K: price, // 对于图片生成，此字段存储单次调用价格
			TotalPrice: price,
			Quota:      quota,
		})
		totalQuota += quota
	}

	return ToolCallResult{
		TotalQuota: totalQuota,
		Items:      items,
	}
}
