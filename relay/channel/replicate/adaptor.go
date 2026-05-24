// Package replicate 实现了 Replicate 平台的渠道适配器。
// 负责将统一的图片生成请求转换为 Replicate API 格式，
// 处理图片上传、预测结果解析、URL 转 base64 等功能。
package replicate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	// 第三方依赖
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// Adaptor 是 Replicate 渠道的适配器结构体，
// 实现了 channel.Adaptor 接口，负责请求转换和响应处理。
type Adaptor struct {
}

// Init 初始化适配器。Replicate 渠道无需额外初始化操作。
// 参数:
//   - info: 中继请求的上下文信息
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建 Replicate API 的完整请求 URL。
// 如果未设置基础 URL，则使用默认的 Replicate API 地址。
// 参数:
//   - info: 包含基础 URL 和请求路径的中继信息
// 返回:
//   - string: 完整的请求 URL
//   - error: 构建失败时返回错误
	if info == nil {
		return "", errors.New("replicate adaptor: relay info is nil")
	}
	if info.ChannelBaseUrl == "" {
		info.ChannelBaseUrl = constant.ChannelBaseURLs[constant.ChannelTypeReplicate]
	}
	requestPath := info.RequestURLPath
	if requestPath == "" {
		return info.ChannelBaseUrl, nil
	}
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestPath, info.ChannelType), nil
}

// SetupRequestHeader 设置 Replicate API 请求头。
// 包括 Authorization Bearer token、Prefer 同步等待、Content-Type 和 Accept 等。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: 包含 API Key 等信息的中继信息
// 返回:
//   - error: 设置失败时返回错误（如缺少 API Key）
	if info == nil {
		return errors.New("replicate adaptor: relay info is nil")
	}
	if info.ApiKey == "" {
		return errors.New("replicate adaptor: api key is required")
	}
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	req.Set("Prefer", "wait")
	if req.Get("Content-Type") == "" {
		req.Set("Content-Type", "application/json")
	}
	if req.Get("Accept") == "" {
		req.Set("Accept", "application/json")
	}
	return nil
}

// ConvertImageRequest 将统一的图片生成请求转换为 Replicate API 格式。
// 处理内容包括：prompt、图片尺寸/宽高比映射、输出格式、批量数量、质量设置等。
// 对于图片编辑模式（RelayModeImagesEdits），会上传参考图片到 Replicate 文件 API。
// 支持通过 extra_fields 和 extra 字段透传自定义参数。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息，包含上游模型名、中继模式等
//   - request: 统一的图片请求结构体
// 返回:
//   - any: 转换后的 Replicate 请求体（包含 input 字段）
//   - error: 转换失败时返回错误
	if info == nil {
		return nil, errors.New("replicate adaptor: relay info is nil")
	}
	if strings.TrimSpace(request.Prompt) == "" {
		if v := c.PostForm("prompt"); strings.TrimSpace(v) != "" {
			request.Prompt = v
		}
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("replicate adaptor: prompt is required")
	}

	modelName := strings.TrimSpace(info.UpstreamModelName)
	if modelName == "" {
		modelName = strings.TrimSpace(request.Model)
	}
	if modelName == "" {
		modelName = ModelFlux11Pro
	}
	info.UpstreamModelName = modelName

	info.RequestURLPath = fmt.Sprintf("/v1/models/%s/predictions", modelName)

	inputPayload := make(map[string]any)
	inputPayload["prompt"] = request.Prompt

	if size := strings.TrimSpace(request.Size); size != "" {
		if aspect, width, height, ok := mapOpenAISizeToFlux(size); ok {
			if aspect != "" {
				if aspect == "custom" {
					inputPayload["aspect_ratio"] = "custom"
					if width > 0 {
						inputPayload["width"] = width
					}
					if height > 0 {
						inputPayload["height"] = height
					}
				} else {
					inputPayload["aspect_ratio"] = aspect
				}
			}
		}
	}

	if len(request.OutputFormat) > 0 {
		var outputFormat string
		if err := json.Unmarshal(request.OutputFormat, &outputFormat); err == nil && strings.TrimSpace(outputFormat) != "" {
			inputPayload["output_format"] = outputFormat
		}
	}

	if imageN := lo.FromPtrOr(request.N, uint(0)); imageN > 0 {
		inputPayload["num_outputs"] = int(imageN)
	}

	if strings.EqualFold(request.Quality, "hd") || strings.EqualFold(request.Quality, "high") {
		inputPayload["prompt_upsampling"] = true
	}

	if info.RelayMode == relayconstant.RelayModeImagesEdits {
		imageURL, err := uploadFileFromForm(c, info, "image", "image[]", "image_prompt")
		if err != nil {
			return nil, err
		}
		if imageURL == "" {
			return nil, errors.New("replicate adaptor: image file is required for edits")
		}
		inputPayload["image_prompt"] = imageURL
	}

	if len(request.ExtraFields) > 0 {
		var extra map[string]any
		if err := common.Unmarshal(request.ExtraFields, &extra); err != nil {
			return nil, fmt.Errorf("replicate adaptor: failed to decode extra_fields: %w", err)
		}
		for key, val := range extra {
			inputPayload[key] = val
		}
	}

	for key, raw := range request.Extra {
		if strings.EqualFold(key, "input") {
			var extraInput map[string]any
			if err := common.Unmarshal(raw, &extraInput); err != nil {
				return nil, fmt.Errorf("replicate adaptor: failed to decode extra input: %w", err)
			}
			for k, v := range extraInput {
				inputPayload[k] = v
			}
			continue
		}
		if raw == nil {
			continue
		}
		var val any
		if err := common.Unmarshal(raw, &val); err != nil {
			return nil, fmt.Errorf("replicate adaptor: failed to decode extra field %s: %w", key, err)
		}
		inputPayload[key] = val
	}

	return map[string]any{
		"input": inputPayload,
	}, nil
}

