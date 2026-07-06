// Package middleware - distributor.go
// 该文件实现了渠道分发中间件，是请求转发流程中的核心环节
//
// 核心职责：
// 1. 从请求中解析出模型名称（支持多种 API 格式：OpenAI、Claude、Gemini、Midjourney、Suno 等）
// 2. 验证令牌（Token）对请求模型的访问权限
// 3. 选择合适的上游渠道（Channel）来处理请求
// 4. 将选中渠道的配置信息写入请求上下文（供后续 relay 使用）
//
// 渠道选择策略：
// - 如果请求指定了 specific_channel_id（管理员专用），直接使用该渠道
// - 如果存在渠道亲和性（Channel Affinity），优先使用上次成功的渠道
// - 否则通过 CacheGetRandomSatisfiedChannel 随机选择一个满足条件的渠道
// - 支持 auto 分组的跨分组重试机制
package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"                       // 公共工具包
	"github.com/c1cada/NexusTok/constant"                     // 常量定义
	"github.com/c1cada/NexusTok/dto"                          // 数据传输对象
	"github.com/c1cada/NexusTok/i18n"                         // 国际化
	"github.com/c1cada/NexusTok/model"                        // 数据模型
	relayconstant "github.com/c1cada/NexusTok/relay/constant" // 中继常量
	"github.com/c1cada/NexusTok/service"                      // 服务层
	"github.com/c1cada/NexusTok/setting/ratio_setting"        // 比率设置
	"github.com/c1cada/NexusTok/types"                        // 类型定义

	"github.com/gin-gonic/gin" // Gin 框架
)

// ModelRequest 模型请求结构体
// 从请求体中解析出的模型信息，用于渠道选择
type ModelRequest struct {
	Model string `json:"model"`           // 请求的模型名称，如 "gpt-4"、"claude-3-opus"
	Group string `json:"group,omitempty"` // 请求的分组（仅 Playground 使用）
}

// Distribute 渠道分发中间件
// 这是请求转发流程中的核心中间件，负责：
// 1. 解析请求中的模型名称
// 2. 验证令牌对模型的访问权限
// 3. 选择合适的上游渠道
// 4. 将渠道配置写入上下文
//
// 执行时机：在 TokenAuth 之后、relay 处理之前
// 返回值：gin.HandlerFunc 中间件函数
func Distribute() func(c *gin.Context) {
	return func(c *gin.Context) {
		channel, ok := PrepareRelayChannelContext(c)
		if !ok {
			return
		}
		// 请求结束后释放选中的账号资源。这里保留在中间件层做释放，保证所有标准
		// Relay 路由、Playground 路由以及后续新增的入口都不会遗留账号并发占用。
		defer service.ReleaseSelectedChannelAccount(c)
		defer service.ReleaseSelectedPoolAccount(c)
		c.Next()
		RecordRelayChannelAffinityIfSucceeded(c, channel)
	}
}

// PrepareRelayChannelContext 为一次 Relay 请求完成渠道选择并写入上下文。
//
// 该函数从 Distribute 中间件中抽出，供 CPAMC 嵌入式 api-call 这类“已经
// 通过 NexusTok session 完成管理员鉴权，但仍需要复用主 Relay 链路”的入口调用。
// 它保持与标准 TokenAuth -> Distribute 链路相同的不变量：
// - 根据请求体或路径解析模型；
// - 检查 Token 模型限制；
// - 按用户分组、渠道亲和性和随机策略选择渠道；
// - 将渠道 Key、BaseURL、参数覆盖、账号池上下文等写入 Gin context。
//
// 如果准备失败，本函数会直接写入 OpenAI 兼容错误响应并 Abort，调用方只需要
// 根据返回的 ok 判断是否继续执行 Relay。
func PrepareRelayChannelContext(c *gin.Context) (*model.Channel, bool) {
	var channel *model.Channel
	// 检查是否指定了特定渠道（管理员功能）
	channelId, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId)
	// 从请求中解析模型信息
	modelRequest, shouldSelectChannel, err := getModelRequest(c)
	if err != nil {
		abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidRequest, map[string]any{"Error": err.Error()}))
		return nil, false
	}
	if ok {
		// ========== 管理员指定了特定渠道 ==========
		id, err := strconv.Atoi(channelId.(string))
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidChannelId))
			return nil, false
		}
		channel, err = model.GetChannelById(id, true)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidChannelId))
			return nil, false
		}
		// 检查渠道是否启用
		if channel.Status != common.ChannelStatusEnabled {
			abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorChannelDisabled))
			return nil, false
		}
	} else {
		selected, selectOk := selectRelayChannel(c, modelRequest, shouldSelectChannel)
		if !selectOk {
			return nil, false
		}
		channel = selected
	}
	// 记录请求开始时间（用于计费和日志）
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	// 将选中渠道的配置信息写入上下文
	if setupErr := SetupContextForSelectedChannel(c, channel, modelRequest.Model); setupErr != nil {
		abortWithOpenAiMessage(c, setupErr.StatusCode, setupErr.Error(), setupErr.GetErrorCode())
		return nil, false
	}
	return channel, true
}

