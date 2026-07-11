// Package service - token_counter.go
// 该文件实现了请求/响应的 Token 计数功能
//
// 功能：
// - 图像 Token 计算（支持 Patch-based 和 Tile-based 两种算法）
// - 文本 Token 统计（OpenAI 模型使用 tokenizer，其他模型使用估算）
// - 音频 Token 计算（输入/输出）
// - 实时会话（Realtime）Token 统计
// - 请求总 Token 预估（包含文本、图像、音频、视频等多模态）
package service

import (
	"errors"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	constant2 "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// getImageToken 计算图像的 Token 数量
//
// 支持两种计算算法：
// - Patch-based: 适用于 gpt-4.1-mini/nano、o4-mini、gpt-5-mini/nano 等模型，使用 32x32 补丁，上限 1536
// - Tile-based: 适用于 gpt-4o/4.1/4.5/o1/o3 等模型，使用 512px 瓦片
//
// 参数：
//   - c: Gin 上下文
//   - fileMeta: 文件元数据（包含图片源和 detail 设置）
//   - model: 模型名称
//   - stream: 是否为流式模式
//
// 返回值：
//   - int: 图像 Token 数量
//   - error: 错误
func getImageToken(c *gin.Context, fileMeta *types.FileMeta, model string, stream bool) (int, error) {
	if fileMeta == nil || fileMeta.Source == nil {
		return 0, fmt.Errorf("image_url_is_nil")
	}

	// Defaults for 4o/4.1/4.5 family unless overridden below
	baseTokens := 85
	tileTokens := 170

	// Model classification
	lowerModel := strings.ToLower(model)

	// Special cases from existing behavior
	if strings.HasPrefix(lowerModel, "glm-4") {
		return 1047, nil
	}

	// Patch-based models (32x32 patches, capped at 1536, with multiplier)
	isPatchBased := false
	multiplier := 1.0
	switch {
	case strings.Contains(lowerModel, "gpt-4.1-mini"):
		isPatchBased = true
		multiplier = 1.62
	case strings.Contains(lowerModel, "gpt-4.1-nano"):
		isPatchBased = true
		multiplier = 2.46
	case strings.HasPrefix(lowerModel, "o4-mini"):
		isPatchBased = true
		multiplier = 1.72
	case strings.HasPrefix(lowerModel, "gpt-5-mini"):
		isPatchBased = true
		multiplier = 1.62
	case strings.HasPrefix(lowerModel, "gpt-5-nano"):
		isPatchBased = true
		multiplier = 2.46
	}

	// Tile-based model tokens and bases per doc
	if !isPatchBased {
		if strings.HasPrefix(lowerModel, "gpt-4o-mini") {
			baseTokens = 2833
			tileTokens = 5667
		} else if strings.HasPrefix(lowerModel, "gpt-5-chat-latest") || (strings.HasPrefix(lowerModel, "gpt-5") && !strings.Contains(lowerModel, "mini") && !strings.Contains(lowerModel, "nano")) {
			baseTokens = 70
			tileTokens = 140
		} else if strings.HasPrefix(lowerModel, "o1") || strings.HasPrefix(lowerModel, "o3") || strings.HasPrefix(lowerModel, "o1-pro") {
			baseTokens = 75
			tileTokens = 150
		} else if strings.Contains(lowerModel, "computer-use-preview") {
			baseTokens = 65
			tileTokens = 129
		} else if strings.Contains(lowerModel, "4.1") || strings.Contains(lowerModel, "4o") || strings.Contains(lowerModel, "4.5") {
			baseTokens = 85
			tileTokens = 170
		}
	}

	// Respect existing feature flags/short-circuits
	if fileMeta.Detail == "low" && !isPatchBased {
		return baseTokens, nil
	}

	// Whether to count image tokens at all
	if !constant.GetMediaToken {
		return 3 * baseTokens, nil
	}

	if !constant.GetMediaTokenNotStream && !stream {
		return 3 * baseTokens, nil
	}
	// Normalize detail
	if fileMeta.Detail == "auto" || fileMeta.Detail == "" {
		fileMeta.Detail = "high"
	}

	// 使用统一的文件服务获取图片配置
	config, format, err := GetImageConfig(c, fileMeta.Source)
	if err != nil {
		return 0, err
	}
	if config.Width == 0 || config.Height == 0 {
		// not an image, but might be a valid file
		if format != "" {
			// file type
			return 3 * baseTokens, nil
		}
		return 0, errors.New(fmt.Sprintf("fail to decode image config: %s", fileMeta.GetIdentifier()))
	}

	width := config.Width
	height := config.Height
	log.Printf("format: %s, width: %d, height: %d", format, width, height)

	if isPatchBased {
		// 32x32 patch-based calculation with 1536 cap and model multiplier
		ceilDiv := func(a, b int) int { return (a + b - 1) / b }
		rawPatchesW := ceilDiv(width, 32)
		rawPatchesH := ceilDiv(height, 32)
		rawPatches := rawPatchesW * rawPatchesH
		if rawPatches > 1536 {
			// scale down
			area := float64(width * height)
			r := math.Sqrt(float64(32*32*1536) / area)
			wScaled := float64(width) * r
			hScaled := float64(height) * r
			// adjust to fit whole number of patches after scaling
			adjW := math.Floor(wScaled/32.0) / (wScaled / 32.0)
			adjH := math.Floor(hScaled/32.0) / (hScaled / 32.0)
			adj := math.Min(adjW, adjH)
			if !math.IsNaN(adj) && adj > 0 {
				r = r * adj
			}
			wScaled = float64(width) * r
			hScaled = float64(height) * r
			patchesW := math.Ceil(wScaled / 32.0)
			patchesH := math.Ceil(hScaled / 32.0)
			imageTokens := int(patchesW * patchesH)
			if imageTokens > 1536 {
				imageTokens = 1536
			}
			return int(math.Round(float64(imageTokens) * multiplier)), nil
		}
		// below cap
		imageTokens := rawPatches
		return int(math.Round(float64(imageTokens) * multiplier)), nil
	}

	// Tile-based calculation for 4o/4.1/4.5/o1/o3/etc.
	// Step 1: fit within 2048x2048 square
	maxSide := math.Max(float64(width), float64(height))
	fitScale := 1.0
	if maxSide > 2048 {
		fitScale = maxSide / 2048.0
	}
	fitW := int(math.Round(float64(width) / fitScale))
	fitH := int(math.Round(float64(height) / fitScale))

	// Step 2: scale so that shortest side is exactly 768
	minSide := math.Min(float64(fitW), float64(fitH))
	if minSide == 0 {
		return baseTokens, nil
	}
	shortScale := 768.0 / minSide
	finalW := int(math.Round(float64(fitW) * shortScale))
	finalH := int(math.Round(float64(fitH) * shortScale))

	// Count 512px tiles
	tilesW := (finalW + 512 - 1) / 512
	tilesH := (finalH + 512 - 1) / 512
	tiles := tilesW * tilesH

	if common.DebugEnabled {
		log.Printf("scaled to: %dx%d, tiles: %d", finalW, finalH, tiles)
	}

	return tiles*tileTokens + baseTokens, nil
}

// EstimateAudioDurationTokens 按音频时长估算 token 数。
//
// 音频转写、翻译和 TTS fallback 都按“每分钟 1000 token”的历史规则计数。
// duration 来自用户上传文件或上游音频元数据，可能被伪造为负数、NaN 或极大值；
// 这里先把负值钳为 0，再通过统一 quota 四舍五入入口转换，避免裸 int 转换回绕。
func EstimateAudioDurationTokens(duration float64) int {
	if duration < 0 {
		duration = 0
	}
	return common.QuotaRound(math.Ceil(duration) / 60.0 * 1000)
}

// EstimateRequestToken 预估请求的 Token 总数
//
// 处理流程：
// 1. 检查是否启用 Token 统计
// 2. 音频转录/翻译模式：根据音频时长计算（每分钟 1000 token）
// 3. 文本模式：统计文本 token + OpenAI 消息格式化开销
// 4. 多模态文件：根据文件类型（图像/音频/视频/文件）累加 token
//
// 参数：
//   - c: Gin 上下文
//   - meta: Token 统计元数据（文本、文件、消息数等）
//   - info: 中继信息
//
// 返回值：
//   - int: 预估的 Token 总数
//   - error: 错误
func EstimateRequestToken(c *gin.Context, meta *types.TokenCountMeta, info *relaycommon.RelayInfo) (int, error) {
	// 是否统计token
	if !constant.CountToken {
		return 0, nil
	}

	if meta == nil {
		return 0, errors.New("token count meta is nil")
	}

	if info.RelayFormat == types.RelayFormatOpenAIRealtime {
		return 0, nil
	}
	if info.RelayMode == constant2.RelayModeAudioTranscription || info.RelayMode == constant2.RelayModeAudioTranslation {
		multiForm, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return 0, fmt.Errorf("error parsing multipart form: %v", err)
		}
		fileHeaders := multiForm.File["file"]
		totalAudioToken := 0
		for _, fileHeader := range fileHeaders {
			file, err := fileHeader.Open()
			if err != nil {
				return 0, fmt.Errorf("error opening audio file: %v", err)
			}
			defer file.Close()
			// get ext and io.seeker
			ext := filepath.Ext(fileHeader.Filename)
			duration, err := common.GetAudioDuration(c.Request.Context(), file, ext)
			if err != nil {
				return 0, fmt.Errorf("error getting audio duration: %v", err)
			}
			// 一分钟 1000 token，与 $price / minute 对齐。
			totalAudioToken += EstimateAudioDurationTokens(duration)
		}
		return totalAudioToken, nil
	}

	model := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	tkm := 0

	if meta.TokenType == types.TokenTypeTextNumber {
		tkm += utf8.RuneCountInString(meta.CombineText)
	} else {
		tkm += CountTextToken(meta.CombineText, model)
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		tkm += meta.ToolsCount * 8
		tkm += meta.MessagesCount * 3 // 每条消息的格式化token数量
		tkm += meta.NameCount * 3
		tkm += 3
	}

	shouldFetchFiles := true

	if info.RelayFormat == types.RelayFormatGemini {
		shouldFetchFiles = false
	}

	// 是否本地计算媒体token数量
	if !constant.GetMediaToken {
		shouldFetchFiles = false
	}

	// 是否在非流模式下本地计算媒体token数量
	if !constant.GetMediaTokenNotStream && !info.IsStream {
		shouldFetchFiles = false
	}

	// 使用统一的文件服务获取文件类型
	for _, file := range meta.Files {
		if file.Source == nil {
			continue
		}

		// 如果文件类型未知且需要获取，通过 MIME 类型检测
		if file.FileType == "" || (file.Source.IsURL() && shouldFetchFiles) {
			// 注意：这里我们直接调用 LoadFileSource 而不是 GetMimeType
			// 因为 GetMimeType 内部可能会调用 GetFileTypeFromUrl (HEAD 请求)
			// 而我们这里既然要计算 token，通常需要完整数据
			cachedData, err := LoadFileSource(c, file.Source, "token_counter")
			if err != nil {
				if shouldFetchFiles {
					return 0, fmt.Errorf("error getting file type: %v", err)
				}
				continue
			}
			file.FileType = DetectFileType(cachedData.MimeType)
		}
	}

	for i, file := range meta.Files {
		switch file.FileType {
		case types.FileTypeImage:
			if common.IsOpenAITextModel(model) {
				token, err := getImageToken(c, file, model, info.IsStream)
				if err != nil {
					return 0, fmt.Errorf("error counting image token, media index[%d], identifier[%s], err: %v", i, file.GetIdentifier(), err)
				}
				tkm += token
			} else {
				tkm += 520
			}
		case types.FileTypeAudio:
			tkm += 256
		case types.FileTypeVideo:
			tkm += 4096 * 2
		case types.FileTypeFile:
			tkm += 4096
		default:
			tkm += 4096 // Default case for unknown file types
		}
	}

	common.SetContextKey(c, constant.ContextKeyPromptTokens, tkm)
	return tkm, nil
}

