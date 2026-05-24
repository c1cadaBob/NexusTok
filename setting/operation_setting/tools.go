// tools.go — 工具调用定价与特殊模型定价配置
// 职责：管理 AI 工具调用（如 Web Search、File Search）的定价配置，
// 支持按工具名和模型名前缀匹配的分级定价策略。
// 同时包含 GPT Image 1 的按调用定价和 Gemini 音频输入的按 token 定价。

package operation_setting

import (
	"sort"
	"strings"
	"sync/atomic"

	"github.com/c1cada/NexusTok/setting/config"
)

// ---------------------------------------------------------------------------
// 工具调用价格（$/千次调用，管理员可配置）
// 数据库键: tool_price_setting.prices
//
// 键格式：
//   - "tool_name"              → 该工具对所有模型的默认价格
//   - "tool_name:model_prefix*" → 该工具对匹配前缀的模型的覆盖价格
//
// 查找顺序：最长前缀匹配 → 工具默认价 → 硬编码回退 → 0
// ---------------------------------------------------------------------------

// defaultToolPrices 工具调用的默认价格配置（$/千次调用）
var defaultToolPrices = map[string]float64{
	"web_search":         10.0, // OpenAI 网页搜索（所有模型）/ Claude 网页搜索
	"web_search_preview": 10.0, // OpenAI 网页搜索预览（默认：推理模型）
	"file_search":        2.5,  // OpenAI 文件搜索（Responses API）
	"google_search":      14.0, // Gemini Grounding with Google Search
}

// defaultToolPriceOverrides 工具调用价格的模型特定覆盖配置
var defaultToolPriceOverrides = map[string]float64{
	"web_search_preview:gpt-4o*":       25.0, // 非推理模型
	"web_search_preview:gpt-4.1*":      25.0,
	"web_search_preview:gpt-4o-mini*":  25.0,
	"web_search_preview:gpt-4.1-mini*": 25.0,
}

// ToolPriceSetting 工具价格配置结构体，通过 config.GlobalConfig.Register 注册管理
type ToolPriceSetting struct {
	// Prices 存储工具调用价格，键格式见上方注释
	Prices map[string]float64 `json:"prices"`
}

// toolPriceSetting 是全局工具价格配置实例，初始化时合并默认价格和覆盖价格
var toolPriceSetting = ToolPriceSetting{
	Prices: func() map[string]float64 {
		m := make(map[string]float64, len(defaultToolPrices)+len(defaultToolPriceOverrides))
		for k, v := range defaultToolPrices {
			m[k] = v
		}
		for k, v := range defaultToolPriceOverrides {
			m[k] = v
		}
		return m
	}(),
}

// init 注册工具价格配置并构建初始查找索引
func init() {
	config.GlobalConfig.Register("tool_price_setting", &toolPriceSetting)
	RebuildToolPriceIndex()
}

// ---------------------------------------------------------------------------
// 预计算的价格索引（原子操作，读路径无锁）
// ---------------------------------------------------------------------------

// prefixEntry 表示一个模型前缀匹配条目
type prefixEntry struct {
	prefix string  // 模型名前缀
	price  float64 // 对应的价格
}

// toolPriceIndex 工具价格查找索引
type toolPriceIndex struct {
	defaults map[string]float64          // 工具名 → 默认价格
	prefixes map[string][]prefixEntry     // 工具名 → 按前缀长度降序排列的前缀条目
}

// currentIndex 原子存储的当前价格索引，支持无锁读取
var currentIndex atomic.Pointer[toolPriceIndex]

// RebuildToolPriceIndex 从当前配置重建价格查找索引
// 在初始化时和配置更新后调用，不在计费热路径上
func RebuildToolPriceIndex() {
	// 合并三层价格配置：默认价格 → 覆盖价格 → 用户自定义价格
	merged := make(map[string]float64, len(defaultToolPrices)+len(defaultToolPriceOverrides)+len(toolPriceSetting.Prices))
	for k, v := range defaultToolPrices {
		merged[k] = v
	}
	for k, v := range defaultToolPriceOverrides {
		merged[k] = v
	}
	for k, v := range toolPriceSetting.Prices {
		merged[k] = v
	}

	idx := &toolPriceIndex{
		defaults: make(map[string]float64),
		prefixes: make(map[string][]prefixEntry),
	}

	// 将合并后的配置拆分为默认价格和前缀匹配价格
	for key, price := range merged {
		colonIdx := strings.IndexByte(key, ':')
		if colonIdx < 0 {
			// 无冒号，作为工具默认价格
			idx.defaults[key] = price
			continue
		}
		// 有冒号，解析为 "工具名:模型前缀*" 格式
		toolName := key[:colonIdx]
		modelPart := key[colonIdx+1:]
		prefix := strings.TrimSuffix(modelPart, "*")
		idx.prefixes[toolName] = append(idx.prefixes[toolName], prefixEntry{prefix: prefix, price: price})
	}

	// 对每个工具的前缀条目按前缀长度降序排列，确保最长前缀优先匹配
	for tool := range idx.prefixes {
		entries := idx.prefixes[tool]
		sort.Slice(entries, func(i, j int) bool {
			return len(entries[i].prefix) > len(entries[j].prefix)
		})
		idx.prefixes[tool] = entries
	}

	currentIndex.Store(idx)
}