// DoRequest 执行实际的 HTTP API 请求，委托给通用的 DoApiRequest 方法。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - requestBody: 请求体 io.Reader
// 返回:
//   - any: 原始响应
//   - error: 请求失败时返回错误
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理 Replicate API 的响应。
// 流程：读取响应体 -> 解析 PredictionResponse -> 检查错误和状态 ->
// 提取输出 URL -> 根据请求格式返回 URL 或 base64 -> 写入标准图片响应。
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: 中继信息
// 返回:
//   - any: 使用量信息（Usage）
//   - *types.NexusTokError: 错误信息
	if resp == nil {
		return nil, types.NewError(errors.New("replicate adaptor: empty response"), types.ErrorCodeBadResponse)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	_ = resp.Body.Close()

	var prediction PredictionResponse
	if err := common.Unmarshal(responseBody, &prediction); err != nil {
		return nil, types.NewError(fmt.Errorf("replicate adaptor: failed to decode response: %w", err), types.ErrorCodeBadResponseBody)
	}

	if prediction.Error != nil {
		errMsg := prediction.Error.Message
		if errMsg == "" {
			errMsg = prediction.Error.Detail
		}
		if errMsg == "" {
			errMsg = prediction.Error.Code
		}
		if errMsg == "" {
			errMsg = "replicate adaptor: prediction error"
		}
		return nil, types.NewError(errors.New(errMsg), types.ErrorCodeBadResponse)
	}

	if prediction.Status != "" && !strings.EqualFold(prediction.Status, "succeeded") {
		return nil, types.NewError(fmt.Errorf("replicate adaptor: prediction status %q", prediction.Status), types.ErrorCodeBadResponse)
	}

	var urls []string

	appendOutput := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		urls = append(urls, value)
	}

	switch output := prediction.Output.(type) {
	case string:
		appendOutput(output)
	case []any:
		for _, item := range output {
			if str, ok := item.(string); ok {
				appendOutput(str)
			}
		}
	case nil:
		// no output
	default:
		if str, ok := output.(fmt.Stringer); ok {
			appendOutput(str.String())
		}
	}

	if len(urls) == 0 {
		return nil, types.NewError(errors.New("replicate adaptor: empty prediction output"), types.ErrorCodeBadResponseBody)
	}

	var imageReq *dto.ImageRequest
	if info != nil {
		if req, ok := info.Request.(*dto.ImageRequest); ok {
			imageReq = req
		}
	}

	wantsBase64 := imageReq != nil && strings.EqualFold(imageReq.ResponseFormat, "b64_json")

	imageResponse := dto.ImageResponse{
		Created: common.GetTimestamp(),
		Data:    make([]dto.ImageData, 0),
	}

	if wantsBase64 {
		converted, convErr := downloadImagesToBase64(urls)
		if convErr != nil {
			return nil, types.NewError(convErr, types.ErrorCodeBadResponse)
		}
		for _, content := range converted {
			if content == "" {
				continue
			}
			imageResponse.Data = append(imageResponse.Data, dto.ImageData{B64Json: content})
		}
	} else {
		for _, url := range urls {
			if url == "" {
				continue
			}
			imageResponse.Data = append(imageResponse.Data, dto.ImageData{Url: url})
		}
	}

	if len(imageResponse.Data) == 0 {
		return nil, types.NewError(errors.New("replicate adaptor: no usable image data"), types.ErrorCodeBadResponse)
	}

	responseBytes, err := common.Marshal(imageResponse)
	if err != nil {
		return nil, types.NewError(fmt.Errorf("replicate adaptor: encode response failed: %w", err), types.ErrorCodeBadResponseBody)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(responseBytes)

	usage := &dto.Usage{}
	return usage, nil
}

