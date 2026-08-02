// model_ratio.go — 模型定价比率与价格配置管理
// 职责：管理所有 AI 模型的定价相关配置，包括：
//   - ModelRatio：模型倍率（相对于基准价格的倍数）
//   - CompletionRatio：输出 token 相对于输入 token 的价格倍率
//   - ModelPrice：模型的固定价格（用于按次计费的模型）
//   - ImageRatio：图片生成模型的倍率
//   - AudioRatio / AudioCompletionRatio：音频模型的倍率
//
// 支持模型名通配符匹配（如 gpt-4-gizmo-*）和思考预算模型名规范化。
// 所有 Map 均为线程安全的 RWMap，更新时自动刷新暴露数据缓存。

package ratio_setting

import (
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/types"
)

// USD2RMB 美元兑人民币汇率（暂定值）
const (
	USD2RMB = 7.3 // 暂定 1 USD = 7.3 RMB
	USD     = 500 // $0.002 = 1 -> $1 = 500
	RMB     = USD / USD2RMB
)

// defaultModelRatio 默认的模型倍率配置
// 计费单位：1 === $0.002 / 1K tokens === ￥0.014 / 1k tokens
// 参考文档：
//   - https://platform.openai.com/docs/models/model-endpoint-compatibility
//   - https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Blfmc9dlf
//   - https://openai.com/pricing
var defaultModelRatio = map[string]float64{
	//"midjourney":                50,
	"gpt-4-gizmo-*":  15,
	"gpt-4o-gizmo-*": 2.5,
	"gpt-4-all":      15,
	"gpt-4o-all":     15,
	"gpt-4":          15,
	//"gpt-4-0314":                   15, //deprecated
	"gpt-4-0613": 15,
	"gpt-4-32k":  30,
	//"gpt-4-32k-0314":               30, //deprecated
	"gpt-4-32k-0613":                          30,
	"gpt-4-1106-preview":                      5,    // $10 / 1M tokens
	"gpt-4-0125-preview":                      5,    // $10 / 1M tokens
	"gpt-4-turbo-preview":                     5,    // $10 / 1M tokens
	"gpt-4-vision-preview":                    5,    // $10 / 1M tokens
	"gpt-4-1106-vision-preview":               5,    // $10 / 1M tokens
	"chatgpt-4o-latest":                       2.5,  // $5 / 1M tokens
	"gpt-4o":                                  1.25, // $2.5 / 1M tokens
	"gpt-4o-audio-preview":                    1.25, // $2.5 / 1M tokens
	"gpt-4o-audio-preview-2024-10-01":         1.25, // $2.5 / 1M tokens
	"gpt-4o-2024-05-13":                       2.5,  // $5 / 1M tokens
	"gpt-4o-2024-08-06":                       1.25, // $2.5 / 1M tokens
	"gpt-4o-2024-11-20":                       1.25, // $2.5 / 1M tokens
	"gpt-4o-realtime-preview":                 2.5,
	"gpt-4o-realtime-preview-2024-10-01":      2.5,
	"gpt-4o-realtime-preview-2024-12-17":      2.5,
	"gpt-4o-mini-realtime-preview":            0.3,
	"gpt-4o-mini-realtime-preview-2024-12-17": 0.3,
	"gpt-4.1":                          1.0,  // $2 / 1M tokens
	"gpt-4.1-2025-04-14":               1.0,  // $2 / 1M tokens
	"gpt-4.1-mini":                     0.2,  // $0.4 / 1M tokens
	"gpt-4.1-mini-2025-04-14":          0.2,  // $0.4 / 1M tokens
	"gpt-4.1-nano":                     0.05, // $0.1 / 1M tokens
	"gpt-4.1-nano-2025-04-14":          0.05, // $0.1 / 1M tokens
	"gpt-image-1":                      2.5,  // $5 / 1M tokens
	"o1":                               7.5,  // $15 / 1M tokens
	"o1-2024-12-17":                    7.5,  // $15 / 1M tokens
	"o1-preview":                       7.5,  // $15 / 1M tokens
	"o1-preview-2024-09-12":            7.5,  // $15 / 1M tokens
	"o1-mini":                          0.55, // $1.1 / 1M tokens
	"o1-mini-2024-09-12":               0.55, // $1.1 / 1M tokens
	"o1-pro":                           75.0, // $150 / 1M tokens
	"o1-pro-2025-03-19":                75.0, // $150 / 1M tokens
	"o3-mini":                          0.55,
	"o3-mini-2025-01-31":               0.55,
	"o3-mini-high":                     0.55,
	"o3-mini-2025-01-31-high":          0.55,
	"o3-mini-low":                      0.55,
	"o3-mini-2025-01-31-low":           0.55,
	"o3-mini-medium":                   0.55,
	"o3-mini-2025-01-31-medium":        0.55,
	"o3":                               1.0,  // $2 / 1M tokens
	"o3-2025-04-16":                    1.0,  // $2 / 1M tokens
	"o3-pro":                           10.0, // $20 / 1M tokens
	"o3-pro-2025-06-10":                10.0, // $20 / 1M tokens
	"o3-deep-research":                 5.0,  // $10 / 1M tokens
	"o3-deep-research-2025-06-26":      5.0,  // $10 / 1M tokens
	"o4-mini":                          0.55, // $1.1 / 1M tokens
	"o4-mini-2025-04-16":               0.55, // $1.1 / 1M tokens
	"o4-mini-deep-research":            1.0,  // $2 / 1M tokens
	"o4-mini-deep-research-2025-06-26": 1.0,  // $2 / 1M tokens
	"gpt-4o-mini":                      0.075,
	"gpt-4o-mini-2024-07-18":           0.075,
	"gpt-4-turbo":                      5, // $0.01 / 1K tokens
	"gpt-4-turbo-2024-04-09":           5, // $0.01 / 1K tokens
	"gpt-4.5-preview":                  37.5,
	"gpt-4.5-preview-2025-02-27":       37.5,
	"gpt-5":                            0.625,
	"gpt-5-2025-08-07":                 0.625,
	"gpt-5-chat-latest":                0.625,
	"gpt-5-mini":                       0.125,
	"gpt-5-mini-2025-08-07":            0.125,
	"gpt-5-nano":                       0.025,
	"gpt-5-nano-2025-08-07":            0.025,
	"gpt-5.5":                          2.5, // $5 / 1M tokens
	//"gpt-3.5-turbo-0301":           0.75, //deprecated
	"gpt-3.5-turbo":          0.25,
	"gpt-3.5-turbo-0613":     0.75,
	"gpt-3.5-turbo-16k":      1.5, // $0.003 / 1K tokens
	"gpt-3.5-turbo-16k-0613": 1.5,
	"gpt-3.5-turbo-instruct": 0.75, // $0.0015 / 1K tokens
	"gpt-3.5-turbo-1106":     0.5,  // $0.001 / 1K tokens
	"gpt-3.5-turbo-0125":     0.25,
	"babbage-002":            0.2, // $0.0004 / 1K tokens
	"davinci-002":            1,   // $0.002 / 1K tokens
	"text-ada-001":           0.2,
	"text-babbage-001":       0.25,
	"text-curie-001":         1,
	//"text-davinci-002":               10,
	//"text-davinci-003":               10,
	"text-davinci-edit-001":                     10,
	"code-davinci-edit-001":                     10,
	"whisper-1":                                 15,  // $0.006 / minute -> $0.006 / 150 words -> $0.006 / 200 tokens -> $0.03 / 1k tokens
	"tts-1":                                     7.5, // 1k characters -> $0.015
	"tts-1-1106":                                7.5, // 1k characters -> $0.015
	"tts-1-hd":                                  15,  // 1k characters -> $0.03
	"tts-1-hd-1106":                             15,  // 1k characters -> $0.03
	"davinci":                                   10,
	"curie":                                     10,
	"babbage":                                   10,
	"ada":                                       10,
	"text-embedding-3-small":                    0.01,
	"text-embedding-3-large":                    0.065,
	"text-embedding-ada-002":                    0.05,
	"text-search-ada-doc-001":                   10,
	"text-moderation-stable":                    0.1,
	"text-moderation-latest":                    0.1,
	"claude-3-haiku-20240307":                   0.125, // $0.25 / 1M tokens
	"claude-3-5-haiku-20241022":                 0.5,   // $1 / 1M tokens
	"claude-haiku-4-5-20251001":                 0.5,   // $1 / 1M tokens
	"claude-3-sonnet-20240229":                  1.5,   // $3 / 1M tokens
	"claude-3-5-sonnet-20240620":                1.5,
	"claude-3-5-sonnet-20241022":                1.5,
	"claude-3-7-sonnet-20250219":                1.5,
	"claude-3-7-sonnet-20250219-thinking":       1.5,
	"claude-sonnet-4-20250514":                  1.5,
	"claude-sonnet-4-5-20250929":                1.5,
	"claude-opus-4-5-20251101":                  2.5,
	"claude-opus-4-6":                           2.5,
	"claude-opus-4-6-max":                       2.5,
	"claude-opus-4-6-high":                      2.5,
	"claude-opus-4-6-medium":                    2.5,
	"claude-opus-4-6-low":                       2.5,
	"claude-opus-4-7":                           2.5,
	"claude-opus-4-7-max":                       2.5,
	"claude-opus-4-7-xhigh":                     2.5,
	"claude-opus-4-7-high":                      2.5,
	"claude-opus-4-7-medium":                    2.5,
	"claude-opus-4-7-low":                       2.5,
	"claude-3-opus-20240229":                    7.5, // $15 / 1M tokens
	"claude-opus-4-20250514":                    7.5,
	"claude-opus-4-1-20250805":                  7.5,
	"ERNIE-4.0-8K":                              0.120 * RMB,
	"ERNIE-3.5-8K":                              0.012 * RMB,
	"ERNIE-3.5-8K-0205":                         0.024 * RMB,
	"ERNIE-3.5-8K-1222":                         0.012 * RMB,
	"ERNIE-Bot-8K":                              0.024 * RMB,
	"ERNIE-3.5-4K-0205":                         0.012 * RMB,
	"ERNIE-Speed-8K":                            0.004 * RMB,
	"ERNIE-Speed-128K":                          0.004 * RMB,
	"ERNIE-Lite-8K-0922":                        0.008 * RMB,
	"ERNIE-Lite-8K-0308":                        0.003 * RMB,
	"ERNIE-Tiny-8K":                             0.001 * RMB,
	"BLOOMZ-7B":                                 0.004 * RMB,
	"Embedding-V1":                              0.002 * RMB,
	"bge-large-zh":                              0.002 * RMB,
	"bge-large-en":                              0.002 * RMB,
	"tao-8k":                                    0.002 * RMB,
	"PaLM-2":                                    1,
	"gemini-1.5-pro-latest":                     1.25, // $3.5 / 1M tokens
	"gemini-1.5-flash-latest":                   0.075,
	"gemini-2.0-flash":                          0.05,
	"gemini-2.5-pro-exp-03-25":                  0.625,
	"gemini-2.5-pro-preview-03-25":              0.625,
	"gemini-2.5-pro":                            0.625,
	"gemini-2.5-flash-preview-04-17":            0.075,
	"gemini-2.5-flash-preview-04-17-thinking":   0.075,
	"gemini-2.5-flash-preview-04-17-nothinking": 0.075,
	"gemini-2.5-flash-preview-05-20":            0.075,
	"gemini-2.5-flash-preview-05-20-thinking":   0.075,
	"gemini-2.5-flash-preview-05-20-nothinking": 0.075,
	"gemini-2.5-flash-thinking-*":               0.075, // 用于为后续所有2.5 flash thinking budget 模型设置默认倍率
	"gemini-2.5-pro-thinking-*":                 0.625, // 用于为后续所有2.5 pro thinking budget 模型设置默认倍率
	"gemini-2.5-flash-lite-preview-thinking-*":  0.05,
	"gemini-2.5-flash-lite-preview-06-17":       0.05,
	"gemini-2.5-flash":                          0.15,
	"gemini-robotics-er-1.5-preview":            0.15,
	"gemini-embedding-001":                      0.075,
	"text-embedding-004":                        0.001,
	"chatglm_turbo":                             0.3572,     // ￥0.005 / 1k tokens
	"chatglm_pro":                               0.7143,     // ￥0.01 / 1k tokens
	"chatglm_std":                               0.3572,     // ￥0.005 / 1k tokens
	"chatglm_lite":                              0.1429,     // ￥0.002 / 1k tokens
	"glm-4":                                     7.143,      // ￥0.1 / 1k tokens
	"glm-4v":                                    0.05 * RMB, // ￥0.05 / 1k tokens
	"glm-4-alltools":                            0.1 * RMB,  // ￥0.1 / 1k tokens
	"glm-3-turbo":                               0.3572,
	"glm-4-plus":                                0.05 * RMB,
	"glm-4-0520":                                0.1 * RMB,
	"glm-4-air":                                 0.001 * RMB,
	"glm-4-airx":                                0.01 * RMB,
	"glm-4-long":                                0.001 * RMB,
	"glm-4-flash":                               0,
	"glm-4v-plus":                               0.01 * RMB,
	"qwen-turbo":                                0.8572, // ￥0.012 / 1k tokens
	"qwen-plus":                                 10,     // ￥0.14 / 1k tokens
	"text-embedding-v1":                         0.05,   // ￥0.0007 / 1k tokens
	"SparkDesk-v1.1":                            1.2858, // ￥0.018 / 1k tokens
	"SparkDesk-v2.1":                            1.2858, // ￥0.018 / 1k tokens
	"SparkDesk-v3.1":                            1.2858, // ￥0.018 / 1k tokens
	"SparkDesk-v3.5":                            1.2858, // ￥0.018 / 1k tokens
	"SparkDesk-v4.0":                            1.2858,
	"360GPT_S2_V9":                              0.8572, // ¥0.012 / 1k tokens
	"360gpt-turbo":                              0.0858, // ¥0.0012 / 1k tokens
	"360gpt-turbo-responsibility-8k":            0.8572, // ¥0.012 / 1k tokens
	"360gpt-pro":                                0.8572, // ¥0.012 / 1k tokens
	"360gpt2-pro":                               0.8572, // ¥0.012 / 1k tokens
	"embedding-bert-512-v1":                     0.0715, // ¥0.001 / 1k tokens
	"embedding_s1_v1":                           0.0715, // ¥0.001 / 1k tokens
	"semantic_similarity_s1_v1":                 0.0715, // ¥0.001 / 1k tokens
	"hunyuan":                                   7.143,  // ¥0.1 / 1k tokens  // https://cloud.tencent.com/document/product/1729/97731#e0e6be58-60c8-469f-bdeb-6c264ce3b4d0
	// https://platform.lingyiwanwu.com/docs#-计费单元
	// 已经按照 7.2 来换算美元价格
	"yi-34b-chat-0205":       0.18,
	"yi-34b-chat-200k":       0.864,
	"yi-vl-plus":             0.432,
	"yi-large":               20.0 / 1000 * RMB,
	"yi-medium":              2.5 / 1000 * RMB,
	"yi-vision":              6.0 / 1000 * RMB,
	"yi-medium-200k":         12.0 / 1000 * RMB,
	"yi-spark":               1.0 / 1000 * RMB,
	"yi-large-rag":           25.0 / 1000 * RMB,
	"yi-large-turbo":         12.0 / 1000 * RMB,
	"yi-large-preview":       20.0 / 1000 * RMB,
	"yi-large-rag-preview":   25.0 / 1000 * RMB,
	"command":                0.5,
	"command-nightly":        0.5,
	"command-light":          0.5,
	"command-light-nightly":  0.5,
	"command-r":              0.25,
	"command-r-plus":         1.5,
	"command-r-08-2024":      0.075,
	"command-r-plus-08-2024": 1.25,
	"deepseek-chat":          0.27 / 2,
	"deepseek-coder":         0.27 / 2,
	"deepseek-reasoner":      0.55 / 2, // 0.55 / 1k tokens
	// Perplexity online 模型对搜索额外收费，有需要应自行调整，此处不计入搜索费用
	"llama-3-sonar-small-32k-chat":   0.2 / 1000 * USD,
	"llama-3-sonar-small-32k-online": 0.2 / 1000 * USD,
	"llama-3-sonar-large-32k-chat":   1 / 1000 * USD,
	"llama-3-sonar-large-32k-online": 1 / 1000 * USD,
	// grok
	"grok-3-beta":           1.5,
	"grok-3-mini-beta":      0.15,
	"grok-2":                1,
	"grok-2-vision":         1,
	"grok-beta":             2.5,
	"grok-vision-beta":      2.5,
	"grok-3-fast-beta":      2.5,
	"grok-3-mini-fast-beta": 0.3,
	// submodel
	"NousResearch/Hermes-4-405B-FP8":          0.8,
	"Qwen/Qwen3-235B-A22B-Thinking-2507":      0.6,
	"Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8": 0.8,
	"Qwen/Qwen3-235B-A22B-Instruct-2507":      0.3,
	"zai-org/GLM-4.5-FP8":                     0.8,
	"openai/gpt-oss-120b":                     0.5,
	"deepseek-ai/DeepSeek-R1-0528":            0.8,
	"deepseek-ai/DeepSeek-R1":                 0.8,
	"deepseek-ai/DeepSeek-V3-0324":            0.8,
	"deepseek-ai/DeepSeek-V3.1":               0.8,
}

