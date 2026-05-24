// ollama - relay-ollama.go
// Ollama 渠道的请求转换与响应处理文件。
// 本文件负责：
// - 将 OpenAI 格式的请求转换为 Ollama 原生格式（聊天、生成、嵌入）
// - 处理 Ollama 嵌入 API 的响应并转换为 OpenAI 格式
// - 提供 Ollama 模型管理功能（列出、拉取、删除、查询版本）
package ollama

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// openAIChatToOllamaChat 将 OpenAI 格式的聊天请求转换为 Ollama 聊天请求格式。
// 该函数执行以下转换工作：
//  1. 基础字段映射：model、stream、think
//  2. 响应格式处理：支持 json 和 json_schema 两种格式
//  3. 参数映射：将 OpenAI 的采样参数（temperature、top_p、top_k 等）映射到 Ollama 的 options 字段
//  4. 停止词处理：支持 string、[]string、[]any 三种停止词格式
//  5. 工具定义转换：将 OpenAI 的 function calling 工具定义转换为 Ollama 格式
//  6. 消息转换：
//     - 处理文本内容和多模态内容（图片）
//     - 将图片 URL 转换为 base64 编码
//     - 处理工具调用（tool_calls）和工具响应（tool message）
//
// 参数:
//   - c: Gin 上下文，用于获取请求信息和调用服务
//   - r: OpenAI 格式的通用请求对象
//
// 返回:
//   - *OllamaChatRequest: 转换后的 Ollama 聊天请求
//   - error: 转换过程中的错误（如图片获取失败）
func openAIChatToOllamaChat(c *gin.Context, r *dto.GeneralOpenAIRequest) (*OllamaChatRequest, error) {
	chatReq := &OllamaChatRequest{
		Model:   r.Model,
		Stream:  lo.FromPtrOr(r.Stream, false),
		Options: map[string]any{},
		Think:   r.Think,
	}
	if r.ResponseFormat != nil {
		if r.ResponseFormat.Type == "json" {
			chatReq.Format = "json"
		} else if r.ResponseFormat.Type == "json_schema" {
			if len(r.ResponseFormat.JsonSchema) > 0 {
				var schema any
				_ = json.Unmarshal(r.ResponseFormat.JsonSchema, &schema)
				chatReq.Format = schema
			}
		}
	}

	// options mapping
	if r.Temperature != nil {
		chatReq.Options["temperature"] = r.Temperature
	}
	if r.TopP != nil {
		chatReq.Options["top_p"] = lo.FromPtr(r.TopP)
	}
	if r.TopK != nil {
		chatReq.Options["top_k"] = lo.FromPtr(r.TopK)
	}
	if r.FrequencyPenalty != nil {
		chatReq.Options["frequency_penalty"] = lo.FromPtr(r.FrequencyPenalty)
	}
	if r.PresencePenalty != nil {
		chatReq.Options["presence_penalty"] = lo.FromPtr(r.PresencePenalty)
	}
	if r.Seed != nil {
		chatReq.Options["seed"] = int(lo.FromPtr(r.Seed))
	}
	if mt := r.GetMaxTokens(); mt != 0 {
		chatReq.Options["num_predict"] = int(mt)
	}

	if r.Stop != nil {
		switch v := r.Stop.(type) {
		case string:
			chatReq.Options["stop"] = []string{v}
		case []string:
			chatReq.Options["stop"] = v
		case []any:
			arr := make([]string, 0, len(v))
			for _, i := range v {
				if s, ok := i.(string); ok {
					arr = append(arr, s)
				}
			}
			if len(arr) > 0 {
				chatReq.Options["stop"] = arr
			}
		}
	}

	if len(r.Tools) > 0 {
		tools := make([]OllamaTool, 0, len(r.Tools))
		for _, t := range r.Tools {
			tools = append(tools, OllamaTool{Type: "function", Function: OllamaToolFunction{Name: t.Function.Name, Description: t.Function.Description, Parameters: t.Function.Parameters}})
		}
		chatReq.Tools = tools
	}

	chatReq.Messages = make([]OllamaChatMessage, 0, len(r.Messages))
	for _, m := range r.Messages {
		var textBuilder strings.Builder
		var images []string
		if m.IsStringContent() {
			textBuilder.WriteString(m.StringContent())
		} else {
			parts := m.ParseContent()
			for _, part := range parts {
				if part.Type == dto.ContentTypeImageURL {
					source := part.ToFileSource()
					if source != nil {
						base64Data, _, err := service.GetBase64Data(c, source, "fetch image for ollama chat")
						if err != nil {
							return nil, err
						}
						if base64Data != "" {
							images = append(images, base64Data)
						}
					}
				} else if part.Type == dto.ContentTypeText {
					textBuilder.WriteString(part.Text)
				}
			}
		}
		cm := OllamaChatMessage{Role: m.Role, Content: textBuilder.String()}
		if len(images) > 0 {
			cm.Images = images
		}
		if m.Role == "tool" && m.Name != nil {
			cm.ToolName = *m.Name
		}
		if m.ToolCalls != nil && len(m.ToolCalls) > 0 {
			parsed := m.ParseToolCalls()
			if len(parsed) > 0 {
				calls := make([]OllamaToolCall, 0, len(parsed))
				for _, tc := range parsed {
					var args interface{}
					if tc.Function.Arguments != "" {
						_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
					}
					if args == nil {
						args = map[string]any{}
					}
					oc := OllamaToolCall{}
					oc.Function.Name = tc.Function.Name
					oc.Function.Arguments = args
					calls = append(calls, oc)
				}
				cm.ToolCalls = calls
			}
		}
		chatReq.Messages = append(chatReq.Messages, cm)
	}
	return chatReq, nil
}

