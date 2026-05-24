// dify - relay-dify.go
// 本文件实现了 Dify 渠道的核心中继逻辑。
// 包含将 OpenAI 请求转换为 Dify 格式、将 Dify 响应转换回 OpenAI 格式的处理函数。
// 支持文件上传（图片）、流式和非流式聊天响应，以及工作流调试信息的透传。
// Dify 是一个开源的 LLM 应用开发平台，其 API 格式与 OpenAI 不同，
// 需要通过适配层进行格式转换。
package dify

// 标准库导入
import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"                    // 公共工具函数
	"github.com/c1cada/NexusTok/constant"                   // 全局常量
	"github.com/c1cada/NexusTok/dto"                        // 数据传输对象
	relaycommon "github.com/c1cada/NexusTok/relay/common"   // relay 层公共工具
	"github.com/c1cada/NexusTok/relay/helper"               // relay 辅助函数
	"github.com/c1cada/NexusTok/service"                    // 服务层
	"github.com/c1cada/NexusTok/types"                      // 类型定义
	"github.com/samber/lo"                                   // Go 工具库

	// 第三方依赖
	"github.com/gin-gonic/gin"
)

// uploadDifyFile 将图片文件上传到 Dify 文件上传接口并返回文件引用。
// 支持 base64 编码的图片数据，解码后通过 multipart/form-data 上传。
// 上传流程：
//  1. 解析 base64 编码的图片数据，移除 data URI 前缀
//  2. 将解码后的数据写入临时文件
//  3. 构建 multipart/form-data 请求体
//  4. 发送 POST 请求到 Dify 文件上传接口（/v1/files/upload）
//  5. 解析响应获取文件 ID
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息（用于获取 API Key 和 Base URL）
//   - user: 用户标识
//   - media: 媒体内容（包含 base64 图片数据和 MIME 类型）
//
// 返回值：DifyFile 文件引用指针，上传失败时返回 nil。
func uploadDifyFile(c *gin.Context, info *relaycommon.RelayInfo, user string, media dto.MediaContent) *DifyFile {
	uploadUrl := fmt.Sprintf("%s/v1/files/upload", info.ChannelBaseUrl)
	switch media.Type {
	case dto.ContentTypeImageURL:
		// 解码 base64 图片数据
		imageMedia := media.GetImageMedia()
		base64Data := imageMedia.Url
		// 移除 base64 前缀（如 "data:image/jpeg;base64,"）
		if idx := strings.Index(base64Data, ","); idx != -1 {
			base64Data = base64Data[idx+1:]
		}

		// 解码 base64 字符串
		decodedData, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			common.SysLog("failed to decode base64: " + err.Error())
			return nil
		}

		// 创建临时文件存储解码后的图片数据
		tempFile, err := os.CreateTemp("", "dify-upload-*")
		if err != nil {
			common.SysLog("failed to create temp file: " + err.Error())
			return nil
		}
		defer tempFile.Close()
		defer os.Remove(tempFile.Name()) // 用完后删除临时文件

		// 写入解码数据到临时文件
		if _, err := tempFile.Write(decodedData); err != nil {
			common.SysLog("failed to write to temp file: " + err.Error())
			return nil
		}

		// 创建 multipart form 数据
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		// 添加 user 字段
		if err := writer.WriteField("user", user); err != nil {
			common.SysLog("failed to add user field: " + err.Error())
			return nil
		}

		// 确定 MIME 类型，默认为 image/jpeg
		mimeType := imageMedia.MimeType
		if mimeType == "" {
			mimeType = "image/jpeg"
		}

		// 创建 form 文件字段
		part, err := writer.CreateFormFile("file", fmt.Sprintf("image.%s", strings.TrimPrefix(mimeType, "image/")))
		if err != nil {
			common.SysLog("failed to create form file: " + err.Error())
			return nil
		}

		// 将图片数据写入 form 文件
		if _, err = io.Copy(part, bytes.NewReader(decodedData)); err != nil {
			common.SysLog("failed to copy file content: " + err.Error())
			return nil
		}
		writer.Close()

		// 创建 HTTP 上传请求
		req, err := http.NewRequest("POST", uploadUrl, body)
		if err != nil {
			common.SysLog("failed to create request: " + err.Error())
			return nil
		}

		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))

		// 发送上传请求
		client := service.GetHttpClient()
		resp, err := client.Do(req)
		if err != nil {
			common.SysLog("failed to send request: " + err.Error())
			return nil
		}
		defer resp.Body.Close()

		// 解析上传响应，获取文件 ID
		var result struct {
			Id string `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			common.SysLog("failed to decode response: " + err.Error())
			return nil
		}

		// 返回文件引用
		return &DifyFile{
			UploadFileId: result.Id,
			Type:         "image",
			TransferMode: "local_file",
		}
	}
	return nil
}

// requestOpenAI2Dify 将 OpenAI 格式的聊天请求转换为 Dify 格式。
// 转换规则：
//   - 将 OpenAI 的 messages 数组合并为单个 query 文本
//   - system 消息添加 "SYSTEM:" 前缀
//   - assistant 消息添加 "ASSISTANT:" 前缀
//   - user 消息中的文本添加 "USER:" 前缀
//   - user 消息中的图片通过 uploadDifyFile 上传或使用远程 URL
//   - 根据 stream 参数设置 response_mode 为 "streaming" 或 "blocking"
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - request: OpenAI 格式的请求体
//
// 返回值：转换后的 Dify 请求体指针。
func requestOpenAI2Dify(c *gin.Context, info *relaycommon.RelayInfo, request dto.GeneralOpenAIRequest) *DifyChatRequest {
	difyReq := DifyChatRequest{
		Inputs:           make(map[string]interface{}),
		AutoGenerateName: false,
	}

	// 解析用户标识
	user := request.User
	if len(user) == 0 {
		user = json.RawMessage(helper.GetResponseID(c))
	}
	var stringUser string
	err := json.Unmarshal(user, &stringUser)
	if err != nil {
		common.SysLog("failed to unmarshal user: " + err.Error())
		stringUser = helper.GetResponseID(c)
	}
	difyReq.User = stringUser

	// 遍历消息列表，构建 query 和文件列表
	files := make([]DifyFile, 0)
	var content strings.Builder
	for _, message := range request.Messages {
		if message.Role == "system" {
			// 系统消息添加 SYSTEM: 前缀
			content.WriteString("SYSTEM: \n" + message.StringContent() + "\n")
		} else if message.Role == "assistant" {
			// 助手消息添加 ASSISTANT: 前缀
			content.WriteString("ASSISTANT: \n" + message.StringContent() + "\n")
		} else {
			// 用户消息，解析多模态内容
			parseContent := message.ParseContent()
			for _, mediaContent := range parseContent {
				switch mediaContent.Type {
				case dto.ContentTypeText:
					// 文本内容添加 USER: 前缀
					content.WriteString("USER: \n" + mediaContent.Text + "\n")
				case dto.ContentTypeImageURL:
					// 图片内容，根据是否为远程图片选择不同的处理方式
					media := mediaContent.GetImageMedia()
					var file *DifyFile
					if media.IsRemoteImage() {
						// 远程图片使用 URL 引用
						file.Type = media.MimeType
						file.TransferMode = "remote_url"
						file.URL = media.Url
					} else {
						// 本地 base64 图片需要上传到 Dify
						file = uploadDifyFile(c, info, difyReq.User, mediaContent)
					}
					if file != nil {
						files = append(files, *file)
					}
				}
			}
		}
	}
	difyReq.Query = content.String()
	difyReq.Files = files
	// 设置响应模式
	mode := "blocking"
	if lo.FromPtrOr(request.Stream, false) {
		mode = "streaming"
	}
	difyReq.ResponseMode = mode
	return &difyReq
}

// streamResponseDify2OpenAI 将 Dify 流式响应事件转换为 OpenAI 兼容格式。
// 处理逻辑：
//   - workflow_* 事件：调试模式下输出工作流信息到 reasoning content
//   - node_* 事件：调试模式下输出节点信息到 reasoning content
//   - message/agent_message 事件：将文本内容输出到 content 字段
//   - 特殊处理 Dify 的思考标签（将 HTML 的 <details> 标签转换为 <think> 标签）
//
// 参数：difyResponse - Dify 流式响应事件。
// 返回值：OpenAI 格式的流式响应指针。
func streamResponseDify2OpenAI(difyResponse DifyChunkChatCompletionResponse) *dto.ChatCompletionsStreamResponse {
	response := dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "dify",
	}
	var choice dto.ChatCompletionsStreamResponseChoice
	if strings.HasPrefix(difyResponse.Event, "workflow_") {
		// 工作流事件，仅在调试模式下输出
		if constant.DifyDebug {
			text := "Workflow: " + difyResponse.Data.WorkflowId
			if difyResponse.Event == "workflow_finished" {
				text += " " + difyResponse.Data.Status
			}
			choice.Delta.SetReasoningContent(text + "\n")
		}
	} else if strings.HasPrefix(difyResponse.Event, "node_") {
		// 节点事件，仅在调试模式下输出
		if constant.DifyDebug {
			text := "Node: " + difyResponse.Data.NodeType
			if difyResponse.Event == "node_finished" {
				text += " " + difyResponse.Data.Status
			}
			choice.Delta.SetReasoningContent(text + "\n")
		}
	} else if difyResponse.Event == "message" || difyResponse.Event == "agent_message" {
		// 消息事件，处理 Dify 特有的思考标签格式
		// Dify 使用 HTML <details> 标签表示思考过程，需要转换为 <think> 标签
		if difyResponse.Answer == "<details style=\"color:gray;background-color: #f8f8f8;padding: 8px;border-radius: 4px;\" open> <summary> Thinking... </summary>\n" {
			difyResponse.Answer = "<think>"
		} else if difyResponse.Answer == "</details>" {
			difyResponse.Answer = "</think>"
		}

		choice.Delta.SetContentString(difyResponse.Answer)
	}
	response.Choices = append(response.Choices, choice)
	return &response
}

// difyStreamHandler 处理 Dify 流式聊天响应。
// 将 Dify 的 SSE 事件流转换为 OpenAI 兼容的流式格式。
// 主要流程：
//  1. 设置 SSE 响应头
//  2. 使用 StreamScannerHandler 逐事件处理
//  3. 对于 message_end 事件，提取用量信息并结束
//  4. 对于 error 事件，终止流并返回错误
//  5. 其他事件转换为 OpenAI 格式并发送到客户端
//  6. 如果上游未返回 token 用量，使用估算值
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - resp: 上游 HTTP 响应
//
// 返回值：usage 用量信息和可能的错误。
func difyStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	var responseText string
	usage := &dto.Usage{}
	var nodeToken int // 工作流/节点事件的 token 计数
	helper.SetEventStreamHeaders(c)
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var difyResponse DifyChunkChatCompletionResponse
		if err := json.Unmarshal([]byte(data), &difyResponse); err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			sr.Error(err)
			return
		}
		if difyResponse.Event == "message_end" {
			// 流结束事件，提取用量信息
			usage = &difyResponse.MetaData.Usage
			sr.Done()
			return
		} else if difyResponse.Event == "error" {
			// 错误事件，终止流
			sr.Stop(fmt.Errorf("dify error event"))
			return
		}
		// 转换为 OpenAI 格式并发送
		openaiResponse := *streamResponseDify2OpenAI(difyResponse)
		if len(openaiResponse.Choices) != 0 {
			responseText += openaiResponse.Choices[0].Delta.GetContentString()
			if openaiResponse.Choices[0].Delta.ReasoningContent != nil {
				nodeToken += 1 // 调试信息也计入 token
			}
		}
		if err := helper.ObjectData(c, openaiResponse); err != nil {
			common.SysLog(err.Error())
			sr.Error(err)
		}
	})
	helper.Done(c)
	// 如果上游未返回 token 用量，使用估算值
	if usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens())
	}
	usage.CompletionTokens += nodeToken // 加上工作流/节点事件的 token
	return usage, nil
}

// difyHandler 处理 Dify 非流式聊天响应。
// 将 Dify 的完整 JSON 响应转换为 OpenAI TextResponse 格式并写入客户端。
// 处理流程：
//  1. 读取并解析 DifyChatCompletionResponse 响应体
//  2. 构建 OpenAI 格式的 TextResponse（包含 id、object、created、usage、choices）
//  3. 将响应序列化为 JSON 并写入客户端
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - resp: 上游 HTTP 响应
//
// 返回值：usage 用量信息和可能的错误。
func difyHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	var difyResponse DifyChatCompletionResponse
	responseBody, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.CloseResponseBodyGracefully(resp) // 优雅关闭响应体
	err = json.Unmarshal(responseBody, &difyResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	// 构建 OpenAI 格式的响应
	fullTextResponse := dto.OpenAITextResponse{
		Id:      difyResponse.ConversationId,
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Usage:   difyResponse.MetaData.Usage,
	}
	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role:    "assistant",
			Content: difyResponse.Answer,
		},
		FinishReason: "stop",
	}
	fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	// 写入响应
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	c.Writer.Write(jsonResponse)
	return &difyResponse.MetaData.Usage, nil
}
