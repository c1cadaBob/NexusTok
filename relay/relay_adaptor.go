// Package relay 实现了 AI API 中继/代理的核心路由逻辑。
// 本文件负责根据 API 类型（如 OpenAI、Claude、Gemini 等）和任务平台类型，
// 创建并返回对应的上游渠道适配器（Adaptor）实例。
package relay

import (
	"strconv"

	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/advancedcustom"
	"github.com/c1cada/NexusTok/relay/channel/ali"
	"github.com/c1cada/NexusTok/relay/channel/aws"
	"github.com/c1cada/NexusTok/relay/channel/baidu"
	"github.com/c1cada/NexusTok/relay/channel/baidu_v2"
	"github.com/c1cada/NexusTok/relay/channel/claude"
	"github.com/c1cada/NexusTok/relay/channel/cloudflare"
	"github.com/c1cada/NexusTok/relay/channel/codex"
	"github.com/c1cada/NexusTok/relay/channel/cohere"
	"github.com/c1cada/NexusTok/relay/channel/coze"
	"github.com/c1cada/NexusTok/relay/channel/deepseek"
	"github.com/c1cada/NexusTok/relay/channel/dify"
	"github.com/c1cada/NexusTok/relay/channel/gemini"
	"github.com/c1cada/NexusTok/relay/channel/jimeng"
	"github.com/c1cada/NexusTok/relay/channel/jina"
	"github.com/c1cada/NexusTok/relay/channel/minimax"
	"github.com/c1cada/NexusTok/relay/channel/mistral"
	"github.com/c1cada/NexusTok/relay/channel/mokaai"
	"github.com/c1cada/NexusTok/relay/channel/moonshot"
	"github.com/c1cada/NexusTok/relay/channel/ollama"
	"github.com/c1cada/NexusTok/relay/channel/openai"
	"github.com/c1cada/NexusTok/relay/channel/palm"
	"github.com/c1cada/NexusTok/relay/channel/perplexity"
	"github.com/c1cada/NexusTok/relay/channel/replicate"
	"github.com/c1cada/NexusTok/relay/channel/siliconflow"
	"github.com/c1cada/NexusTok/relay/channel/submodel"
	taskali "github.com/c1cada/NexusTok/relay/channel/task/ali"
	taskdoubao "github.com/c1cada/NexusTok/relay/channel/task/doubao"
	taskGemini "github.com/c1cada/NexusTok/relay/channel/task/gemini"
	"github.com/c1cada/NexusTok/relay/channel/task/hailuo"
	taskjimeng "github.com/c1cada/NexusTok/relay/channel/task/jimeng"
	"github.com/c1cada/NexusTok/relay/channel/task/kling"
	tasksora "github.com/c1cada/NexusTok/relay/channel/task/sora"
	"github.com/c1cada/NexusTok/relay/channel/task/suno"
	taskvertex "github.com/c1cada/NexusTok/relay/channel/task/vertex"
	taskVidu "github.com/c1cada/NexusTok/relay/channel/task/vidu"
	"github.com/c1cada/NexusTok/relay/channel/tencent"
	"github.com/c1cada/NexusTok/relay/channel/vertex"
	"github.com/c1cada/NexusTok/relay/channel/volcengine"
	"github.com/c1cada/NexusTok/relay/channel/xai"
	"github.com/c1cada/NexusTok/relay/channel/xunfei"
	"github.com/c1cada/NexusTok/relay/channel/zhipu"
	"github.com/c1cada/NexusTok/relay/channel/zhipu_4v"
	"github.com/gin-gonic/gin"
)