// openAIToGenerate 将 OpenAI 格式的补全请求（completions）转换为 Ollama 生成请求格式。
// 与聊天请求不同，生成请求使用 prompt 字段而非 messages 数组。
// 该函数处理以下转换：
//   - prompt 字段：支持 string 和 []any 两种格式，[]any 会被拼接为字符串
//   - suffix 字段：用于代码补全场景的后缀文本
//   - 响应格式：支持 json 和 json_schema 格式
//   - 采样参数：temperature、top_p、top_k、frequency_penalty、presence_penalty、seed 等
//   - 停止词：支持 string、[]string、[]any 格式
//   - 最大 token 数：映射到 Ollama 的 num_predict 参数
//
// 参数:
//   - c: Gin 上下文
//   - r: OpenAI 格式的通用请求对象
//
// 返回:
//   - *OllamaGenerateRequest: 转换后的 Ollama 生成请求
//   - error: 转换过程中的错误
func openAIToGenerate(c *gin.Context, r *dto.GeneralOpenAIRequest) (*OllamaGenerateRequest, error) {
	gen := &OllamaGenerateRequest{
		Model:   r.Model,
		Stream:  lo.FromPtrOr(r.Stream, false),
		Options: map[string]any{},
		Think:   r.Think,
	}
	// Prompt may be in r.Prompt (string or []any)
	if r.Prompt != nil {
		switch v := r.Prompt.(type) {
		case string:
			gen.Prompt = v
		case []any:
			var sb strings.Builder
			for _, it := range v {
				if s, ok := it.(string); ok {
					sb.WriteString(s)
				}
			}
			gen.Prompt = sb.String()
		default:
			gen.Prompt = fmt.Sprintf("%v", r.Prompt)
		}
	}
	if r.Suffix != nil {
		if s, ok := r.Suffix.(string); ok {
			gen.Suffix = s
		}
	}
	if r.ResponseFormat != nil {
		if r.ResponseFormat.Type == "json" {
			gen.Format = "json"
		} else if r.ResponseFormat.Type == "json_schema" {
			var schema any
			_ = json.Unmarshal(r.ResponseFormat.JsonSchema, &schema)
			gen.Format = schema
		}
	}
	if r.Temperature != nil {
		gen.Options["temperature"] = r.Temperature
	}
	if r.TopP != nil {
		gen.Options["top_p"] = lo.FromPtr(r.TopP)
	}
	if r.TopK != nil {
		gen.Options["top_k"] = lo.FromPtr(r.TopK)
	}
	if r.FrequencyPenalty != nil {
		gen.Options["frequency_penalty"] = lo.FromPtr(r.FrequencyPenalty)
	}
	if r.PresencePenalty != nil {
		gen.Options["presence_penalty"] = lo.FromPtr(r.PresencePenalty)
	}
	if r.Seed != nil {
		gen.Options["seed"] = int(lo.FromPtr(r.Seed))
	}
	if mt := r.GetMaxTokens(); mt != 0 {
		gen.Options["num_predict"] = int(mt)
	}
	if r.Stop != nil {
		switch v := r.Stop.(type) {
		case string:
			gen.Options["stop"] = []string{v}
		case []string:
			gen.Options["stop"] = v
		case []any:
			arr := make([]string, 0, len(v))
			for _, i := range v {
				if s, ok := i.(string); ok {
					arr = append(arr, s)
				}
			}
			if len(arr) > 0 {
				gen.Options["stop"] = arr
			}
		}
	}
	return gen, nil
}

