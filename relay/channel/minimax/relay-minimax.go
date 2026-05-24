// MiniMax 请求 URL 构建文件。
// 负责根据不同的 RelayMode 和 RelayFormat 构建对应的 MiniMax API 请求地址。
// 支持对话补全、图片生成、语音合成和 Claude 格式等模式。
package minimax

// 标准库导入
import (
	"fmt" // 格式化字符串

	// 项目内部依赖
	channelconstant "github.com/c1cada/NexusTok/constant"       // 渠道常量定义
	relaycommon "github.com/c1cada/NexusTok/relay/common"        // Relay 通用模块
	"github.com/c1cada/NexusTok/relay/constant"                 // Relay 常量定义
	"github.com/c1cada/NexusTok/types"                          // 公共类型定义
)

// GetRequestURL 根据 RelayMode 和 RelayFormat 构建 MiniMax API 的请求 URL。
// 支持的模式：
//   - Claude 格式: {baseUrl}/anthropic/v1/messages
//   - 对话补全: {baseUrl}/v1/text/chatcompletion_v2
//   - 图片生成: {baseUrl}/v1/image_generation
//   - 语音合成: {baseUrl}/v1/t2a_v2
//
// 当渠道基础 URL 为空时，使用全局配置的默认 URL。
// 参数:
//   - info: Relay 信息，包含基础 URL、请求模式和格式
// 返回:
//   - string: 完整的 API 请求 URL
//   - error: 不支持的模式时返回错误
func GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseUrl := info.ChannelBaseUrl
	if baseUrl == "" {
		baseUrl = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeMiniMax]
	}
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return fmt.Sprintf("%s/anthropic/v1/messages", info.ChannelBaseUrl), nil
	default:
		switch info.RelayMode {
		case constant.RelayModeChatCompletions:
			return fmt.Sprintf("%s/v1/text/chatcompletion_v2", baseUrl), nil
		case constant.RelayModeImagesGenerations:
			return fmt.Sprintf("%s/v1/image_generation", baseUrl), nil
		case constant.RelayModeAudioSpeech:
			return fmt.Sprintf("%s/v1/t2a_v2", baseUrl), nil
		default:
			return "", fmt.Errorf("unsupported relay mode: %d", info.RelayMode)
		}
	}
}