// GetModelList 返回 Replicate 渠道支持的模型列表。
// 返回:
//   - []string: 模型名称切片
	return ModelList
}

// GetChannelName 返回渠道名称标识 "replicate"。
// 返回:
//   - string: 渠道名称
	return ChannelName
}

// downloadImagesToBase64 批量下载图片并转换为 base64 编码字符串。
// 参数:
//   - urls: 图片 URL 列表
// 返回:
//   - []string: base64 编码的图片数据列表
//   - error: 下载或转换失败时返回错误
	results := make([]string, 0, len(urls))
	for _, url := range urls {
		if strings.TrimSpace(url) == "" {
			continue
		}
		_, data, err := service.GetImageFromUrl(url)
		if err != nil {
			return nil, fmt.Errorf("replicate adaptor: failed to download image from %s: %w", url, err)
		}
		results = append(results, data)
	}
	return results, nil
}

// mapOpenAISizeToFlux 将 OpenAI 格式的图片尺寸（如 "1024x1024"）映射为
// Replicate Flux 模型支持的宽高比或自定义宽高。
// 支持预定义比例：1:1、16:9、9:16、3:2、2:3 等，
// 不支持的比例会计算最简比后判断，仍不匹配则返回 custom + 归一化宽高。
// 参数:
//   - size: OpenAI 格式的尺寸字符串，如 "1792x1024"
// 返回:
//   - aspect: 宽高比字符串，如 "16:9" 或 "custom"
//   - width: 自定义宽度（仅 aspect="custom" 时有效）
//   - height: 自定义高度（仅 aspect="custom" 时有效）
//   - ok: 是否成功映射
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return "", 0, 0, false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return "", 0, 0, false
	}

	switch {
	case w == h:
		return "1:1", 0, 0, true
	case w == 1792 && h == 1024:
		return "16:9", 0, 0, true
	case w == 1024 && h == 1792:
		return "9:16", 0, 0, true
	case w == 1536 && h == 1024:
		return "3:2", 0, 0, true
	case w == 1024 && h == 1536:
		return "2:3", 0, 0, true
	}

	rw, rh := reduceRatio(w, h)
	ratioStr := fmt.Sprintf("%d:%d", rw, rh)
	switch ratioStr {
	case "1:1", "16:9", "9:16", "3:2", "2:3", "4:5", "5:4", "3:4", "4:3":
		return ratioStr, 0, 0, true
	}

	width = normalizeFluxDimension(w)
	height = normalizeFluxDimension(h)
	return "custom", width, height, true
}

// reduceRatio 将宽高值约简为最简整数比。
// 参数:
//   - w: 宽度
//   - h: 高度
// 返回:
//   - int: 约简后的宽度
//   - int: 约简后的高度
	g := gcd(w, h)
	if g == 0 {
		return w, h
	}
	return w / g, h / g
}

// gcd 计算两个整数的最大公约数（Greatest Common Divisor）。
// 使用欧几里得辗转相除法实现。
// 参数:
//   - a: 第一个整数
//   - b: 第二个整数
// 返回:
//   - int: 最大公约数
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

// normalizeFluxDimension 将图片尺寸归一化到 Flux 模型支持的范围内。
// 范围限制为 [256, 1440]，并按 32 的步长进行四舍五入对齐。
// 参数:
//   - value: 原始尺寸值
// 返回:
//   - int: 归一化后的尺寸值
	const (
		minDim = 256
		maxDim = 1440
		step   = 32
	)
	if value < minDim {
		value = minDim
	}
	if value > maxDim {
		value = maxDim
	}
	remainder := value % step
	if remainder != 0 {
		if remainder >= step/2 {
			value += step - remainder
		} else {
			value -= remainder
		}
	}
	if value < minDim {
		value = minDim
	}
	if value > maxDim {
		value = maxDim
	}
	return value
}