// CountTokenRealtime 统计实时会话（Realtime API）的 Token 数量
//
// 根据事件类型分别统计文本和音频 Token：
// - SessionUpdate: 统计 instructions 文本 token
// - ResponseAudioDelta: 统计输出音频 token
// - ResponseAudioTranscriptionDelta/ResponseFunctionCallArgumentsDelta: 统计文本 token
// - InputAudioBufferAppend: 统计输入音频 token
// - ConversationItemCreated: 统计消息中的文本 token
// - ResponseDone: 统计工具定义的 token
//
// 参数：
//   - info: 中继信息
//   - request: 实时事件请求
//   - model: 模型名称
//
// 返回值：
//   - int: 文本 Token 数量
//   - int: 音频 Token 数量
//   - error: 错误
func CountTokenRealtime(info *relaycommon.RelayInfo, request dto.RealtimeEvent, model string) (int, int, error) {
	audioToken := 0
	textToken := 0
	switch request.Type {
	case dto.RealtimeEventTypeSessionUpdate:
		if request.Session != nil {
			msgTokens := CountTextToken(request.Session.Instructions, model)
			textToken += msgTokens
		}
	case dto.RealtimeEventResponseAudioDelta:
		// count audio token
		atk, err := CountAudioTokenOutput(request.Delta, info.OutputAudioFormat)
		if err != nil {
			return 0, 0, fmt.Errorf("error counting audio token: %v", err)
		}
		audioToken += atk
	case dto.RealtimeEventResponseAudioTranscriptionDelta, dto.RealtimeEventResponseFunctionCallArgumentsDelta:
		// count text token
		tkm := CountTextToken(request.Delta, model)
		textToken += tkm
	case dto.RealtimeEventInputAudioBufferAppend:
		// count audio token
		atk, err := CountAudioTokenInput(request.Audio, info.InputAudioFormat)
		if err != nil {
			return 0, 0, fmt.Errorf("error counting audio token: %v", err)
		}
		audioToken += atk
	case dto.RealtimeEventConversationItemCreated:
		if request.Item != nil {
			switch request.Item.Type {
			case "message":
				for _, content := range request.Item.Content {
					if content.Type == "input_text" {
						tokens := CountTextToken(content.Text, model)
						textToken += tokens
					}
				}
			}
		}
	case dto.RealtimeEventTypeResponseDone:
		// count tools token
		if !info.IsFirstRequest {
			if info.RealtimeTools != nil && len(info.RealtimeTools) > 0 {
				for _, tool := range info.RealtimeTools {
					toolTokens := CountTokenInput(tool, model)
					textToken += 8
					textToken += toolTokens
				}
			}
		}
	}
	return textToken, audioToken, nil
}