// selectRelayChannel 按标准分发规则为请求选择渠道。
//
// 该函数只负责“自动选路”分支，管理员指定 specific_channel_id 的路径仍由
// PrepareRelayChannelContext 直接处理。拆分出来是为了让中间件和 CPAMC
// 内部重放请求共用同一套模型限制、分组、亲和性和随机选路逻辑。
func selectRelayChannel(c *gin.Context, modelRequest *ModelRequest, shouldSelectChannel bool) (*model.Channel, bool) {
	var channel *model.Channel
	var selectGroup string
	var err error

	// ========== 自动选择渠道 ==========
	// 检查令牌是否启用了模型限制
	modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
	if modelLimitEnable {
		s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
		if !ok {
			// 令牌模型限制列表为空，表示所有模型都被禁止访问
			abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorTokenNoModelAccess))
			return nil, false
		}
		var tokenModelLimit map[string]bool
		tokenModelLimit, ok = s.(map[string]bool)
		if !ok {
			tokenModelLimit = map[string]bool{}
		}
		// 匹配模型名称（包括 GPTs 和 thinking-* 前缀的模型）
		matchName := ratio_setting.FormatMatchingModelName(modelRequest.Model)
		if _, ok := tokenModelLimit[matchName]; !ok {
			abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorTokenModelForbidden, map[string]any{"Model": modelRequest.Model}))
			return nil, false
		}
	}

	if !shouldSelectChannel {
		return nil, true
	}
	if modelRequest.Model == "" {
		abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorModelNameRequired))
		return nil, false
	}
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	// Playground 路径特殊处理：允许用户在请求中指定分组
	if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") {
		playgroundRequest := &dto.PlayGroundRequest{}
		err = common.UnmarshalBodyReusable(c, playgroundRequest)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidPlayground, map[string]any{"Error": err.Error()}))
			return nil, false
		}
		if playgroundRequest.Group != "" {
			if !service.GroupInUserUsableGroups(usingGroup, playgroundRequest.Group) && playgroundRequest.Group != usingGroup {
				abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorGroupAccessDenied))
				return nil, false
			}
			usingGroup = playgroundRequest.Group
			common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
		}
	}

	// ========== 渠道亲和性（Channel Affinity）优先选择 ==========
	// 如果之前对该模型的成功请求使用过某个渠道，优先复用该渠道
	if preferredChannelID, found := service.GetPreferredChannelByAffinity(c, modelRequest.Model, usingGroup); found {
		preferred, err := model.CacheGetChannel(preferredChannelID)
		if err == nil && preferred != nil {
			if preferred.Status != common.ChannelStatusEnabled {
				// 亲和性渠道已禁用
				if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
					abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorAffinityChannelDisabled))
					return nil, false
				}
			} else if usingGroup == "auto" {
				// auto 分组：遍历用户可用的自动分组，找到第一个支持该模型的分组
				userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
				autoGroups := service.GetUserAutoGroup(userGroup)
				for _, g := range autoGroups {
					if model.IsChannelEnabledForGroupModel(g, modelRequest.Model, preferred.Id) {
						selectGroup = g
						common.SetContextKey(c, constant.ContextKeyAutoGroup, g)
						channel = preferred
						service.MarkChannelAffinityUsed(c, g, preferred.Id)
						break
					}
				}
			} else if model.IsChannelEnabledForGroupModel(usingGroup, modelRequest.Model, preferred.Id) {
				// 普通分组：检查渠道是否在该分组中支持该模型
				channel = preferred
				selectGroup = usingGroup
				service.MarkChannelAffinityUsed(c, usingGroup, preferred.Id)
			}
		}
	}

	// ========== 随机选择渠道（兜底逻辑） ==========
	if channel == nil {
		channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
			Ctx:        c,
			ModelName:  modelRequest.Model,
			TokenGroup: usingGroup,
			Retry:      common.GetPointer(0),
		})
		if err != nil {
			showGroup := usingGroup
			if usingGroup == "auto" {
				showGroup = fmt.Sprintf("auto(%s)", selectGroup)
			}
			message := i18n.T(c, i18n.MsgDistributorGetChannelFailed, map[string]any{"Group": showGroup, "Model": modelRequest.Model, "Error": err.Error()})
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, message, types.ErrorCodeModelNotFound)
			return nil, false
		}
		if channel == nil {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, i18n.T(c, i18n.MsgDistributorNoAvailableChannel, map[string]any{"Group": usingGroup, "Model": modelRequest.Model}), types.ErrorCodeModelNotFound)
			return nil, false
		}
	}
	return channel, true
}

// RecordRelayChannelAffinityIfSucceeded 在请求成功后记录渠道亲和性。
//
// 该函数独立出来，是为了让非标准 Gin 中间件链（例如 CPAMC api-call 在
// NexusTok 后端内部重放到主 Relay）也能在成功时复用相同的亲和性记录规则。
func RecordRelayChannelAffinityIfSucceeded(c *gin.Context, channel *model.Channel) {
	if channel != nil && c.Writer != nil && c.Writer.Status() < http.StatusBadRequest {
		service.RecordChannelAffinity(c, channel.Id)
	}
}

