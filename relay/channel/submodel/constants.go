// Package submodel 的常量定义文件。
// 定义了子模型渠道支持的模型列表和渠道名称。

package submodel

// ModelList 是子模型渠道支持的模型列表。
// 包含多个主流开源大语言模型，如 Qwen3、DeepSeek、GLM、GPT 等。
var ModelList = []string{
	"NousResearch/Hermes-4-405B-FP8",
	"Qwen/Qwen3-235B-A22B-Thinking-2507",
	"Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8",
	"Qwen/Qwen3-235B-A22B-Instruct-2507",
	"zai-org/GLM-4.5-FP8",
	"openai/gpt-oss-120b",
	"deepseek-ai/DeepSeek-R1-0528",
	"deepseek-ai/DeepSeek-R1",
	"deepseek-ai/DeepSeek-V3-0324",
	"deepseek-ai/DeepSeek-V3.1",
}

// ChannelName 渠道名称标识，用于路由和日志中识别子模型渠道。
const ChannelName = "submodel"