// defaultModelPrice 默认的模型固定价格配置（按次计费）
// 适用于图片生成、音乐生成、视频生成等非 token 计费的模型
var defaultModelPrice = map[string]float64{
	"suno_music":                     0.1,
	"suno_lyrics":                    0.01,
	"dall-e-3":                       0.04,
	"imagen-3.0-generate-002":        0.03,
	"black-forest-labs/flux-1.1-pro": 0.04,
	"gpt-4-gizmo-*":                  0.1,
	"mj_video":                       0.8,
	"mj_imagine":                     0.1,
	"mj_edits":                       0.1,
	"mj_variation":                   0.1,
	"mj_reroll":                      0.1,
	"mj_blend":                       0.1,
	"mj_modal":                       0.1,
	"mj_zoom":                        0.1,
	"mj_shorten":                     0.1,
	"mj_high_variation":              0.1,
	"mj_low_variation":               0.1,
	"mj_pan":                         0.1,
	"mj_inpaint":                     0,
	"mj_custom_zoom":                 0,
	"mj_describe":                    0.05,
	"mj_upscale":                     0.05,
	"swap_face":                      0.05,
	"mj_upload":                      0.05,
	"sora-2":                         0.3,
	"sora-2-pro":                     0.5,
	"gpt-4o-mini-tts":                0.3,
	"veo-3.0-generate-001":           0.4,
	"veo-3.0-fast-generate-001":      0.15,
	"veo-3.1-generate-preview":       0.4,
	"veo-3.1-fast-generate-preview":  0.15,
}