// getModelFromRequest 从请求体中读取模型信息
// 根据 Content-Type 自动处理不同格式：
// - application/json: 从 JSON body 解析
// - application/x-www-form-urlencoded: 从表单字段解析
// - multipart/form-data: 从表单字段解析
//
// 参数：
//   - c: Gin 上下文
//
// 返回值：
//   - *ModelRequest: 解析出的模型请求信息
//   - error: 解析错误
func getModelFromRequest(c *gin.Context) (*ModelRequest, error) {
	var modelRequest ModelRequest
	err := common.UnmarshalBodyReusable(c, &modelRequest)
	if err != nil {
		return nil, errors.New(i18n.T(c, i18n.MsgDistributorInvalidRequest, map[string]any{"Error": err.Error()}))
	}
	return &modelRequest, nil
}

// getModelRequest 从请求中解析模型信息和中继模式
// 根据请求路径自动识别 API 类型，并返回对应的模型名称
//
// 支持的 API 类型：
// - Midjourney: /mj/ 路径，从 MidjourneyRequest 中提取模型
// - Suno: /suno/ 路径，从 action 参数转换为模型名
// - Video: /v1/videos/ 和 /v1/video/generations 路径
// - Gemini: /v1beta/models/ 和 /v1/models/ 路径
// - OpenAI: 默认路径，从 JSON body 中提取 model 字段
// - Realtime: /v1/realtime 路径，从 query 参数获取 model
// - Moderations: /v1/moderations 路径，默认 "text-moderation-stable"
// - Embeddings: embeddings 路径，从 URL 参数获取 model
// - Images: /v1/images/ 路径，默认 "dall-e"
// - Audio: /v1/audio/ 路径，默认 "tts-1" 或 "whisper-1"
// - Playground: /pg/chat/completions 路径，支持分组参数
//
// 参数：
//   - c: Gin 上下文
//
// 返回值：
//   - *ModelRequest: 解析出的模型请求信息
//   - bool: 是否需要选择渠道（任务查询类请求不需要）
//   - error: 解析错误
func getModelRequest(c *gin.Context) (*ModelRequest, bool, error) {
	var modelRequest ModelRequest
	shouldSelectChannel := true
	var err error
	// ========== Midjourney API 路径处理 ==========
	if strings.Contains(c.Request.URL.Path, "/mj/") {
		// 根据路径识别 Midjourney 中继模式
		relayMode := relayconstant.Path2RelayModeMidjourney(c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeMidjourneyTaskFetch ||
			relayMode == relayconstant.RelayModeMidjourneyTaskFetchByCondition ||
			relayMode == relayconstant.RelayModeMidjourneyNotify ||
			relayMode == relayconstant.RelayModeMidjourneyTaskImageSeed {
			// 任务查询/通知类请求不需要选择渠道（使用原始任务的渠道）
			shouldSelectChannel = false
		} else {
			// 提交类请求需要从 body 中解析模型
			midjourneyRequest := dto.MidjourneyRequest{}
			err = common.UnmarshalBodyReusable(c, &midjourneyRequest)
			if err != nil {
				return nil, false, errors.New(i18n.T(c, i18n.MsgDistributorInvalidMidjourney, map[string]any{"Error": err.Error()}))
			}
			midjourneyModel, mjErr, success := service.GetMjRequestModel(relayMode, &midjourneyRequest)
			if mjErr != nil {
				return nil, false, fmt.Errorf("%s", mjErr.Description)
			}
			if midjourneyModel == "" {
				if !success {
					return nil, false, fmt.Errorf("%s", i18n.T(c, i18n.MsgDistributorInvalidParseModel))
				} else {
					// 任务查询类请求
					shouldSelectChannel = false
				}
			}
			modelRequest.Model = midjourneyModel
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/suno/") {
		// ========== Suno 音乐生成 API 路径处理 ==========
		relayMode := relayconstant.Path2RelaySuno(c.Request.Method, c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeSunoFetch ||
			relayMode == relayconstant.RelayModeSunoFetchByID {
			// 任务查询请求不需要选择渠道
			shouldSelectChannel = false
		} else {
			// 提交请求：从 action 参数转换为模型名
			modelName := service.CoverTaskActionToModelName(constant.TaskPlatformSuno, c.Param("action"))
			modelRequest.Model = modelName
		}
		c.Set("platform", string(constant.TaskPlatformSuno))
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos/") && strings.HasSuffix(c.Request.URL.Path, "/remix") {
		// ========== 视频 Remix API 路径处理 ==========
		relayMode := relayconstant.RelayModeVideoSubmit
		c.Set("relay_mode", relayMode)
		shouldSelectChannel = false
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos") {
		// ========== 视频生成 API 路径处理（Sora 等） ==========
		// 示例：POST /v1/videos -H "Authorization: Bearer $KEY" -F "model=sora-2" -F "prompt=..."
		relayMode := relayconstant.RelayModeUnknown
		if c.Request.Method == http.MethodPost {
			relayMode = relayconstant.RelayModeVideoSubmit
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			if req != nil {
				modelRequest.Model = req.Model
			}
		} else if c.Request.Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/video/generations") {
		// ========== 视频生成 API 路径处理（Kling/Jimeng 等） ==========
		relayMode := relayconstant.RelayModeUnknown
		if c.Request.Method == http.MethodPost {
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			modelRequest.Model = req.Model
			relayMode = relayconstant.RelayModeVideoSubmit
		} else if c.Request.Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
		}
		if _, ok := c.Get("relay_mode"); !ok {
			c.Set("relay_mode", relayMode)
		}
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1beta/models/") || strings.HasPrefix(c.Request.URL.Path, "/v1/models/") {
		// ========== Gemini API 路径处理 ==========
		// 路径格式: /v1beta/models/gemini-2.0-flash:generateContent
		relayMode := relayconstant.RelayModeGemini
		modelName := extractModelNameFromGeminiPath(c.Request.URL.Path)
		if modelName != "" {
			modelRequest.Model = modelName
		}
		c.Set("relay_mode", relayMode)
	} else if !strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") && !strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		// ========== 标准 OpenAI API 路径处理 ==========
		// 排除 audio/transcriptions（使用 multipart/form-data）和 multipart/form-data 类型
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
	}
	// ========== WebSocket Realtime API 处理 ==========
	if strings.HasPrefix(c.Request.URL.Path, "/v1/realtime") {
		// WebSocket 路径格式: wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01
		modelRequest.Model = c.Query("model")
	}
	// ========== Moderations API 处理 ==========
	if strings.HasPrefix(c.Request.URL.Path, "/v1/moderations") {
		if modelRequest.Model == "" {
			modelRequest.Model = "text-moderation-stable"
		}
	}
	// ========== Embeddings API 处理 ==========
	if strings.HasSuffix(c.Request.URL.Path, "embeddings") {
		if modelRequest.Model == "" {
			modelRequest.Model = c.Param("model")
		}
	}
	// ========== Images API 处理 ==========
	if strings.HasPrefix(c.Request.URL.Path, "/v1/images/generations") {
		modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "dall-e")
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1/images/edits") {
		contentType := c.ContentType()
		if slices.Contains([]string{gin.MIMEPOSTForm, gin.MIMEMultipartPOSTForm}, contentType) {
			req, err := getModelFromRequest(c)
			if err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
		}
	}
	// ========== Audio API 处理 ==========
	if strings.HasPrefix(c.Request.URL.Path, "/v1/audio") {
		relayMode := relayconstant.RelayModeAudioSpeech
		if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/speech") {
			// TTS 语音合成，默认模型 tts-1
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "tts-1")
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/translations") {
			// 音频翻译，默认模型 whisper-1
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranslation
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") {
			// 音频转录，默认模型 whisper-1
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranscription
		}
		c.Set("relay_mode", relayMode)
	}
	// ========== Playground API 处理 ==========
	if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") {
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
		modelRequest.Group = req.Group
		common.SetContextKey(c, constant.ContextKeyTokenGroup, modelRequest.Group)
	}
	// ========== Compact Responses API 处理 ==========
	// 为 compact 模型名添加后缀标识
	if strings.HasPrefix(c.Request.URL.Path, "/v1/responses/compact") && modelRequest.Model != "" {
		modelRequest.Model = ratio_setting.WithCompactModelSuffix(modelRequest.Model)
	}
	return &modelRequest, shouldSelectChannel, nil
}

