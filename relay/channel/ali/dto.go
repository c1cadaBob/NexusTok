// ali - dto.go
// 阿里云 DashScope 服务的数据传输对象（DTO）定义文件
// 本文件定义了与阿里云 DashScope API 交互所需的全部请求与响应结构体，
// 涵盖聊天补全（Chat）、文本向量嵌入（Embedding）、图像生成（Image）、
// 文本重排序（Rerank）等能力，并提供了阿里云响应格式到 OpenAI 兼容格式的转换方法。
package ali

import (
	"strings"

	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/service"
	"github.com/gin-gonic/gin"
)

// AliMessage 表示阿里云 DashScope 聊天接口中的单条消息。
// Content 字段为 any 类型，可承载纯文本字符串或包含图文混排的 AliMediaContent 数组。
// Role 字段标识消息角色，常见值为 "user"（用户）或 "assistant"（助手）。
type AliMessage struct {
	Content any    `json:"content"` // 消息内容，可以是字符串或 AliMediaContent 数组
	Role    string `json:"role"`    // 消息角色，如 "user"、"assistant"
}

// AliMediaContent 表示阿里云多模态消息中的单个内容块。
// 用于图文混排场景，每个内容块要么是图片（Image，支持 URL 或 Base64 编码），
// 要么是纯文本（Text）。
type AliMediaContent struct {
	Image string `json:"image,omitempty"` // 图片数据，支持 HTTP/HTTPS URL 或 data:...base64 格式
	Text  string `json:"text,omitempty"`  // 纯文本内容
}

// AliInput 表示阿里云聊天接口的输入结构。
// Prompt 字段用于简单文本提示场景，Messages 字段用于多轮对话或图文混排场景。
// 两者通常互斥使用。
type AliInput struct {
	Prompt string `json:"prompt,omitempty"` // 简单文本提示词（部分模型使用）
	//History []AliMessage `json:"history,omitempty"`  // 历史对话记录（已弃用，改用 Messages）
	Messages []AliMessage `json:"messages"` // 多轮对话消息列表
}

// AliParameters 表示阿里云聊天接口的采样与功能参数。
type AliParameters struct {
	TopP              float64 `json:"top_p,omitempty"`              // 核采样概率阈值，取值范围 (0,1]
	TopK              int     `json:"top_k,omitempty"`              // 从概率最高的 TopK 个 token 中采样
	Seed              uint64  `json:"seed,omitempty"`               // 随机数种子，用于结果复现
	EnableSearch      bool    `json:"enable_search,omitempty"`      // 是否启用联网搜索增强
	IncrementalOutput bool    `json:"incremental_output,omitempty"` // 是否启用增量流式输出（SSE 场景）
}

// AliChatRequest 表示发送给阿里云 DashScope 聊天接口的完整请求体。
type AliChatRequest struct {
	Model      string        `json:"model"`                // 模型名称，如 "qwen-turbo"、"qwen-plus"
	Input      AliInput      `json:"input,omitempty"`      // 输入内容（提示词或消息列表）
	Parameters AliParameters `json:"parameters,omitempty"` // 采样及功能参数
}

// AliEmbeddingRequest 表示发送给阿里云 DashScope 文本向量嵌入接口的请求体。
type AliEmbeddingRequest struct {
	Model string `json:"model"` // 嵌入模型名称，如 "text-embedding-v1"
	Input struct {
		Texts []string `json:"texts"` // 待向量化的文本列表，单次最多 25 条
	} `json:"input"` // 输入文本
	Parameters *struct {
		TextType string `json:"text_type,omitempty"` // 文本类型，可选 "query" 或 "document"
	} `json:"parameters,omitempty"` // 可选参数
}

// AliEmbedding 表示阿里云文本向量嵌入接口返回的单条嵌入结果。
type AliEmbedding struct {
	Embedding []float64 `json:"embedding"` // 文本的向量表示，维度取决于模型
	TextIndex int       `json:"text_index"` // 对应输入文本列表中的索引位置
}

// AliEmbeddingResponse 表示阿里云文本向量嵌入接口的完整响应体。
// 内嵌 AliError 用于统一错误处理。
type AliEmbeddingResponse struct {
	Output struct {
		Embeddings []AliEmbedding `json:"embeddings"` // 嵌入结果列表
	} `json:"output"` // 输出结果
	Usage AliUsage `json:"usage"` // Token 用量统计
	AliError                          // 内嵌错误信息
}