// defaultAudioRatio 默认的音频模型输入倍率配置
var defaultAudioRatio = map[string]float64{
	"gpt-4o-audio-preview":         16,
	"gpt-4o-mini-audio-preview":    66.67,
	"gpt-4o-realtime-preview":      8,
	"gpt-4o-mini-realtime-preview": 16.67,
	"gpt-4o-mini-tts":              25,
}

// defaultAudioCompletionRatio 默认的音频模型输出倍率配置
// 值为 0 表示输出不计费（如 TTS 模型）
var defaultAudioCompletionRatio = map[string]float64{
	"gpt-4o-realtime":      2,
	"gpt-4o-mini-realtime": 2,
	"gpt-4o-mini-tts":      1,
	"tts-1":                0,
	"tts-1-hd":             0,
	"tts-1-1106":           0,
	"tts-1-hd-1106":        0,
}

// modelPriceMap 线程安全的模型固定价格 Map
var modelPriceMap = types.NewRWMap[string, float64]()

// modelRatioMap 线程安全的模型倍率 Map
var modelRatioMap = types.NewRWMap[string, float64]()

// completionRatioMap 线程安全的输出倍率 Map
var completionRatioMap = types.NewRWMap[string, float64]()

// defaultCompletionRatio 默认的输出倍率配置
// 输出 token 价格 = 输入 token 价格 * completionRatio
var defaultCompletionRatio = map[string]float64{
	"gpt-4-gizmo-*":  2,
	"gpt-4o-gizmo-*": 3,
	"gpt-4-all":      2,
	"gpt-image-1":    8,
}