// SetupContextForSelectedChannel 将选中渠道的配置信息写入请求上下文
// 这是渠道分发的最后一步，将渠道的所有配置信息设置到 Gin 上下文中，
// 供后续的 relay 处理函数使用
//
// 处理逻辑：
// 1. 设置渠道基础信息（ID、名称、类型等）
// 2. 根据凭证模式（CredentialMode）选择认证方式：
//   - SingleKey: 使用渠道自身的 Key
//   - MultiKey: 使用渠道的多个 Key 中的下一个
//   - AccountPool: 使用渠道关联的账号池中的账号
//   - GlobalAccountPool: 使用全局账号池中的账号
//
// 3. 将渠道配置（设置、覆盖参数、模型映射等）写入上下文
//
// 参数：
//   - c: Gin 上下文
//   - channel: 选中的渠道
//   - modelName: 请求的模型名称
//
// 返回值：
//   - *types.NexusTokError: 设置错误，nil 表示成功
func SetupContextForSelectedChannel(c *gin.Context, channel *model.Channel, modelName string) *types.NexusTokError {
	c.Set("original_model", modelName) // for retry
	if channel == nil {
		return types.NewError(errors.New("channel is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	service.ReleaseSelectedChannelAccount(c)
	service.ReleaseSelectedPoolAccount(c)
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelCreateTime, channel.CreatedTime)
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, channel.GetAutoBan())
	common.SetContextKey(c, constant.ContextKeyChannelCredentialMode, channel.GetCredentialMode())
	common.SetContextKey(c, constant.ContextKeyChannelAccountPool, false)
	common.SetContextKey(c, constant.ContextKeyChannelAccountId, 0)
	common.SetContextKey(c, constant.ContextKeyChannelAccountName, "")
	common.SetContextKey(c, constant.ContextKeyPoolGroupId, 0)
	common.SetContextKey(c, constant.ContextKeyPoolGroupName, "")
	common.SetContextKey(c, constant.ContextKeyPoolAccountId, 0)
	common.SetContextKey(c, constant.ContextKeyPoolAccountName, "")
	common.SetContextKey(c, constant.ContextKeyPoolAccountAuthType, "")

	credentialMode := channel.GetCredentialMode()
	if credentialMode == constant.ChannelCredentialModeAccountPool {
		usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		account, err := service.SelectChannelAccount(c, channel, modelName, usingGroup, c.GetInt("relay_mode"))
		if err != nil {
			if !channel.ChannelInfo.AccountPoolFallback {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeChannelNoAvailableKey, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
			}
		} else {
			applyChannelContext(c, channel, account)
			common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
			common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 0)
			common.SetContextKey(c, constant.ContextKeyChannelKey, account.Key)
			common.SetContextKey(c, constant.ContextKeyChannelAccountPool, true)
			common.SetContextKey(c, constant.ContextKeyChannelAccountId, account.Id)
			common.SetContextKey(c, constant.ContextKeyChannelAccountName, account.Name)
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)
			return nil
		}
	}
	if credentialMode == constant.ChannelCredentialModeGlobalAccountPool {
		usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		group, err := model.GetAccountPoolGroupById(channel.ChannelInfo.AccountPoolGroupId)
		if err != nil || group == nil || group.Status != common.ChannelStatusEnabled {
			return types.NewErrorWithStatusCode(service.ErrNoAvailablePoolAccount, types.ErrorCodeChannelNoAvailableKey, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
		}
		group, account, err := service.SelectPoolAccount(c, channel, modelName, usingGroup, c.GetInt("relay_mode"))
		if err != nil {
			if !channel.ChannelInfo.AccountPoolFallback {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeChannelNoAvailableKey, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
			}
		} else {
			channelKey, err := service.BuildPoolAccountChannelKey(account)
			if err != nil {
				if !channel.ChannelInfo.AccountPoolFallback {
					return types.NewErrorWithStatusCode(err, types.ErrorCodeChannelNoAvailableKey, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
				}
			} else {
				applyPoolAccountContext(c, channel, group, account)
				common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
				common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 0)
				common.SetContextKey(c, constant.ContextKeyChannelKey, channelKey)
				common.SetContextKey(c, constant.ContextKeyChannelAccountPool, true)
				common.SetContextKey(c, constant.ContextKeyPoolGroupId, group.Id)
				common.SetContextKey(c, constant.ContextKeyPoolGroupName, group.Name)
				common.SetContextKey(c, constant.ContextKeyPoolAccountId, account.Id)
				common.SetContextKey(c, constant.ContextKeyPoolAccountName, account.Name)
				common.SetContextKey(c, constant.ContextKeyPoolAccountAuthType, account.AuthType)
				common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)
				return nil
			}
		}
	}

	applyChannelContext(c, channel, nil)
	if credentialMode == constant.ChannelCredentialModeSingleKey {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
		common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 0)
		common.SetContextKey(c, constant.ContextKeyChannelKey, channel.Key)
		common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)
		return nil
	}
	key, index, newAPIError := channel.GetNextEnabledKey()
	if newAPIError != nil {
		return newAPIError
	}
	if channel.ChannelInfo.IsMultiKey {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
		common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, index)
	} else {
		// 必须设置为 false，否则在重试到单个 key 的时候会导致日志显示错误
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
	}
	// c.Request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	common.SetContextKey(c, constant.ContextKeyChannelKey, key)

	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)
	return nil
}