// requestOpenAI2Embeddings 将 OpenAI 格式的嵌入请求转换为 Ollama 嵌入请求格式。
// 该函数处理以下转换：
//   - 采样参数映射：temperature、top_p、frequency_penalty、presence_penalty、seed
//   - 向量维度：如果请求中指定了 dimensions，会同时映射到 options 和顶层字段
//   - 输入文本处理：当输入只有一个元素时，直接使用字符串而非数组，以兼容 Ollama 的单输入格式
//
// 参数:
//   - r: OpenAI 格式的嵌入请求对象
//
// 返回:
//   - *OllamaEmbeddingRequest: 转换后的 Ollama 嵌入请求
func requestOpenAI2Embeddings(r dto.EmbeddingRequest) *OllamaEmbeddingRequest {
	opts := map[string]any{}
	if r.Temperature != nil {
		opts["temperature"] = r.Temperature
	}
	if r.TopP != nil {
		opts["top_p"] = lo.FromPtr(r.TopP)
	}
	if r.FrequencyPenalty != nil {
		opts["frequency_penalty"] = lo.FromPtr(r.FrequencyPenalty)
	}
	if r.PresencePenalty != nil {
		opts["presence_penalty"] = lo.FromPtr(r.PresencePenalty)
	}
	if r.Seed != nil {
		opts["seed"] = int(lo.FromPtr(r.Seed))
	}
	dimensions := lo.FromPtrOr(r.Dimensions, 0)
	if r.Dimensions != nil {
		opts["dimensions"] = dimensions
	}
	input := r.ParseInput()
	if len(input) == 1 {
		return &OllamaEmbeddingRequest{Model: r.Model, Input: input[0], Options: opts, Dimensions: dimensions}
	}
	return &OllamaEmbeddingRequest{Model: r.Model, Input: input, Options: opts, Dimensions: dimensions}
}