// InitRatioSettings 初始化所有模型定价相关的 Map
// 将默认配置加载到各线程安全 Map 中
func InitRatioSettings() {
	modelPriceMap.AddAll(defaultModelPrice)
	modelRatioMap.AddAll(defaultModelRatio)
	completionRatioMap.AddAll(defaultCompletionRatio)
	cacheRatioMap.AddAll(defaultCacheRatio)
	createCacheRatioMap.AddAll(defaultCreateCacheRatio)
	imageRatioMap.AddAll(defaultImageRatio)
	audioRatioMap.AddAll(defaultAudioRatio)
	audioCompletionRatioMap.AddAll(defaultAudioCompletionRatio)
}

// GetModelPriceMap 获取所有模型固定价格的副本
// 返回值：模型名到价格的映射副本
func GetModelPriceMap() map[string]float64 {
	return modelPriceMap.ReadAll()
}

// ModelPrice2JSONString 将模型固定价格 Map 序列化为 JSON 字符串
// 返回值：JSON 字符串
func ModelPrice2JSONString() string {
	return modelPriceMap.MarshalJSONString()
}

// UpdateModelPriceByJSONString 从 JSON 字符串更新模型固定价格配置
// 更新后会自动刷新暴露数据缓存
// 参数：
//   - jsonStr: JSON 格式的价格配置字符串
//
// 返回值：解析失败时返回错误
func UpdateModelPriceByJSONString(jsonStr string) error {
	return loadFloatMapWithDefaults(modelPriceMap, jsonStr, defaultModelPrice, InvalidateExposedDataCache)
}