// applyChannelContext 将渠道配置应用到请求上下文
// 处理渠道设置、参数覆盖、头部覆盖、模型映射等配置
// 当存在账号（AccountPool 模式）时，账号配置会覆盖渠道配置
//
// 参数：
//   - c: Gin 上下文
//   - channel: 渠道对象
//   - account: 渠道账号（可为 nil，表示使用渠道自身的配置）
func applyChannelContext(c *gin.Context, channel *model.Channel, account *model.ChannelAccount) {
	// 设置渠道设置（合并账号设置）
	common.SetContextKey(c, constant.ContextKeyChannelSetting, resolveChannelSetting(channel, account))
	// 设置其他设置
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, resolveChannelOtherSettings(channel, account))
	// 解析参数覆盖和头部覆盖
	paramOverride := resolveChannelParamOverride(channel, account)
	headerOverride := resolveChannelHeaderOverride(channel, account)
	// 应用渠道亲和性覆盖模板（如果存在）
	if mergedParam, applied := service.ApplyChannelAffinityOverrideTemplate(c, paramOverride); applied {
		paramOverride = mergedParam
	}
	// 设置参数覆盖、头部覆盖、组织、模型映射等
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, paramOverride)
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, headerOverride)
	common.SetContextKey(c, constant.ContextKeyChannelOrganization, resolveChannelOrganization(channel, account))
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, resolveChannelModelMapping(channel, account))
	common.SetContextKey(c, constant.ContextKeyChannelStatusCodeMapping, resolveChannelStatusCodeMapping(channel, account))
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, resolveChannelBaseURL(channel, account))

	// 根据渠道类型设置特殊字段
	channelOther := resolveChannelOther(channel, account)
	c.Set("api_version", "")
	c.Set("region", "")
	c.Set("plugin", "")
	c.Set("bot_id", "")
	switch channel.Type {
	case constant.ChannelTypeAzure:
		c.Set("api_version", channelOther) // Azure API 版本
	case constant.ChannelTypeVertexAi:
		c.Set("region", channelOther) // Vertex AI 区域
	case constant.ChannelTypeXunfei:
		c.Set("api_version", channelOther) // 讯飞 API 版本
	case constant.ChannelTypeGemini:
		c.Set("api_version", channelOther) // Gemini API 版本
	case constant.ChannelTypeAli:
		c.Set("plugin", channelOther) // 阿里云插件
	case constant.ChannelCloudflare:
		c.Set("api_version", channelOther) // Cloudflare API 版本
	case constant.ChannelTypeMokaAI:
		c.Set("api_version", channelOther) // MokaAI API 版本
	case constant.ChannelTypeCoze:
		c.Set("bot_id", channelOther) // Coze 机器人 ID
	}
}