// CountTokenInput 统计任意类型输入的 Token 数量
//
// 支持的输入类型：string、[]string、[]interface{}
// 其他类型会通过 fmt.Sprintf 转为字符串后计算
//
// 参数：
//   - input: 输入内容
//   - model: 模型名称
//
// 返回值：
//   - int: Token 数量
func CountTokenInput(input any, model string) int {
	switch v := input.(type) {
	case string:
		return CountTextToken(v, model)
	case []string:
		text := ""
		for _, s := range v {
			text += s
		}
		return CountTextToken(text, model)
	case []interface{}:
		text := ""
		for _, item := range v {
			text += fmt.Sprintf("%v", item)
		}
		return CountTextToken(text, model)
	}
	return CountTokenInput(fmt.Sprintf("%v", input), model)
}

// CountAudioTokenInput 统计输入音频的 Token 数量
//
// 计算公式：duration / 60 * 100 / 0.06
//
// 参数：
//   - audioBase64: Base64 编码的音频数据
//   - audioFormat: 音频格式
//
// 返回值：
//   - int: Token 数量
//   - error: 错误
func CountAudioTokenInput(audioBase64 string, audioFormat string) (int, error) {
	if audioBase64 == "" {
		return 0, nil
	}
	duration, err := parseAudio(audioBase64, audioFormat)
	if err != nil {
		return 0, err
	}
	return estimateRealtimeAudioInputTokens(duration), nil
}