// GetModelPrice 获取指定模型的固定价格
// 支持紧凑模型后缀的通配符匹配
// 参数：
//   - name: 模型名称
//   - printErr: 未找到时是否打印错误日志
//
// 返回值：
//   - float64: 模型价格
//   - bool: 是否找到该模型的价格
func GetModelPrice(name string, printErr bool) (float64, bool) {
	name = FormatMatchingModelName(name)

	if price, ok := modelPriceMap.Get(name); ok {
		return price, true
	}

	if strings.HasSuffix(name, CompactModelSuffix) {
		price, ok := modelPriceMap.Get(CompactWildcardModelKey)
		if !ok {
			if printErr {
				common.SysError("model price not found: " + name)
			}
			return -1, false
		}
		return price, true
	}

	if printErr {
		common.SysError("model price not found: " + name)
	}
	return -1, false
}

// UpdateModelRatioByJSONString 从 JSON 字符串更新模型倍率配置
// 更新后会自动刷新暴露数据缓存
// 参数：
//   - jsonStr: JSON 格式的倍率配置字符串
//
// 返回值：解析失败时返回错误
func UpdateModelRatioByJSONString(jsonStr string) error {
	return loadFloatMapWithDefaults(modelRatioMap, jsonStr, defaultModelRatio, InvalidateExposedDataCache)
}

// loadFloatMapWithDefaults 将持久化 JSON 作为覆盖层叠加到内置默认值上。
//
// 历史版本会把数据库中的 ModelRatio/ModelPrice 完全替换内存默认值；当旧数据库里保存了
// `{}` 或很早以前的不完整 JSON 时，代码中新增的模型默认价格会被清空，最终导致 relay
// 热路径误报“模型价格未配置”。这里先解析覆盖层，解析成功后一次性替换为“默认值 + 覆盖值”，
// 既保留管理员自定义价格，也保证随版本发布的新默认模型在旧实例升级后立即可用。
func loadFloatMapWithDefaults(target *types.RWMap[string, float64], jsonStr string, defaults map[string]float64, onSuccess func()) error {
	overrides := make(map[string]float64)
	if strings.TrimSpace(jsonStr) != "" {
		if err := common.Unmarshal([]byte(jsonStr), &overrides); err != nil {
			return err
		}
	}
	merged := copyFloatMap(defaults)
	for key, value := range overrides {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		merged[key] = value
	}
	target.ReplaceAll(merged)
	if onSuccess != nil {
		onSuccess()
	}
	return nil
}

// handleThinkingBudgetModel 处理带有思考预算的模型名称
// 将带 thinking budget 参数的模型名规范化为通配符形式，方便统一定价
// 参数：
//   - name: 原始模型名
//   - prefix: 模型前缀
//   - wildcard: 通配符目标名称
//
// 返回值：规范化后的模型名
func handleThinkingBudgetModel(name, prefix, wildcard string) string {
	if strings.HasPrefix(name, prefix) && strings.Contains(name, "-thinking-") {
		return wildcard
	}
	return name
}

// GetModelRatio 获取指定模型的倍率
// 支持紧凑模型后缀的通配符匹配，未找到时返回默认倍率 37.5
// 参数：
//   - name: 模型名称
//
// 返回值：
//   - float64: 模型倍率
//   - bool: 是否找到该模型
//   - string: 匹配后的模型名
func GetModelRatio(name string) (float64, bool, string) {
	name = FormatMatchingModelName(name)

	ratio, ok := modelRatioMap.Get(name)
	if !ok {
		if strings.HasSuffix(name, CompactModelSuffix) {
			if wildcardRatio, ok := modelRatioMap.Get(CompactWildcardModelKey); ok {
				return wildcardRatio, true, name
			}
			//return 0, true, name
		}
		return 37.5, operation_setting.SelfUseModeEnabled, name
	}
	return ratio, true, name
}