// applyPoolAccountContext 将账号池账号的配置应用到请求上下文
// 与 applyChannelContext 类似，但配置来源是账号池组和账号池账号
// 配置优先级：账号池账号 > 账号池组 > 渠道
//
// 参数：
//   - c: Gin 上下文
//   - channel: 渠道对象
//   - group: 账号池组对象
//   - account: 账号池账号对象
func applyPoolAccountContext(c *gin.Context, channel *model.Channel, group *model.AccountPoolGroup, account *model.PoolAccount) {
	common.SetContextKey(c, constant.ContextKeyChannelSetting, resolvePoolChannelSetting(channel, group, account))
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, resolvePoolChannelOtherSettings(channel, account))
	paramOverride := resolvePoolChannelParamOverride(channel, account)
	headerOverride := resolvePoolChannelHeaderOverride(channel, account)
	if mergedParam, applied := service.ApplyChannelAffinityOverrideTemplate(c, paramOverride); applied {
		paramOverride = mergedParam
	}
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, paramOverride)
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, headerOverride)
	common.SetContextKey(c, constant.ContextKeyChannelOrganization, resolvePoolChannelOrganization(channel, account))
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, resolvePoolChannelModelMapping(channel, group, account))
	common.SetContextKey(c, constant.ContextKeyChannelStatusCodeMapping, resolvePoolChannelStatusCodeMapping(channel, account))
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, resolvePoolChannelBaseURL(channel, account))

	channelOther := resolvePoolChannelOther(channel, account)
	c.Set("api_version", "")
	c.Set("region", "")
	c.Set("plugin", "")
	c.Set("bot_id", "")
	switch channel.Type {
	case constant.ChannelTypeAzure:
		c.Set("api_version", channelOther)
	case constant.ChannelTypeVertexAi:
		c.Set("region", channelOther)
	case constant.ChannelTypeXunfei:
		c.Set("api_version", channelOther)
	case constant.ChannelTypeGemini:
		c.Set("api_version", channelOther)
	case constant.ChannelTypeAli:
		c.Set("plugin", channelOther)
	case constant.ChannelCloudflare:
		c.Set("api_version", channelOther)
	case constant.ChannelTypeMokaAI:
		c.Set("api_version", channelOther)
	case constant.ChannelTypeCoze:
		c.Set("bot_id", channelOther)
	}
}

// applyCLIProxyAccountPoolContext 应用 CLI Proxy 账号池上下文
// CLI Proxy 是一种特殊的账号池模式，使用外部 CLI 工具代理请求
// 配置来源：渠道 > 账号池组（头部覆盖合并）
//
// 参数：
//   - c: Gin 上下文
//   - channel: 渠道对象
//   - group: 账号池组对象
func applyCLIProxyAccountPoolContext(c *gin.Context, channel *model.Channel, group *model.AccountPoolGroup) {
	common.SetContextKey(c, constant.ContextKeyChannelSetting, resolvePoolChannelSetting(channel, group, nil))
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, channel.GetOtherSettings())
	paramOverride := channel.GetParamOverride()
	if mergedParam, applied := service.ApplyChannelAffinityOverrideTemplate(c, paramOverride); applied {
		paramOverride = mergedParam
	}
	headerOverride := service.MergeHeaderOverrides(channel.GetHeaderOverride(), service.BuildCLIProxyGroupHeaderOverride(group))
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, paramOverride)
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, headerOverride)
	if channel.OpenAIOrganization != nil {
		common.SetContextKey(c, constant.ContextKeyChannelOrganization, *channel.OpenAIOrganization)
	} else {
		common.SetContextKey(c, constant.ContextKeyChannelOrganization, "")
	}
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, resolvePoolChannelModelMapping(channel, group, nil))
	common.SetContextKey(c, constant.ContextKeyChannelStatusCodeMapping, channel.GetStatusCodeMapping())
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, service.AccountPoolCLIProxyURL())

	c.Set("api_version", "")
	c.Set("region", "")
	c.Set("plugin", "")
	c.Set("bot_id", "")
}

// resolveChannelSetting 解析渠道设置
// 优先使用账号设置，如果账号没有设置则使用渠道设置
//
// 参数：
//   - channel: 渠道对象
//   - account: 渠道账号（可为 nil）
//
// 返回值：
//   - dto.ChannelSettings: 合并后的渠道设置
func resolveChannelSetting(channel *model.Channel, account *model.ChannelAccount) dto.ChannelSettings {
	setting := channel.GetSetting()
	if account == nil || account.Setting == nil || strings.TrimSpace(*account.Setting) == "" {
		return setting
	}
	if err := common.Unmarshal([]byte(*account.Setting), &setting); err != nil {
		common.SysLog(fmt.Sprintf("failed to unmarshal channel account setting: account_id=%d, error=%v", account.Id, err))
	}
	return setting
}