// CountAudioTokenOutput 统计输出音频的 Token 数量
//
// 计算公式：duration / 60 * 200 / 0.24
//
// 参数：
//   - audioBase64: Base64 编码的音频数据
//   - audioFormat: 音频格式
//
// 返回值：
//   - int: Token 数量
//   - error: 错误
func CountAudioTokenOutput(audioBase64 string, audioFormat string) (int, error) {
	if audioBase64 == "" {
		return 0, nil
	}
	duration, err := parseAudio(audioBase64, audioFormat)
	if err != nil {
		return 0, err
	}
	return estimateRealtimeAudioOutputTokens(duration), nil
}

// estimateRealtimeAudioInputTokens 将实时音频输入时长换算为 token。
//
// duration 来自用户提供的音频元数据。正常路径继续使用历史公式：
// duration / 60 * 100 / 0.06；转换阶段复用统一 quota 饱和保护，避免异常大值
// 在裸 int 转换时回绕。负时长没有真实使用量含义，按 0 处理，避免低估预扣费。
func estimateRealtimeAudioInputTokens(duration float64) int {
	if duration < 0 {
		duration = 0
	}
	return common.QuotaFromFloat(duration / 60 * 100 / 0.06)
}

// estimateRealtimeAudioOutputTokens 将实时音频输出时长换算为 token。
//
// duration 来自上游返回的音频元数据。正常路径继续使用历史公式：
// duration / 60 * 200 / 0.24；转换阶段复用统一 quota 饱和保护，避免异常大值
// 在裸 int 转换时回绕。负时长没有真实使用量含义，按 0 处理，避免生成负 token。
func estimateRealtimeAudioOutputTokens(duration float64) int {
	if duration < 0 {
		duration = 0
	}
	return common.QuotaFromFloat(duration / 60 * 200 / 0.24)
}

// CountTextToken 统计文本的token数量，仅OpenAI模型使用tokenizer，其余模型使用估算
func CountTextToken(text string, model string) int {
	if text == "" {
		return 0
	}
	if common.IsOpenAITextModel(model) {
		tokenEncoder := getTokenEncoder(model)
		return getTokenNum(tokenEncoder, text)
	} else {
		// 非openai模型，使用tiktoken-go计算没有意义，使用估算节省资源
		return EstimateTokenByModel(model, text)
	}
}