// GetToolPriceForModel 根据工具名和模型名获取工具调用价格（$/千次调用）
// 查找顺序：最长前缀匹配 → 工具默认价 → 0
// 参数：
//   - toolName: 工具名称
//   - modelName: 模型名称
//
// 返回值：工具调用价格
func GetToolPriceForModel(toolName, modelName string) float64 {
	idx := currentIndex.Load()
	if idx == nil {
		// 索引未构建时的回退逻辑
		if v, ok := defaultToolPrices[toolName]; ok {
			return v
		}
		return 0
	}

	// 尝试前缀匹配（entries 已按长度降序排列，第一个匹配即为最长前缀）
	if entries, ok := idx.prefixes[toolName]; ok && modelName != "" {
		for _, e := range entries {
			if strings.HasPrefix(modelName, e.prefix) {
				return e.price
			}
		}
	}

	// 前缀未匹配，使用工具默认价格
	if p, ok := idx.defaults[toolName]; ok {
		return p
	}
	return 0
}

// GetToolPrice 获取工具的默认调用价格（不考虑模型名）
// 参数：
//   - toolName: 工具名称
//
// 返回值：工具调用价格
func GetToolPrice(toolName string) float64 {
	return GetToolPriceForModel(toolName, "")
}

// ---------------------------------------------------------------------------
// GPT Image 1 按调用定价（特殊：取决于质量和尺寸）
// ---------------------------------------------------------------------------

// GPT Image 1 各质量/尺寸组合的单次调用价格（美元）
const (
	GPTImage1Low1024x1024    = 0.011
	GPTImage1Low1024x1536    = 0.016
	GPTImage1Low1536x1024    = 0.016
	GPTImage1Medium1024x1024 = 0.042
	GPTImage1Medium1024x1536 = 0.063
	GPTImage1Medium1536x1024 = 0.063
	GPTImage1High1024x1024   = 0.167
	GPTImage1High1024x1536   = 0.25
	GPTImage1High1536x1024   = 0.25
)

// GetGPTImage1PriceOnceCall 根据质量和尺寸获取 GPT Image 1 的单次调用价格
// 参数：
//   - quality: 图像质量，可选 "low"、"medium"、"high"
//   - size: 图像尺寸，可选 "1024x1024"、"1024x1536"、"1536x1024"
//
// 返回值：单次调用价格（美元），未匹配时回退到 high 1024x1024 价格
func GetGPTImage1PriceOnceCall(quality string, size string) float64 {
	prices := map[string]map[string]float64{
		"low": {
			"1024x1024": GPTImage1Low1024x1024,
			"1024x1536": GPTImage1Low1024x1536,
			"1536x1024": GPTImage1Low1536x1024,
		},
		"medium": {
			"1024x1024": GPTImage1Medium1024x1024,
			"1024x1536": GPTImage1Medium1024x1536,
			"1536x1024": GPTImage1Medium1536x1024,
		},
		"high": {
			"1024x1024": GPTImage1High1024x1024,
			"1024x1536": GPTImage1High1024x1536,
			"1536x1024": GPTImage1High1536x1024,
		},
	}

	if qualityMap, exists := prices[quality]; exists {
		if price, exists := qualityMap[size]; exists {
			return price
		}
	}

	// 回退到 high 1024x1024 的默认价格
	return GPTImage1High1024x1024
}

// ---------------------------------------------------------------------------
// Gemini 音频输入定价（每百万 token，按模型区分）
// ---------------------------------------------------------------------------

// Gemini 各模型的音频输入价格（$/百万 token）
const (
	Gemini25FlashPreviewInputAudioPrice     = 1.00  // Gemini 2.5 Flash Preview
	Gemini25FlashProductionInputAudioPrice  = 1.00  // Gemini 2.5 Flash 正式版
	Gemini25FlashLitePreviewInputAudioPrice = 0.50  // Gemini 2.5 Flash Lite Preview
	Gemini25FlashNativeAudioInputAudioPrice = 3.00  // Gemini 2.5 Flash 原生音频
	Gemini20FlashInputAudioPrice            = 0.70  // Gemini 2.0 Flash
	GeminiRoboticsER15InputAudioPrice       = 1.00  // Gemini Robotics ER 1.5
)

// GetGeminiInputAudioPricePerMillionTokens 根据模型名获取 Gemini 音频输入价格
// 使用前缀匹配，按优先级从高到低检查
// 参数：
//   - modelName: 模型名称
//
// 返回值：音频输入价格（$/百万 token），未匹配时返回 0
func GetGeminiInputAudioPricePerMillionTokens(modelName string) float64 {
	if strings.HasPrefix(modelName, "gemini-2.5-flash-preview-native-audio") {
		return Gemini25FlashNativeAudioInputAudioPrice
	} else if strings.HasPrefix(modelName, "gemini-2.5-flash-preview-lite") {
		return Gemini25FlashLitePreviewInputAudioPrice
	} else if strings.HasPrefix(modelName, "gemini-2.5-flash-preview") {
		return Gemini25FlashPreviewInputAudioPrice
	} else if strings.HasPrefix(modelName, "gemini-2.5-flash") {
		return Gemini25FlashProductionInputAudioPrice
	} else if strings.HasPrefix(modelName, "gemini-2.0-flash") {
		return Gemini20FlashInputAudioPrice
	} else if strings.HasPrefix(modelName, "gemini-robotics-er-1.5") {
		return GeminiRoboticsER15InputAudioPrice
	}
	return 0
}