// resolveChannelOtherSettings 解析渠道其他设置
// 优先使用账号的其他设置，如果账号没有则使用渠道的
func resolveChannelOtherSettings(channel *model.Channel, account *model.ChannelAccount) dto.ChannelOtherSettings {
	setting := channel.GetOtherSettings()
	if account == nil || strings.TrimSpace(account.OtherSettings) == "" {
		return setting
	}
	if err := common.UnmarshalJsonStr(account.OtherSettings, &setting); err != nil {
		common.SysLog(fmt.Sprintf("failed to unmarshal channel account other settings: account_id=%d, error=%v", account.Id, err))
	}
	return setting
}

// resolveChannelParamOverride 解析渠道参数覆盖
// 参数覆盖用于在转发请求时修改请求参数（如 temperature、max_tokens 等）
func resolveChannelParamOverride(channel *model.Channel, account *model.ChannelAccount) map[string]interface{} {
	override := channel.GetParamOverride()
	if account == nil || account.ParamOverride == nil || strings.TrimSpace(*account.ParamOverride) == "" {
		return override
	}
	override = make(map[string]interface{})
	if err := common.Unmarshal([]byte(*account.ParamOverride), &override); err != nil {
		common.SysLog(fmt.Sprintf("failed to unmarshal channel account param override: account_id=%d, error=%v", account.Id, err))
	}
	return override
}

// resolveChannelHeaderOverride 解析渠道请求头覆盖
// 请求头覆盖用于在转发请求时修改 HTTP 请求头（如添加自定义头、修改认证头等）
func resolveChannelHeaderOverride(channel *model.Channel, account *model.ChannelAccount) map[string]interface{} {
	override := channel.GetHeaderOverride()
	if account == nil || account.HeaderOverride == nil || strings.TrimSpace(*account.HeaderOverride) == "" {
		return override
	}
	override = make(map[string]interface{})
	if err := common.Unmarshal([]byte(*account.HeaderOverride), &override); err != nil {
		common.SysLog(fmt.Sprintf("failed to unmarshal channel account header override: account_id=%d, error=%v", account.Id, err))
	}
	return override
}

// resolveChannelOrganization 解析渠道 OpenAI 组织标识
// 优先使用账号的组织标识，如果账号没有则使用渠道的
func resolveChannelOrganization(channel *model.Channel, account *model.ChannelAccount) string {
	if account != nil && account.OpenAIOrganization != nil && strings.TrimSpace(*account.OpenAIOrganization) != "" {
		return *account.OpenAIOrganization
	}
	if channel.OpenAIOrganization != nil {
		return *channel.OpenAIOrganization
	}
	return ""
}

// resolveChannelModelMapping 解析渠道模型映射
// 模型映射用于将请求中的模型名转换为上游支持的模型名
// 例如：gpt-4 -> gpt-4-turbo
func resolveChannelModelMapping(channel *model.Channel, account *model.ChannelAccount) string {
	if account != nil && account.ModelMapping != nil && strings.TrimSpace(*account.ModelMapping) != "" {
		return *account.ModelMapping
	}
	return channel.GetModelMapping()
}

// resolveChannelStatusCodeMapping 解析渠道状态码映射
// 状态码映射用于将上游返回的 HTTP 状态码转换为其他状态码
func resolveChannelStatusCodeMapping(channel *model.Channel, account *model.ChannelAccount) string {
	if account != nil && account.StatusCodeMapping != nil && strings.TrimSpace(*account.StatusCodeMapping) != "" {
		return *account.StatusCodeMapping
	}
	return channel.GetStatusCodeMapping()
}

// resolveChannelBaseURL 解析渠道基础 URL
// 基础 URL 是上游 API 的根地址
// 优先使用账号的 URL，如果账号没有则使用渠道的
func resolveChannelBaseURL(channel *model.Channel, account *model.ChannelAccount) string {
	if account != nil && account.BaseURL != nil && strings.TrimSpace(*account.BaseURL) != "" {
		return *account.BaseURL
	}
	return channel.GetBaseURL()
}

// resolveChannelOther 解析渠道其他配置
// 其他配置是渠道特定的附加信息，如 API 版本、区域、插件等
func resolveChannelOther(channel *model.Channel, account *model.ChannelAccount) string {
	if account != nil && strings.TrimSpace(account.Other) != "" {
		return account.Other
	}
	return channel.Other
}

// resolvePoolChannelSetting 解析账号池渠道设置
// 配置优先级：账号池账号 > 账号池组 > 渠道
func resolvePoolChannelSetting(channel *model.Channel, group *model.AccountPoolGroup, account *model.PoolAccount) dto.ChannelSettings {
	setting := channel.GetSetting()
	if group != nil && strings.TrimSpace(group.Settings) != "" {
		if err := common.UnmarshalJsonStr(group.Settings, &setting); err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal account pool group setting: group_id=%d, error=%v", group.Id, err))
		}
	}
	if account == nil {
		return setting
	}
	if account.Setting != nil && strings.TrimSpace(*account.Setting) != "" {
		if err := common.Unmarshal([]byte(*account.Setting), &setting); err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal pool account setting: account_id=%d, error=%v", account.Id, err))
		}
	}
	// 账号池账号的 Proxy 是导入文件或登录流程产生的账号级网络配置。
	// Relay 发送上游请求只读取 ChannelSettings.Proxy，因此必须在这里把独立字段合并进去；
	// 否则像 Sub2api 导入的账号即使保存了代理，也会在热路径中退回容器直连上游。
	if proxy := strings.TrimSpace(account.Proxy); proxy != "" {
		setting.Proxy = proxy
	}
	return setting
}