// AliError 表示阿里云 DashScope 接口返回的错误信息。
// 当请求失败时，响应体会包含 Code、Message 和 RequestId 字段。
type AliError struct {
	Code      string `json:"code"`       // 错误码，如 "InvalidApiKey"、"Throttling"
	Message   string `json:"message"`    // 错误描述信息
	RequestId string `json:"request_id"` // 请求的唯一标识，用于问题排查
}

// AliUsage 表示阿里云 DashScope 接口返回的 Token 用量统计信息。
type AliUsage struct {
	InputTokens  int `json:"input_tokens"`            // 输入 Token 数量
	OutputTokens int `json:"output_tokens"`           // 输出 Token 数量
	TotalTokens  int `json:"total_tokens"`            // Token 总数量
	ImageCount   int `json:"image_count,omitempty"`   // 生成的图片数量（图像生成场景）
}

// TaskResult 表示阿里云异步任务（如图像生成）返回的单个结果项。
// 对于图像生成任务，结果可以是 Base64 编码的图片数据或图片 URL。
type TaskResult struct {
	B64Image string `json:"b64_image,omitempty"` // Base64 编码的图片数据
	Url      string `json:"url,omitempty"`       // 图片的 HTTP/HTTPS 访问地址
	Code     string `json:"code,omitempty"`      // 错误码（任务失败时）
	Message  string `json:"message,omitempty"`   // 错误描述信息（任务失败时）
}

// AliOutput 表示阿里云 DashScope 接口的输出结构。
// 该结构体同时兼容聊天补全（text/choices）和图像生成（task_id/results）两种场景：
//   - 聊天补全：使用 Text、FinishReason、Choices 字段
//   - 图像生成（异步）：使用 TaskId、TaskStatus、Results 字段
//   - 图像生成（同步）：使用 Choices 字段，其中 Content 包含图文混排内容
type AliOutput struct {
	TaskId       string       `json:"task_id,omitempty"`       // 异步任务 ID，用于轮询任务状态
	TaskStatus   string       `json:"task_status,omitempty"`   // 任务状态，如 "PENDING"、"RUNNING"、"SUCCEEDED"、"FAILED"、"CANCELED"
	Text         string       `json:"text"`                    // 文本输出（部分模型直接返回文本）
	FinishReason string       `json:"finish_reason"`           // 生成结束原因，如 "stop"、"length"
	Message      string       `json:"message,omitempty"`       // 错误信息（任务失败时）
	Code         string       `json:"code,omitempty"`          // 错误码（任务失败时）
	Results      []TaskResult `json:"results,omitempty"`       // 异步任务的结果列表（图像生成）
	Choices      []struct {
		FinishReason string `json:"finish_reason,omitempty"` // 该选项的生成结束原因
		Message      struct {
			Role             string            `json:"role,omitempty"`              // 消息角色
			Content          []AliMediaContent `json:"content,omitempty"`           // 多模态内容列表（图文混排）
			ReasoningContent string            `json:"reasoning_content,omitempty"` // 推理链内容（思维链模型返回）
		} `json:"message,omitempty"` // 消息内容
	} `json:"choices,omitempty"` // 聊天补全的选项列表
}