// DefaultModelRatio2JSONString 将默认模型倍率配置序列化为 JSON 字符串
// 返回值：JSON 字符串
func DefaultModelRatio2JSONString() string {
	jsonBytes, err := common.Marshal(defaultModelRatio)
	if err != nil {
		common.SysError("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func copyFloatMap(source map[string]float64) map[string]float64 {
	copied := make(map[string]float64, len(source))
	for key, value := range source {
		copied[key] = value
	}
	return copied
}

// GetDefaultModelRatioMap 获取默认模型倍率 Map 的副本。
// 返回值：默认倍率 Map 副本，调用方修改不会影响包内默认配置。
func GetDefaultModelRatioMap() map[string]float64 {
	return copyFloatMap(defaultModelRatio)
}

// GetDefaultModelPriceMap 获取默认模型固定价格 Map 的副本。
// 返回值：默认价格 Map 副本。
func GetDefaultModelPriceMap() map[string]float64 {
	return copyFloatMap(defaultModelPrice)
}

// GetDefaultCompletionRatioMap 获取默认输出倍率 Map 的副本。
// 用于区分系统内置倍率和管理员持久化覆盖倍率。
func GetDefaultCompletionRatioMap() map[string]float64 {
	return copyFloatMap(defaultCompletionRatio)
}

// CompletionRatio2JSONString 将输出倍率 Map 序列化为 JSON 字符串
// 返回值：JSON 字符串
func CompletionRatio2JSONString() string {
	return completionRatioMap.MarshalJSONString()
}

// UpdateCompletionRatioByJSONString 从 JSON 字符串更新输出倍率配置
// 更新后会自动刷新暴露数据缓存
// 参数：
//   - jsonStr: JSON 格式的输出倍率配置字符串
//
// 返回值：解析失败时返回错误
func UpdateCompletionRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(completionRatioMap, jsonStr, InvalidateExposedDataCache)
}

// GetCompletionRatio 获取指定模型的输出倍率
// 优先查找自定义配置，然后查找硬编码的默认倍率
// 参数：
//   - name: 模型名称
//
// 返回值：输出倍率
func GetCompletionRatio(name string) float64 {
	name = FormatMatchingModelName(name)

	if strings.Contains(name, "/") {
		if ratio, ok := completionRatioMap.Get(name); ok {
			return ratio
		}
	}
	hardCodedRatio, contain := getHardcodedCompletionModelRatio(name)
	if contain {
		return hardCodedRatio
	}
	if ratio, ok := completionRatioMap.Get(name); ok {
		return ratio
	}
	return hardCodedRatio
}

// CompletionRatioInfo 输出倍率信息结构体，包含倍率值和是否锁定
type CompletionRatioInfo struct {
	// Ratio 输出倍率值
	Ratio float64 `json:"ratio"`
	// Locked 是否为硬编码锁定的倍率（不可通过配置覆盖）
	Locked bool `json:"locked"`
}

// GetCompletionRatioInfo 获取指定模型的输出倍率详细信息
// 区分硬编码锁定的倍率和可配置的倍率
// 参数：
//   - name: 模型名称
//
// 返回值：输出倍率信息
func GetCompletionRatioInfo(name string) CompletionRatioInfo {
	name = FormatMatchingModelName(name)

	if strings.Contains(name, "/") {
		if ratio, ok := completionRatioMap.Get(name); ok {
			return CompletionRatioInfo{
				Ratio:  ratio,
				Locked: false,
			}
		}
	}

	hardCodedRatio, locked := getHardcodedCompletionModelRatio(name)
	if locked {
		return CompletionRatioInfo{
			Ratio:  hardCodedRatio,
			Locked: true,
		}
	}

	if ratio, ok := completionRatioMap.Get(name); ok {
		return CompletionRatioInfo{
			Ratio:  ratio,
			Locked: false,
		}
	}

	return CompletionRatioInfo{
		Ratio:  hardCodedRatio,
		Locked: false,
	}
}

// getHardcodedCompletionModelRatio 获取硬编码的模型输出倍率
// 根据模型名称前缀匹配返回预设的输出倍率
// 参数：
//   - name: 模型名称
//
// 返回值：
//   - float64: 输出倍率
//   - bool: 是否为锁定的硬编码值（不可配置覆盖）
func getHardcodedCompletionModelRatio(name string) (float64, bool) {

	isReservedModel := strings.HasSuffix(name, "-all") || strings.HasSuffix(name, "-gizmo-*")
	if isReservedModel {
		return 2, false
	}

	if strings.HasPrefix(name, "gpt-") {
		if strings.HasPrefix(name, "gpt-4o") {
			if name == "gpt-4o-2024-05-13" {
				return 3, true
			}
			if strings.HasPrefix(name, "gpt-4o-mini-tts") {
				return 20, false
			}
			return 4, false
		}
		// gpt-5 匹配
		if strings.HasPrefix(name, "gpt-5") {
			if strings.HasPrefix(name, "gpt-5.5") {
				return 6, true
			}
			if strings.HasPrefix(name, "gpt-5.4") {
				if strings.HasPrefix(name, "gpt-5.4-nano") {
					return 6.25, true
				}
				return 6, true
			}
			return 8, true
		}
		// gpt-4.5-preview匹配
		if strings.HasPrefix(name, "gpt-4.5-preview") {
			return 2, true
		}
		if strings.HasPrefix(name, "gpt-4-turbo") || strings.HasSuffix(name, "gpt-4-1106") || strings.HasSuffix(name, "gpt-4-1105") {
			return 3, true
		}
		// 没有特殊标记的 gpt-4 模型默认倍率为 2
		return 2, false
	}
	if strings.HasPrefix(name, "o1") || strings.HasPrefix(name, "o3") {
		return 4, true
	}
	if name == "chatgpt-4o-latest" {
		return 3, true
	}

	if strings.Contains(name, "claude-3") {
		return 5, true
	} else if strings.Contains(name, "claude-sonnet-4") || strings.Contains(name, "claude-opus-4") || strings.Contains(name, "claude-haiku-4") {
		return 5, true
	}

	if strings.HasPrefix(name, "gpt-3.5") {
		if name == "gpt-3.5-turbo" || strings.HasSuffix(name, "0125") {
			// https://openai.com/blog/new-embedding-models-and-api-updates
			// Updated GPT-3.5 Turbo model and lower pricing
			return 3, true
		}
		if strings.HasSuffix(name, "1106") {
			return 2, true
		}
		return 4.0 / 3.0, true
	}
	if strings.HasPrefix(name, "mistral-") {
		return 3, true
	}
	if strings.HasPrefix(name, "gemini-") {
		if strings.HasPrefix(name, "gemini-1.5") {
			return 4, true
		} else if strings.HasPrefix(name, "gemini-2.0") {
			return 4, true
		} else if strings.HasPrefix(name, "gemini-2.5-pro") { // 移除preview来增加兼容性，这里假设正式版的倍率和preview一致
			return 8, false
		} else if strings.HasPrefix(name, "gemini-2.5-flash") { // 处理不同的flash模型倍率
			if strings.HasPrefix(name, "gemini-2.5-flash-preview") {
				if strings.HasSuffix(name, "-nothinking") {
					return 4, false
				}
				return 3.5 / 0.15, false
			}
			if strings.HasPrefix(name, "gemini-2.5-flash-lite") {
				return 4, false
			}
			return 2.5 / 0.3, false
		} else if strings.HasPrefix(name, "gemini-robotics-er-1.5") {
			return 2.5 / 0.3, false
		} else if strings.HasPrefix(name, "gemini-3-pro") {
			if strings.HasPrefix(name, "gemini-3-pro-image") {
				return 60, false
			}
			return 6, false
		}
		return 4, false
	}
	if strings.HasPrefix(name, "command") {
		switch name {
		case "command-r":
			return 3, true
		case "command-r-plus":
			return 5, true
		case "command-r-08-2024":
			return 4, true
		case "command-r-plus-08-2024":
			return 4, true
		default:
			return 4, false
		}
	}
	// hint 只给官方上4倍率，由于开源模型供应商自行定价，不对其进行补全倍率进行强制对齐
	if strings.HasPrefix(name, "ERNIE-Speed-") {
		return 2, true
	} else if strings.HasPrefix(name, "ERNIE-Lite-") {
		return 2, true
	} else if strings.HasPrefix(name, "ERNIE-Character") {
		return 2, true
	} else if strings.HasPrefix(name, "ERNIE-Functions") {
		return 2, true
	}
	switch name {
	case "llama2-70b-4096":
		return 0.8 / 0.64, true
	case "llama3-8b-8192":
		return 2, true
	case "llama3-70b-8192":
		return 0.79 / 0.59, true
	}
	return 1, false
}

// GetAudioRatio 获取指定音频模型的输入倍率
// 参数：
//   - name: 模型名称
//
// 返回值：音频输入倍率，未找到时默认返回 1
func GetAudioRatio(name string) float64 {
	name = FormatMatchingModelName(name)
	if ratio, ok := audioRatioMap.Get(name); ok {
		return ratio
	}
	return 1
}

// GetAudioCompletionRatio 获取指定音频模型的输出倍率
// 参数：
//   - name: 模型名称
//
// 返回值：音频输出倍率，未找到时默认返回 1
func GetAudioCompletionRatio(name string) float64 {
	name = FormatMatchingModelName(name)
	if ratio, ok := audioCompletionRatioMap.Get(name); ok {
		return ratio
	}
	return 1
}

// ContainsAudioRatio 判断指定音频模型是否配置了输入倍率
// 参数：
//   - name: 模型名称
//
// 返回值：存在则返回 true
func ContainsAudioRatio(name string) bool {
	name = FormatMatchingModelName(name)
	_, ok := audioRatioMap.Get(name)
	return ok
}

// ContainsAudioCompletionRatio 判断指定音频模型是否配置了输出倍率
// 参数：
//   - name: 模型名称
//
// 返回值：存在则返回 true
func ContainsAudioCompletionRatio(name string) bool {
	name = FormatMatchingModelName(name)
	_, ok := audioCompletionRatioMap.Get(name)
	return ok
}

// ModelRatio2JSONString 将模型倍率 Map 序列化为 JSON 字符串
// 返回值：JSON 字符串
func ModelRatio2JSONString() string {
	return modelRatioMap.MarshalJSONString()
}

// defaultImageRatio 默认的图片生成模型倍率配置
var defaultImageRatio = map[string]float64{
	"gpt-image-1": 2,
}

// imageRatioMap 线程安全的图片倍率 Map
var imageRatioMap = types.NewRWMap[string, float64]()

// audioRatioMap 线程安全的音频输入倍率 Map
var audioRatioMap = types.NewRWMap[string, float64]()

// audioCompletionRatioMap 线程安全的音频输出倍率 Map
var audioCompletionRatioMap = types.NewRWMap[string, float64]()

// ImageRatio2JSONString 将图片倍率 Map 序列化为 JSON 字符串
// 返回值：JSON 字符串
func ImageRatio2JSONString() string {
	return imageRatioMap.MarshalJSONString()
}

// GetDefaultImageRatioMap 获取默认图片倍率 Map 的副本。
func GetDefaultImageRatioMap() map[string]float64 {
	return copyFloatMap(defaultImageRatio)
}

// UpdateImageRatioByJSONString 从 JSON 字符串更新图片倍率配置
// 参数：
//   - jsonStr: JSON 格式的图片倍率配置字符串
//
// 返回值：解析失败时返回错误
func UpdateImageRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(imageRatioMap, jsonStr)
}