// uploadFileFromForm 从 multipart form 中提取图片文件并上传到 Replicate 文件 API。
// 按优先级依次查找 fieldCandidates 中指定的表单字段名，
// 找到文件后构建 multipart 请求上传到 /v1/files 端点。
// 参数:
//   - c: Gin 上下文，包含 multipart form 数据
//   - info: 中继信息，包含基础 URL 和 API Key
//   - fieldCandidates: 候选的表单字段名列表
// 返回:
//   - string: 上传成功后返回的文件 GET URL
//   - error: 上传失败时返回错误
	if info == nil {
		return "", errors.New("replicate adaptor: relay info is nil")
	}

	mf := c.Request.MultipartForm
	if mf == nil {
		if _, err := c.MultipartForm(); err != nil {
			return "", fmt.Errorf("replicate adaptor: parse multipart form failed: %w", err)
		}
		mf = c.Request.MultipartForm
	}
	if mf == nil || len(mf.File) == 0 {
		return "", nil
	}

	if len(fieldCandidates) == 0 {
		fieldCandidates = []string{"image", "image[]", "image_prompt"}
	}

	var fileHeader *multipart.FileHeader
	for _, key := range fieldCandidates {
		if files := mf.File[key]; len(files) > 0 {
			fileHeader = files[0]
			break
		}
	}
	if fileHeader == nil {
		for _, files := range mf.File {
			if len(files) > 0 {
				fileHeader = files[0]
				break
			}
		}
	}
	if fileHeader == nil {
		return "", nil
	}

	file, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("replicate adaptor: failed to open image file: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf("form-data; name=\"content\"; filename=\"%s\"", fileHeader.Filename))
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	hdr.Set("Content-Type", contentType)

	part, err := writer.CreatePart(hdr)
	if err != nil {
		writer.Close()
		return "", fmt.Errorf("replicate adaptor: create upload form failed: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		writer.Close()
		return "", fmt.Errorf("replicate adaptor: copy image content failed: %w", err)
	}
	formContentType := writer.FormDataContentType()
	writer.Close()

	baseURL := info.ChannelBaseUrl
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[constant.ChannelTypeReplicate]
	}
	uploadURL := relaycommon.GetFullRequestURL(baseURL, "/v1/files", info.ChannelType)

	req, err := http.NewRequest(http.MethodPost, uploadURL, &body)
	if err != nil {
		return "", fmt.Errorf("replicate adaptor: create upload request failed: %w", err)
	}
	req.Header.Set("Content-Type", formContentType)
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)

	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("replicate adaptor: upload image failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("replicate adaptor: read upload response failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("replicate adaptor: upload image failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var uploadResp FileUploadResponse
	if err := common.Unmarshal(respBody, &uploadResp); err != nil {
		return "", fmt.Errorf("replicate adaptor: decode upload response failed: %w", err)
	}
	if uploadResp.Urls.Get == "" {
		return "", errors.New("replicate adaptor: upload response missing url")
	}
	return uploadResp.Urls.Get, nil
}

// ConvertOpenAIRequest 未实现，Replicate 渠道不支持 OpenAI 格式的文本补全请求。
func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("replicate adaptor: ConvertOpenAIRequest is not implemented")
}

// ConvertRerankRequest 未实现，Replicate 渠道不支持 Rerank 请求。
func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("replicate adaptor: ConvertRerankRequest is not implemented")
}

// ConvertEmbeddingRequest 未实现，Replicate 渠道不支持 Embedding 请求。
func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("replicate adaptor: ConvertEmbeddingRequest is not implemented")
}

// ConvertAudioRequest 未实现，Replicate 渠道不支持音频请求。
func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("replicate adaptor: ConvertAudioRequest is not implemented")
}

// ConvertOpenAIResponsesRequest 未实现，Replicate 渠道不支持 OpenAI Responses 请求。
func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("replicate adaptor: ConvertOpenAIResponsesRequest is not implemented")
}

// ConvertClaudeRequest 未实现，Replicate 渠道不支持 Claude 格式请求。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("replicate adaptor: ConvertClaudeRequest is not implemented")
}

// ConvertGeminiRequest 未实现，Replicate 渠道不支持 Gemini 格式请求。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("replicate adaptor: ConvertGeminiRequest is not implemented")
}