// resolvePoolChannelOtherSettings 解析账号池渠道其他设置
func resolvePoolChannelOtherSettings(channel *model.Channel, account *model.PoolAccount) dto.ChannelOtherSettings {
	setting := channel.GetOtherSettings()
	if account == nil || strings.TrimSpace(account.OtherSettings) == "" {
		return setting
	}
	if err := common.UnmarshalJsonStr(account.OtherSettings, &setting); err != nil {
		common.SysLog(fmt.Sprintf("failed to unmarshal pool account other settings: account_id=%d, error=%v", account.Id, err))
	}
	return setting
}

// resolvePoolChannelParamOverride 解析账号池渠道参数覆盖
func resolvePoolChannelParamOverride(channel *model.Channel, account *model.PoolAccount) map[string]interface{} {
	override := channel.GetParamOverride()
	if account == nil || account.ParamOverride == nil || strings.TrimSpace(*account.ParamOverride) == "" {
		return override
	}
	override = make(map[string]interface{})
	if err := common.Unmarshal([]byte(*account.ParamOverride), &override); err != nil {
		common.SysLog(fmt.Sprintf("failed to unmarshal pool account param override: account_id=%d, error=%v", account.Id, err))
	}
	return override
}

// resolvePoolChannelHeaderOverride 解析账号池渠道请求头覆盖
func resolvePoolChannelHeaderOverride(channel *model.Channel, account *model.PoolAccount) map[string]interface{} {
	override := channel.GetHeaderOverride()
	if account == nil || account.HeaderOverride == nil || strings.TrimSpace(*account.HeaderOverride) == "" {
		return override
	}
	override = make(map[string]interface{})
	if err := common.Unmarshal([]byte(*account.HeaderOverride), &override); err != nil {
		common.SysLog(fmt.Sprintf("failed to unmarshal pool account header override: account_id=%d, error=%v", account.Id, err))
	}
	return override
}

// resolvePoolChannelOrganization 解析账号池渠道 OpenAI 组织标识
func resolvePoolChannelOrganization(channel *model.Channel, account *model.PoolAccount) string {
	if account != nil && account.OpenAIOrganization != nil && strings.TrimSpace(*account.OpenAIOrganization) != "" {
		return *account.OpenAIOrganization
	}
	if channel.OpenAIOrganization != nil {
		return *channel.OpenAIOrganization
	}
	return ""
}

// resolvePoolChannelModelMapping 解析账号池渠道模型映射
// 配置优先级：账号池账号 > 账号池组 > 渠道
func resolvePoolChannelModelMapping(channel *model.Channel, group *model.AccountPoolGroup, account *model.PoolAccount) string {
	if account != nil && account.ModelMapping != nil && strings.TrimSpace(*account.ModelMapping) != "" {
		return *account.ModelMapping
	}
	if group != nil && group.ModelMapping != nil && strings.TrimSpace(*group.ModelMapping) != "" {
		return *group.ModelMapping
	}
	return channel.GetModelMapping()
}

// resolvePoolChannelStatusCodeMapping 解析账号池渠道状态码映射
func resolvePoolChannelStatusCodeMapping(channel *model.Channel, account *model.PoolAccount) string {
	if account != nil && account.StatusCodeMapping != nil && strings.TrimSpace(*account.StatusCodeMapping) != "" {
		return *account.StatusCodeMapping
	}
	return channel.GetStatusCodeMapping()
}

// resolvePoolChannelBaseURL 解析账号池渠道基础 URL
func resolvePoolChannelBaseURL(channel *model.Channel, account *model.PoolAccount) string {
	if account != nil && account.BaseURL != nil && strings.TrimSpace(*account.BaseURL) != "" {
		return *account.BaseURL
	}
	return channel.GetBaseURL()
}

// resolvePoolChannelOther 解析账号池渠道其他配置
func resolvePoolChannelOther(channel *model.Channel, account *model.PoolAccount) string {
	if account != nil && strings.TrimSpace(account.Other) != "" {
		return account.Other
	}
	return channel.Other
}

// extractModelNameFromGeminiPath 从 Gemini API URL 路径中提取模型名
// 输入格式: /v1beta/models/gemini-2.0-flash:generateContent
// 输出: gemini-2.0-flash
func extractModelNameFromGeminiPath(path string) string {
	// 查找 "/models/" 的位置
	modelsPrefix := "/models/"
	modelsIndex := strings.Index(path, modelsPrefix)
	if modelsIndex == -1 {
		return ""
	}

	// 从 "/models/" 之后开始提取
	startIndex := modelsIndex + len(modelsPrefix)
	if startIndex >= len(path) {
		return ""
	}

	// 查找 ":" 的位置，模型名在 ":" 之前
	colonIndex := strings.Index(path[startIndex:], ":")
	if colonIndex == -1 {
		// 如果没有找到 ":"，返回从 "/models/" 到路径结尾的部分
		return path[startIndex:]
	}

	// 返回模型名部分
	return path[startIndex : startIndex+colonIndex]
}