// ollamaEmbeddingHandler 处理 Ollama 嵌入 API 的响应，将其转换为 OpenAI 格式的嵌入响应。
// 该函数执行以下操作：
//  1. 读取并解析 Ollama 的嵌入响应体
//  2. 检查响应中是否包含错误信息
//  3. 将 Ollama 的嵌入向量列表转换为 OpenAI 格式的 OpenAIEmbeddingResponseItem 数组
//  4. 从 Ollama 响应中提取 token 使用量信息（prompt_eval_count）
//  5. 构建 OpenAI 格式的嵌入响应并写回客户端
//
// 参数:
//   - c: Gin 上下文，用于写入响应
//   - info: 中继信息，包含上游模型名称等上下文
//   - resp: Ollama 返回的 HTTP 响应对象
//
// 返回:
//   - *dto.Usage: token 使用量信息
//   - *types.NexusTokError: 错误信息（如果发生错误）
func ollamaEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	var oResp OllamaEmbeddingResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	if err = common.Unmarshal(body, &oResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oResp.Error != "" {
		return nil, types.NewOpenAIError(fmt.Errorf("ollama error: %s", oResp.Error), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	data := make([]dto.OpenAIEmbeddingResponseItem, 0, len(oResp.Embeddings))
	for i, emb := range oResp.Embeddings {
		data = append(data, dto.OpenAIEmbeddingResponseItem{Index: i, Object: "embedding", Embedding: emb})
	}
	usage := &dto.Usage{PromptTokens: oResp.PromptEvalCount, CompletionTokens: 0, TotalTokens: oResp.PromptEvalCount}
	embResp := &dto.OpenAIEmbeddingResponse{Object: "list", Data: data, Model: info.UpstreamModelName, Usage: *usage}
	out, _ := common.Marshal(embResp)
	service.IOCopyBytesGracefully(c, resp, out)
	return usage, nil
}

// FetchOllamaModels 从 Ollama 服务器获取已安装的模型列表。
// 通过调用 Ollama 的 /api/tags 端点获取所有已安装模型的信息。
// Ollama 通常不需要 Bearer token 认证，但函数保留了 apiKey 参数以兼容需要认证的场景。
//
// 参数:
//   - baseURL: Ollama 服务器的基础 URL（如 http://localhost:11434）
//   - apiKey: API 密钥（可选，为空则不发送认证头）
//
// 返回:
//   - []OllamaModel: 已安装的模型列表，包含模型名称、大小、格式等详细信息
//   - error: 请求失败或解析错误
func FetchOllamaModels(baseURL, apiKey string) ([]OllamaModel, error) {
	url := fmt.Sprintf("%s/api/tags", baseURL)

	client := &http.Client{}
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// Ollama 通常不需要 Bearer token，但为了兼容性保留
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("服务器返回错误 %d: %s", response.StatusCode, string(body))
	}

	var tagsResponse OllamaTagsResponse
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	err = common.Unmarshal(body, &tagsResponse)
	if err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	return tagsResponse.Models, nil
}

// PullOllamaModel 向 Ollama 服务器发起模型拉取请求（非流式模式）。
// 通过调用 Ollama 的 /api/pull 端点下载指定模型。
// 使用 30 分钟超时以支持大型模型的下载。
//
// 参数:
//   - baseURL: Ollama 服务器的基础 URL
//   - apiKey: API 密钥（可选）
//   - modelName: 要拉取的模型名称（如 "llama3:7b"）
//
// 返回:
//   - error: 拉取失败时返回错误信息
func PullOllamaModel(baseURL, apiKey, modelName string) error {
	url := fmt.Sprintf("%s/api/pull", baseURL)

	pullRequest := OllamaPullRequest{
		Name:   modelName,
		Stream: false, // 非流式，简化处理
	}

	requestBody, err := common.Marshal(pullRequest)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	client := &http.Client{
		Timeout: 30 * 60 * 1000 * time.Millisecond, // 30分钟超时，支持大模型
	}
	request, err := http.NewRequest("POST", url, strings.NewReader(string(requestBody)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("拉取模型失败 %d: %s", response.StatusCode, string(body))
	}

	return nil
}

// PullOllamaModelStream 以流式模式向 Ollama 服务器发起模型拉取请求。
// 通过调用 Ollama 的 /api/pull 端点下载指定模型，启用流式响应以获取实时进度。
// 使用 1 小时超时以支持超大型模型的下载。
// 通过 progressCallback 回调函数实时报告拉取进度（包含状态、已完成大小、总大小等信息）。
//
// 流式响应以 NDJSON（换行分隔的 JSON）格式返回，每行包含一个 OllamaPullResponse 对象。
// 当状态为 "success" 时表示拉取完成，"error" 时表示拉取失败。
//
// 参数:
//   - baseURL: Ollama 服务器的基础 URL
//   - apiKey: API 密钥（可选）
//   - modelName: 要拉取的模型名称
//   - progressCallback: 进度回调函数，每收到一行流式数据时被调用（可为 nil）
//
// 返回:
//   - error: 拉取失败、流读取错误或未收到成功状态时返回错误
func PullOllamaModelStream(baseURL, apiKey, modelName string, progressCallback func(OllamaPullResponse)) error {
	url := fmt.Sprintf("%s/api/pull", baseURL)

	pullRequest := OllamaPullRequest{
		Name:   modelName,
		Stream: true, // 启用流式
	}

	requestBody, err := common.Marshal(pullRequest)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	client := &http.Client{
		Timeout: 60 * 60 * 1000 * time.Millisecond, // 1小时超时，支持超大模型
	}
	request, err := http.NewRequest("POST", url, strings.NewReader(string(requestBody)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("拉取模型失败 %d: %s", response.StatusCode, string(body))
	}

	// 读取流式响应
	scanner := bufio.NewScanner(response.Body)
	successful := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var pullResponse OllamaPullResponse
		if err := common.Unmarshal([]byte(line), &pullResponse); err != nil {
			continue // 忽略解析失败的行
		}

		if progressCallback != nil {
			progressCallback(pullResponse)
		}

		// 检查是否出现错误或完成
		if strings.EqualFold(pullResponse.Status, "error") {
			return fmt.Errorf("拉取模型失败: %s", strings.TrimSpace(line))
		}
		if strings.EqualFold(pullResponse.Status, "success") {
			successful = true
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取流式响应失败: %v", err)
	}

	if !successful {
		return fmt.Errorf("拉取模型未完成: 未收到成功状态")
	}

	return nil
}

// DeleteOllamaModel 从 Ollama 服务器删除指定模型。
// 通过调用 Ollama 的 /api/delete 端点删除已安装的模型，释放磁盘空间。
//
// 参数:
//   - baseURL: Ollama 服务器的基础 URL
//   - apiKey: API 密钥（可选）
//   - modelName: 要删除的模型名称
//
// 返回:
//   - error: 删除失败时返回错误信息
func DeleteOllamaModel(baseURL, apiKey, modelName string) error {
	url := fmt.Sprintf("%s/api/delete", baseURL)

	deleteRequest := OllamaDeleteRequest{
		Name: modelName,
	}

	requestBody, err := common.Marshal(deleteRequest)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	client := &http.Client{}
	request, err := http.NewRequest("DELETE", url, strings.NewReader(string(requestBody)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("删除模型失败 %d: %s", response.StatusCode, string(body))
	}

	return nil
}

// FetchOllamaVersion 查询 Ollama 服务器的版本信息。
// 通过调用 Ollama 的 /api/version 端点获取服务器版本号。
// 使用 10 秒超时，该接口响应通常很快。
//
// 参数:
//   - baseURL: Ollama 服务器的基础 URL（会自动去除末尾的斜杠）
//   - apiKey: API 密钥（可选）
//
// 返回:
//   - string: Ollama 服务器的版本号字符串（如 "0.3.12"）
//   - error: 请求失败或未返回版本信息时的错误
func FetchOllamaVersion(baseURL, apiKey string) (string, error) {
	trimmedBase := strings.TrimRight(baseURL, "/")
	if trimmedBase == "" {
		return "", fmt.Errorf("baseURL 为空")
	}

	url := fmt.Sprintf("%s/api/version", trimmedBase)

	client := &http.Client{Timeout: 10 * time.Second}
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("查询版本失败 %d: %s", response.StatusCode, string(body))
	}

	var versionResp struct {
		Version string `json:"version"`
	}

	if err := json.Unmarshal(body, &versionResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if versionResp.Version == "" {
		return "", fmt.Errorf("未返回版本信息")
	}

	return versionResp.Version, nil
}