// ChoicesToOpenAIImageDate 将阿里云同步图像生成的 Choices 响应转换为 OpenAI 兼容的 ImageData 切片。
// 参数：
//   - c: Gin 上下文，用于日志记录
//   - responseFormat: 响应格式，"b64_json" 表示需要 Base64 编码的图片，其他值仅返回 URL
//
// 转换逻辑：
//   - 遍历 Choices 中的每个 Content 块
//   - 如果 Content.Image 以 "http" 开头，视为图片 URL；当 responseFormat 为 "b64_json" 时，额外下载并编码为 Base64
//   - 如果 Content.Image 不以 "http" 开头，视为已经是 Base64 编码的图片数据
//   - 如果 Content.Text 非空，视为修订后的提示词（RevisedPrompt）
func (o *AliOutput) ChoicesToOpenAIImageDate(c *gin.Context, responseFormat string) []dto.ImageData {
	var imageData []dto.ImageData
	if len(o.Choices) > 0 {
		for _, choice := range o.Choices {
			var data dto.ImageData
			for _, content := range choice.Message.Content {
				if content.Image != "" {
					if strings.HasPrefix(content.Image, "http") {
						// 图片为 URL 格式
						var b64Json string
						if responseFormat == "b64_json" {
							// 客户端请求 Base64 格式，需要下载图片并编码
							_, b64, err := service.GetImageFromUrl(content.Image)
							if err != nil {
								logger.LogError(c, "get_image_data_failed: "+err.Error())
								continue
							}
							b64Json = b64
						}
						data.Url = content.Image
						data.B64Json = b64Json
					} else {
						// 图片已是 Base64 编码格式
						data.B64Json = content.Image
					}
				} else if content.Text != "" {
					// 非图片文本内容视为修订后的提示词
					data.RevisedPrompt = content.Text
				}
			}
			imageData = append(imageData, data)
		}
	}

	return imageData
}

// ResultToOpenAIImageDate 将阿里云异步图像生成的 Results 响应转换为 OpenAI 兼容的 ImageData 切片。
// 参数：
//   - c: Gin 上下文，用于日志记录
//   - responseFormat: 响应格式，"b64_json" 表示需要 Base64 编码的图片，其他值使用原始 B64Image
//
// 转换逻辑：
//   - 遍历 Results 中的每个 TaskResult
//   - 如果 responseFormat 为 "b64_json"，从 URL 下载图片并编码为 Base64
//   - 否则直接使用 TaskResult 中已有的 B64Image 字段
func (o *AliOutput) ResultToOpenAIImageDate(c *gin.Context, responseFormat string) []dto.ImageData {
	var imageData []dto.ImageData
	for _, data := range o.Results {
		var b64Json string
		if responseFormat == "b64_json" {
			// 从 URL 下载图片并转为 Base64
			_, b64, err := service.GetImageFromUrl(data.Url)
			if err != nil {
				logger.LogError(c, "get_image_data_failed: "+err.Error())
				continue
			}
			b64Json = b64
		} else {
			// 直接使用响应中已有的 Base64 图片数据
			b64Json = data.B64Image
		}

		imageData = append(imageData, dto.ImageData{
			Url:           data.Url,           // 图片访问 URL
			B64Json:       b64Json,            // Base64 编码的图片数据
			RevisedPrompt: "",                 // 异步任务无修订提示词
		})
	}
	return imageData
}

// AliResponse 表示阿里云 DashScope 接口的通用响应体。
// 适用于聊天补全和图像生成等场景，内嵌 AliError 用于统一错误处理。
type AliResponse struct {
	Output AliOutput `json:"output"` // 接口输出内容
	Usage  AliUsage  `json:"usage"`  // Token 用量统计
	AliError                          // 内嵌错误信息
}

// AliImageRequest 表示发送给阿里云 DashScope 图像生成接口的请求体。
// 同步和异步图像生成使用不同的 Input 格式，由 image.go 中的转换逻辑处理。
type AliImageRequest struct {
	Model          string             `json:"model"`                      // 图像模型名称，如 "wanx-v1"、"wanx2.1-t2i-turbo"
	Input          any                `json:"input"`                      // 输入内容，可以是 AliImageInput 或包含 Messages 的结构
	Parameters     AliImageParameters `json:"parameters,omitempty"`       // 图像生成参数
	ResponseFormat string             `json:"response_format,omitempty"`  // 响应格式，如 "b64_json"
}

// AliImageParameters 表示阿里云图像生成接口的参数配置。
// 包含图片尺寸、数量、步数、缩放比例、水印、提示词扩展等选项。
// 使用指针类型的 bool/int 字段（如 Watermark、PromptExtend、Seed）以区分"未设置"和"显式设为零值"。
type AliImageParameters struct {
	Size             string `json:"size,omitempty"`               // 图片尺寸，格式为 "宽*高"，如 "1024*1024"
	N                int    `json:"n,omitempty"`                  // 生成图片数量
	Steps            string `json:"steps,omitempty"`              // 生成步数（部分模型支持）
	Scale            string `json:"scale,omitempty"`              // 缩放比例（部分模型支持）
	Watermark        *bool  `json:"watermark,omitempty"`          // 是否添加水印
	PromptExtend     *bool  `json:"prompt_extend,omitempty"`      // 是否启用提示词扩展（开启后按2倍计费）
	ThinkingMode     *bool  `json:"thinking_mode,omitempty"`      // 是否启用思考模式（部分模型支持）
	EnableSequential *bool  `json:"enable_sequential,omitempty"`  // 是否启用顺序生成（部分模型支持）
	BboxList         any    `json:"bbox_list,omitempty"`          // 边界框列表（局部编辑场景）
	ColorPalette     any    `json:"color_palette,omitempty"`      // 调色板（颜色控制场景）
	Seed             *int   `json:"seed,omitempty"`               // 随机数种子，取值范围 [0, 2147483647]
}