// GetImageRatio 获取指定图片模型的倍率
// 参数：
//   - name: 模型名称
//
// 返回值：
//   - float64: 图片倍率，未找到时默认返回 1
//   - bool: 是否在配置中找到了该模型
func GetImageRatio(name string) (float64, bool) {
	ratio, ok := imageRatioMap.Get(name)
	if !ok {
		return 1, false // Default to 1 if not found
	}
	return ratio, true
}

// AudioRatio2JSONString 将音频输入倍率 Map 序列化为 JSON 字符串
// 返回值：JSON 字符串
func AudioRatio2JSONString() string {
	return audioRatioMap.MarshalJSONString()
}

// GetDefaultAudioRatioMap 获取默认音频输入倍率 Map 的副本。
func GetDefaultAudioRatioMap() map[string]float64 {
	return copyFloatMap(defaultAudioRatio)
}

// UpdateAudioRatioByJSONString 从 JSON 字符串更新音频输入倍率配置
// 更新后会自动刷新暴露数据缓存
// 参数：
//   - jsonStr: JSON 格式的音频倍率配置字符串
//
// 返回值：解析失败时返回错误
func UpdateAudioRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(audioRatioMap, jsonStr, InvalidateExposedDataCache)
}

// AudioCompletionRatio2JSONString 将音频输出倍率 Map 序列化为 JSON 字符串
// 返回值：JSON 字符串
func AudioCompletionRatio2JSONString() string {
	return audioCompletionRatioMap.MarshalJSONString()
}