// GetAdaptor 根据 API 类型常量获取对应的上游渠道适配器。
// 该函数是渠道适配器的工厂方法，通过 switch-case 将 API 类型映射到具体的适配器实现。
//
// 参数：
//   - apiType: API 类型常量（如 constant.APITypeOpenAI、constant.APITypeAnthropic 等）
//
// 返回值：
//   - channel.Adaptor: 对应的渠道适配器实例；若 apiType 不匹配任何已知类型则返回 nil
func GetAdaptor(apiType int) channel.Adaptor {
	switch apiType {
	case constant.APITypeAli:
		return &ali.Adaptor{}
	case constant.APITypeAnthropic:
		return &claude.Adaptor{}
	case constant.APITypeBaidu:
		return &baidu.Adaptor{}
	case constant.APITypeGemini:
		return &gemini.Adaptor{}
	case constant.APITypeOpenAI:
		return &openai.Adaptor{}
	case constant.APITypePaLM:
		return &palm.Adaptor{}
	case constant.APITypeTencent:
		return &tencent.Adaptor{}
	case constant.APITypeXunfei:
		return &xunfei.Adaptor{}
	case constant.APITypeZhipu:
		return &zhipu.Adaptor{}
	case constant.APITypeZhipuV4:
		return &zhipu_4v.Adaptor{}
	case constant.APITypeOllama:
		return &ollama.Adaptor{}
	case constant.APITypePerplexity:
		return &perplexity.Adaptor{}
	case constant.APITypeAws:
		return &aws.Adaptor{}
	case constant.APITypeCohere:
		return &cohere.Adaptor{}
	case constant.APITypeDify:
		return &dify.Adaptor{}
	case constant.APITypeJina:
		return &jina.Adaptor{}
	case constant.APITypeCloudflare:
		return &cloudflare.Adaptor{}
	case constant.APITypeSiliconFlow:
		return &siliconflow.Adaptor{}
	case constant.APITypeVertexAi:
		return &vertex.Adaptor{}
	case constant.APITypeMistral:
		return &mistral.Adaptor{}
	case constant.APITypeDeepSeek:
		return &deepseek.Adaptor{}
	case constant.APITypeMokaAI:
		return &mokaai.Adaptor{}
	case constant.APITypeVolcEngine:
		return &volcengine.Adaptor{}
	case constant.APITypeBaiduV2:
		return &baidu_v2.Adaptor{}
	case constant.APITypeOpenRouter:
		return &openai.Adaptor{}
	case constant.APITypeXinference:
		return &openai.Adaptor{}
	case constant.APITypeXai:
		return &xai.Adaptor{}
	case constant.APITypeCoze:
		return &coze.Adaptor{}
	case constant.APITypeJimeng:
		return &jimeng.Adaptor{}
	case constant.APITypeMoonshot:
		return &moonshot.Adaptor{} // Moonshot uses Claude API
	case constant.APITypeSubmodel:
		return &submodel.Adaptor{}
	case constant.APITypeMiniMax:
		return &minimax.Adaptor{}
	case constant.APITypeReplicate:
		return &replicate.Adaptor{}
	case constant.APITypeCodex:
		return &codex.Adaptor{}
	case constant.APITypeAdvancedCustom:
		return &advancedcustom.Adaptor{}
	}
	return nil
}

// GetTaskPlatform 从 Gin 上下文中提取任务平台标识。
// 优先使用 channel_type 整数字段（转换为字符串），若不存在则使用 platform 字符串字段。
//
// 参数：
//   - c: Gin 请求上下文，包含渠道类型等信息
//
// 返回值：
//   - constant.TaskPlatform: 任务平台标识字符串
func GetTaskPlatform(c *gin.Context) constant.TaskPlatform {
	channelType := c.GetInt("channel_type")
	if channelType > 0 {
		return constant.TaskPlatform(strconv.Itoa(channelType))
	}
	return constant.TaskPlatform(c.GetString("platform"))
}

// GetTaskAdaptor 根据任务平台标识获取对应的异步任务适配器。
// 支持两种匹配方式：一是直接匹配字符串平台名（如 Suno），
// 二是将平台字符串解析为渠道类型整数后进行匹配（如阿里云、可灵、即梦等）。
//
// 参数：
//   - platform: 任务平台标识
//
// 返回值：
//   - channel.TaskAdaptor: 对应的任务适配器实例；若不匹配则返回 nil
func GetTaskAdaptor(platform constant.TaskPlatform) channel.TaskAdaptor {
	switch platform {
	//case constant.APITypeAIProxyLibrary:
	//	return &aiproxy.Adaptor{}
	case constant.TaskPlatformSuno:
		return &suno.TaskAdaptor{}
	}
	if channelType, err := strconv.ParseInt(string(platform), 10, 64); err == nil {
		switch channelType {
		case constant.ChannelTypeAli:
			return &taskali.TaskAdaptor{}
		case constant.ChannelTypeKling:
			return &kling.TaskAdaptor{}
		case constant.ChannelTypeJimeng:
			return &taskjimeng.TaskAdaptor{}
		case constant.ChannelTypeVertexAi:
			return &taskvertex.TaskAdaptor{}
		case constant.ChannelTypeVidu:
			return &taskVidu.TaskAdaptor{}
		case constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine:
			return &taskdoubao.TaskAdaptor{}
		case constant.ChannelTypeSora, constant.ChannelTypeOpenAI:
			return &tasksora.TaskAdaptor{}
		case constant.ChannelTypeGemini:
			return &taskGemini.TaskAdaptor{}
		case constant.ChannelTypeMiniMax:
			return &hailuo.TaskAdaptor{}
		}
	}
	return nil
}