// PromptExtendValue 返回 PromptExtend 参数的实际布尔值。
// 如果 PromptExtend 为 nil（未设置），返回 false。
// 如果 PromptExtend 非 nil，返回其指向的布尔值。
func (p *AliImageParameters) PromptExtendValue() bool {
	if p != nil && p.PromptExtend != nil {
		return *p.PromptExtend
	}
	return false
}

// AliImageInput 表示阿里云异步图像生成接口的输入结构。
// 用于异步图像生成任务，通过 Prompt 直接提供文本提示词。
type AliImageInput struct {
	Prompt         string       `json:"prompt,omitempty"`          // 文本提示词，描述期望生成的图像内容
	NegativePrompt string       `json:"negative_prompt,omitempty"` // 反向提示词，描述不希望出现的内容
	Messages       []AliMessage `json:"messages,omitempty"`        // 消息列表（同步图像模型使用）
}

// WanImageInput 表示阿里云万相（Wanx）图像编辑接口的输入结构。
// 支持基于参考图片和文本提示词进行图像编辑/风格迁移。
type WanImageInput struct {
	Prompt         string   `json:"prompt"`                    // 必需：文本提示词，描述生成图像中期望包含的元素和视觉特点
	Images         []string `json:"images"`                    // 必需：图像URL数组，长度不超过2，支持HTTP/HTTPS URL或Base64编码
	NegativePrompt string   `json:"negative_prompt,omitempty"` // 可选：反向提示词，描述不希望在画面中看到的内容
}

// WanImageParameters 表示阿里云万相图像编辑接口的参数配置。
type WanImageParameters struct {
	N         int     `json:"n,omitempty"`         // 生成图片数量，取值范围1-4，默认4
	Watermark *bool   `json:"watermark,omitempty"` // 是否添加水印标识，默认false
	Seed      int     `json:"seed,omitempty"`      // 随机数种子，取值范围[0, 2147483647]
	Strength  float64 `json:"strength,omitempty"`  // 修改幅度 0.0-1.0，默认0.5（部分模型支持）
}

// AliRerankParameters 表示阿里云 DashScope 文本重排序接口的参数配置。
type AliRerankParameters struct {
	TopN            *int  `json:"top_n,omitempty"`            // 返回排名最高的前 N 个文档
	ReturnDocuments *bool `json:"return_documents,omitempty"` // 是否在结果中返回原始文档内容
}

// AliRerankInput 表示阿里云 DashScope 文本重排序接口的输入结构。
type AliRerankInput struct {
	Query     string `json:"query"`     // 查询文本
	Documents []any  `json:"documents"` // 待排序的文档列表，元素可以是字符串或文档对象
}

// AliRerankRequest 表示发送给阿里云 DashScope 文本重排序接口的完整请求体。
type AliRerankRequest struct {
	Model      string              `json:"model"`                // 重排序模型名称，如 "gte-rerank"
	Input      AliRerankInput      `json:"input"`                // 输入内容（查询和文档）
	Parameters AliRerankParameters `json:"parameters,omitempty"` // 重排序参数
}

// AliRerankResponse 表示阿里云 DashScope 文本重排序接口的完整响应体。
// 内嵌 AliError 用于统一错误处理。
type AliRerankResponse struct {
	Output struct {
		Results []dto.RerankResponseResult `json:"results"` // 排序结果列表
	} `json:"output"` // 输出结果
	Usage     AliUsage `json:"usage"`      // Token 用量统计
	RequestId string   `json:"request_id"` // 请求的唯一标识
	AliError                           // 内嵌错误信息
}