// GetDefaultAudioCompletionRatioMap 获取默认音频输出倍率 Map 的副本。
func GetDefaultAudioCompletionRatioMap() map[string]float64 {
	return copyFloatMap(defaultAudioCompletionRatio)
}

// UpdateAudioCompletionRatioByJSONString 从 JSON 字符串更新音频输出倍率配置
// 更新后会自动刷新暴露数据缓存
// 参数：
//   - jsonStr: JSON 格式的音频输出倍率配置字符串
//
// 返回值：解析失败时返回错误
func UpdateAudioCompletionRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(audioCompletionRatioMap, jsonStr, InvalidateExposedDataCache)
}

// GetModelRatioCopy 获取所有模型倍率的副本
// 返回值：模型名到倍率的映射副本
func GetModelRatioCopy() map[string]float64 {
	return modelRatioMap.ReadAll()
}

// GetModelPriceCopy 获取所有模型固定价格的副本
// 返回值：模型名到价格的映射副本
func GetModelPriceCopy() map[string]float64 {
	return modelPriceMap.ReadAll()
}

// GetCompletionRatioCopy 获取所有输出倍率的副本
// 返回值：模型名到输出倍率的映射副本
func GetCompletionRatioCopy() map[string]float64 {
	return completionRatioMap.ReadAll()
}

// GetImageRatioCopy 获取所有图片倍率的副本
// 返回值：模型名到图片倍率的映射副本
func GetImageRatioCopy() map[string]float64 {
	return imageRatioMap.ReadAll()
}

// GetAudioRatioCopy 获取所有音频输入倍率的副本
// 返回值：模型名到音频倍率的映射副本
func GetAudioRatioCopy() map[string]float64 {
	return audioRatioMap.ReadAll()
}

// GetAudioCompletionRatioCopy 获取所有音频输出倍率的副本
// 返回值：模型名到音频输出倍率的映射副本
func GetAudioCompletionRatioCopy() map[string]float64 {
	return audioCompletionRatioMap.ReadAll()
}

// FormatMatchingModelName 规范化模型名称，减少渠道配置的复杂度
// 将带思考预算参数的 Gemini 模型名转换为通配符形式，
// 将 gpt-4-gizmo/gpt-4o-gizmo 前缀的模型名转换为统配符形式
// 参数：
//   - name: 原始模型名称
//
// 返回值：规范化后的模型名称
func FormatMatchingModelName(name string) string {

	if strings.HasPrefix(name, "gemini-2.5-flash-lite") {
		name = handleThinkingBudgetModel(name, "gemini-2.5-flash-lite", "gemini-2.5-flash-lite-thinking-*")
	} else if strings.HasPrefix(name, "gemini-2.5-flash") {
		name = handleThinkingBudgetModel(name, "gemini-2.5-flash", "gemini-2.5-flash-thinking-*")
	} else if strings.HasPrefix(name, "gemini-2.5-pro") {
		name = handleThinkingBudgetModel(name, "gemini-2.5-pro", "gemini-2.5-pro-thinking-*")
	}

	if strings.HasPrefix(name, "gpt-4-gizmo") {
		name = "gpt-4-gizmo-*"
	}
	if strings.HasPrefix(name, "gpt-4o-gizmo") {
		name = "gpt-4o-gizmo-*"
	}
	return name
}

// GetModelRatioOrPrice 获取模型的倍率或固定价格
// 优先返回固定价格，无固定价格时返回倍率
// 参数：
//   - model: 模型名称
//
// 返回值：
//   - float64: 倍率或价格值
//   - bool: 是否为固定价格（true）还是倍率（false）
//   - bool: 是否在配置中找到该模型
func GetModelRatioOrPrice(model string) (float64, bool, bool) { // price or ratio
	price, usePrice := GetModelPrice(model, false)
	if usePrice {
		return price, true, true
	}
	modelRatio, success, _ := GetModelRatio(model)
	if success {
		return modelRatio, false, true
	}
	return 37.5, false, false
}
