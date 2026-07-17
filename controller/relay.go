// Package controller - relay.go
// 该文件实现了 AI API 中继的核心控制器
// 负责处理所有 AI API 请求的转发、重试、计费和错误处理
//
// 核心流程：
// 1. 解析和验证请求
// 2. 生成中继信息（RelayInfo）
// 3. 敏感词检查（可选）
// 4. Token 计数和价格计算
// 5. 预扣费
// 6. 选择渠道并转发请求
// 7. 处理响应和结算
// 8. 错误处理和重试
package controller

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"                       // 公共工具包
	"github.com/c1cada/NexusTok/constant"                     // 常量定义
	"github.com/c1cada/NexusTok/dto"                          // 数据传输对象
	"github.com/c1cada/NexusTok/logger"                       // 日志
	"github.com/c1cada/NexusTok/middleware"                   // 中间件
	"github.com/c1cada/NexusTok/model"                        // 数据模型
	perfmetrics "github.com/c1cada/NexusTok/pkg/perf_metrics" // 性能监控
	"github.com/c1cada/NexusTok/relay"                        // 中继层
	relaycommon "github.com/c1cada/NexusTok/relay/common"     // 中继公共包
	relayconstant "github.com/c1cada/NexusTok/relay/constant" // 中继常量
	"github.com/c1cada/NexusTok/relay/helper"                 // 中继辅助函数
	"github.com/c1cada/NexusTok/service"                      // 服务层
	"github.com/c1cada/NexusTok/setting"                      // 设置
	"github.com/c1cada/NexusTok/setting/operation_setting"    // 运营设置
	"github.com/c1cada/NexusTok/types"                        // 类型定义

	"github.com/bytedance/gopkg/util/gopool" // 字节跳动协程池
	"github.com/samber/lo"                   // Go 泛型工具库

	"github.com/gin-gonic/gin"     // Gin 框架
	"github.com/gorilla/websocket" // WebSocket 支持
)

// relayHandler 中继请求处理器
// 根据中继模式（RelayMode）分发到不同的处理函数
//
// 支持的中继模式：
// - 图像生成/编辑：ImageHelper
// - 音频处理：AudioHelper（语音合成、翻译、转录）
// - 重排序：RerankHelper
// - 文本嵌入：EmbeddingHelper
// - Responses API：ResponsesHelper
// - 默认文本处理：TextHelper（聊天补全、文本补全）
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//
// 返回值：
//   - *types.NexusTokError: 错误信息，成功返回 nil
func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NexusTokError {
	var err *types.NexusTokError

	// 根据中继模式分发到对应的处理函数
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		// 图像生成和编辑
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		// 音频处理（语音合成、翻译、转录）
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		// 重排序
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		// 文本嵌入
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		// Responses API
		err = relay.ResponsesHelper(c, info)
	default:
		// 默认：文本处理（聊天补全、文本补全等）
		err = relay.TextHelper(c, info)
	}

	return err
}

// geminiRelayHandler Gemini 中继请求处理器
// 根据请求路径判断是嵌入请求还是普通请求
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//
// 返回值：
//   - *types.NexusTokError: 错误信息，成功返回 nil
func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NexusTokError {
	var err *types.NexusTokError

	// 根据请求路径判断处理类型
	if strings.Contains(c.Request.URL.Path, "embed") {
		// Gemini 嵌入请求
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		// Gemini 普通请求（聊天、生成等）
		err = relay.GeminiHelper(c, info)
	}

	return err
}

// Relay 主中继入口函数
// 这是所有 AI API 请求的核心处理函数，负责：
// 1. WebSocket 升级（如果是 Realtime API）
// 2. 请求验证和解析
// 3. 中继信息生成
// 4. 敏感词检查（可选）
// 5. Token 计数和价格计算
// 6. 预扣费
// 7. 渠道选择和请求转发
// 8. 重试机制
// 9. 错误处理和结算
//
// 参数：
//   - c: Gin 上下文
//   - relayFormat: 中继格式（OpenAI、Claude、Gemini 等）
func Relay(c *gin.Context, relayFormat types.RelayFormat) {
	// 获取请求 ID，用于日志追踪
	requestId := c.GetString(common.RequestIdKey)

	// 错误变量和 WebSocket 连接
	var (
		newAPIError *types.NexusTokError
		ws          *websocket.Conn
	)

	// 如果是 OpenAI Realtime API，需要升级为 WebSocket 连接
	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		// 升级 HTTP 连接为 WebSocket
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	// 延迟错误处理：根据不同的中继格式返回对应的错误响应
	defer func() {
		if newAPIError != nil {
			// 记录错误日志
			logger.LogError(c, fmt.Sprintf("relay error: %s", newAPIError.Error()))
			// 在错误消息中附加请求 ID
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))

			// 根据中继格式返回不同格式的错误响应
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				// WebSocket 错误
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				// Claude 格式错误
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				// OpenAI 格式错误（默认）
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	// ========================================
	// 步骤 1：获取并验证请求
	// ========================================
	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// 请求体过大返回 413
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}

	// ========================================
	// 步骤 2：生成中继信息
	// ========================================
	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}

	// ========================================
	// 步骤 3：敏感词检查和 Token 计数
	// ========================================
	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken

	// 优化：如果不需要 token 计数和敏感词检查，使用快速路径避免构建大字符串
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	// 敏感词检查
	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}

	// ========================================
	// 步骤 4：Token 计数和价格计算
	// ========================================
	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	// 设置预估的 prompt token 数量
	relayInfo.SetEstimatePromptTokens(tokens)

	// 获取模型价格信息
	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
		return
	}

	// ========================================
	// 步骤 5：预扣费
	// ========================================
	if priceData.FreeModel {
		// 免费模型跳过预扣费
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		// 执行预扣费
		newAPIError = service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	// 延迟处理：如果请求失败，退还预扣的配额
	defer func() {
		if newAPIError != nil {
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			}
			service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
		}
	}()

	// ========================================
	// 步骤 6：重试机制和请求转发
	// ========================================
	retryParam := &service.RetryParam{
		Ctx:         c,
		TokenGroup:  relayInfo.TokenGroup,
		ModelName:   relayInfo.OriginModelName,
		RequestPath: c.Request.URL.Path,
		Retry:       common.GetPointer(0),
	}
	relayInfo.RetryIndex = 0
	relayInfo.LastError = nil

	// 重试循环
	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		relayInfo.RetryIndex = retryParam.GetRetry()

		// 选择渠道
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			newAPIError = channelErr
			break
		}

		// 记录使用的渠道
		addUsedChannel(c, channel.Id)

		// 获取请求体存储
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			service.ReleaseSelectedChannelAccount(c)
			service.ReleaseSelectedPoolAccount(c)
			break
		}
		// 重置请求体
		c.Request.Body = io.NopCloser(bodyStorage)

		// 根据中继格式分发到对应的处理函数
		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}

		// 请求成功，退出重试循环
		if newAPIError == nil {
			// 释放选中的账号
			service.ReleaseSelectedChannelAccount(c)
			service.ReleaseSelectedPoolAccount(c)
			relayInfo.LastError = nil
			return
		}

		// 规范化违规费用错误
		newAPIError = service.NormalizeViolationFeeError(newAPIError)
		relayInfo.LastError = newAPIError

		// 处理渠道错误（禁用渠道、记录日志等）
		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		markChannelExcludedForRetry(c, channel.Id)

		// 错误处理需要读取账号池上下文；处理完成后再释放并发槽位。
		service.ReleaseSelectedChannelAccount(c)
		service.ReleaseSelectedPoolAccount(c)

		// 判断是否应该重试
		if !shouldRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}

	// 记录重试日志
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// 如果请求失败，记录性能指标
	if newAPIError != nil {
		gopool.Go(func() {
			perfmetrics.RecordRelaySample(relayInfo, false, 0)
		})
	}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

// markChannelExcludedForRetry 把本轮失败的渠道加入请求级排除集。
//
// 普通渠道失败后必须立即排除，避免自动禁用异步执行尚未完成时下一轮又选中
// 同一渠道。账号池渠道失败时先由账号级排除集在同渠道内切换账号；只有同渠
// 道已经没有可用账号、准备进入渠道级降级时，才排除整个渠道。
func markChannelExcludedForRetry(c *gin.Context, channelId int) {
	if channelId <= 0 {
		return
	}
	if common.GetContextKeyBool(c, constant.ContextKeyChannelAccountPool) {
		return
	}
	service.AddExcludedChannelId(c, channelId)
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		maxCompletionTokens := lo.FromPtrOr(r.MaxCompletionTokens, uint(0))
		maxTokens := lo.FromPtrOr(r.MaxTokens, uint(0))
		if maxCompletionTokens > maxTokens {
			meta.MaxTokens = int(maxCompletionTokens)
		} else {
			meta.MaxTokens = int(maxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(lo.FromPtrOr(r.MaxOutputTokens, uint(0)))
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(lo.FromPtr(r.MaxTokens))
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NexusTokError) {
	if info.ChannelMeta == nil {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		info.InitChannelMeta(c)
		return &model.Channel{
			Id:   c.GetInt("channel_id"),
			Type: c.GetInt("channel_type"),
			Name: c.GetString("channel_name"),
			ChannelInfo: model.ChannelInfo{
				IsMultiKey: common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey),
			},
			AutoBan: &autoBanInt,
		}, nil
	}
	if channel, setupErr, ok := trySetupAccountPoolRetryChannel(c, info); ok {
		return channel, setupErr
	}

	for attempt := 0; attempt < maxChannelSetupSelectionAttempts(); attempt++ {
		channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)

		if err != nil {
			return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, info.OriginModelName, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
		if channel == nil {
			return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}

		newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
		if newAPIError != nil {
			if shouldExcludeSetupFailedChannel(c, newAPIError) {
				service.AddExcludedChannelId(c, channel.Id)
				continue
			}
			return nil, newAPIError
		}
		info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)
		info.InitChannelMeta(c)
		return channel, nil
	}

	return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的候选渠道均不可用（retry）", retryParam.TokenGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
}

// maxChannelSetupSelectionAttempts 限制单次 getChannel 内因 setup 失败而重选渠道的次数。
//
// 正常情况下排除集会让候选很快耗尽并返回“无可用渠道”；这里设置上限只是为了
// 防止缓存/能力表异常导致同一渠道被反复返回，避免请求线程陷入无限循环。
func maxChannelSetupSelectionAttempts() int {
	if common.RetryTimes > 0 {
		return common.RetryTimes + 16
	}
	return 16
}

// shouldExcludeSetupFailedChannel 判断 setup 阶段失败后是否可以继续选择其他渠道。
//
// 只有“候选渠道没有可用 key/账号”属于渠道局部不可用，可排除后继续选路；
// 管理员指定 specific_channel_id 时保持单渠道调试语义，不自动降级。
func shouldExcludeSetupFailedChannel(c *gin.Context, err *types.NexusTokError) bool {
	if err == nil || err.GetErrorCode() != types.ErrorCodeChannelNoAvailableKey {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	return true
}

// trySetupAccountPoolRetryChannel 在账号失败后的重试中优先复用当前渠道，避免直接跳出账号池。
func trySetupAccountPoolRetryChannel(c *gin.Context, info *relaycommon.RelayInfo) (*model.Channel, *types.NexusTokError, bool) {
	if info == nil {
		return nil, nil, false
	}
	retryChannelID := common.GetContextKeyInt(c, constant.ContextKeyChannelAccountRetryChannelId)
	if retryChannelID <= 0 || (len(service.GetExcludedChannelAccountIds(c)) == 0 && len(service.GetExcludedPoolAccountIds(c)) == 0) {
		return nil, nil, false
	}
	usingGroup := getAccountPoolRetryGroup(c, info)
	if usingGroup == "" || !model.IsChannelEnabledForGroupModel(usingGroup, info.OriginModelName, retryChannelID) {
		common.SetContextKey(c, constant.ContextKeyChannelAccountRetryChannelId, 0)
		return nil, nil, false
	}
	channel, err := model.CacheGetChannel(retryChannelID)
	if err != nil || channel == nil || channel.Status != common.ChannelStatusEnabled || !channel.IsAccountPoolEnabled() {
		common.SetContextKey(c, constant.ContextKeyChannelAccountRetryChannelId, 0)
		return nil, nil, false
	}
	setupErr := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
	if setupErr != nil {
		if setupErr.GetErrorCode() == types.ErrorCodeChannelNoAvailableKey {
			common.SetContextKey(c, constant.ContextKeyChannelAccountRetryChannelId, 0)
			service.AddExcludedChannelId(c, retryChannelID)
			return nil, nil, false
		}
		return nil, setupErr, true
	}
	// 账号池账号失败后的第一次重试，应优先在同一渠道内切换账号。
	info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)
	info.InitChannelMeta(c)
	return channel, nil, true
}

// getAccountPoolRetryGroup 返回账号池重试应使用的实际分组。
func getAccountPoolRetryGroup(c *gin.Context, info *relaycommon.RelayInfo) string {
	if autoGroup := common.GetContextKeyString(c, constant.ContextKeyAutoGroup); autoGroup != "" {
		return autoGroup
	}
	if usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup); usingGroup != "" && usingGroup != "auto" {
		return usingGroup
	}
	if info != nil && info.UsingGroup != "auto" {
		return info.UsingGroup
	}
	return ""
}

func shouldRetry(c *gin.Context, openaiErr *types.NexusTokError, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	code := openaiErr.StatusCode
	if code >= 200 && code < 300 {
		return false
	}
	if code < 100 || code > 599 {
		return true
	}
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}
	return operation_setting.ShouldRetryByStatusCode(code)
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NexusTokError) {
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, err.Error()))
	channelError.CredentialMode = common.GetContextKeyString(c, constant.ContextKeyChannelCredentialMode)
	channelError.AccountPool = common.GetContextKeyBool(c, constant.ContextKeyChannelAccountPool)
	channelError.ChannelAccountId = common.GetContextKeyInt(c, constant.ContextKeyChannelAccountId)
	channelError.ChannelAccountName = common.GetContextKeyString(c, constant.ContextKeyChannelAccountName)
	channelError.PoolGroupId = common.GetContextKeyInt(c, constant.ContextKeyPoolGroupId)
	channelError.PoolGroupName = common.GetContextKeyString(c, constant.ContextKeyPoolGroupName)
	channelError.PoolAccountId = common.GetContextKeyInt(c, constant.ContextKeyPoolAccountId)
	channelError.PoolAccountName = common.GetContextKeyString(c, constant.ContextKeyPoolAccountName)
	channelError.PoolAccountAuthType = common.GetContextKeyString(c, constant.ContextKeyPoolAccountAuthType)

	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if channelError.AccountPool && channelError.PoolAccountId > 0 {
		service.ProcessPoolAccountError(c, channelError, err)
	} else if channelError.AccountPool && channelError.ChannelAccountId > 0 {
		service.ProcessChannelAccountError(c, channelError, err)
	} else if service.ShouldDisableChannel(err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.ErrorWithStatusCode())
		})
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		if credentialMode := common.GetContextKeyString(c, constant.ContextKeyChannelCredentialMode); credentialMode != "" {
			adminInfo["credential_mode"] = credentialMode
		}
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		if common.GetContextKeyBool(c, constant.ContextKeyChannelAccountPool) {
			adminInfo["account_pool"] = true
			adminInfo["channel_account_id"] = common.GetContextKeyInt(c, constant.ContextKeyChannelAccountId)
			adminInfo["channel_account_name"] = common.GetContextKeyString(c, constant.ContextKeyChannelAccountName)
			if poolAccountID := common.GetContextKeyInt(c, constant.ContextKeyPoolAccountId); poolAccountID > 0 {
				adminInfo["pool_group_id"] = common.GetContextKeyInt(c, constant.ContextKeyPoolGroupId)
				adminInfo["pool_group_name"] = common.GetContextKeyString(c, constant.ContextKeyPoolGroupName)
				adminInfo["pool_account_id"] = poolAccountID
				adminInfo["pool_account_name"] = common.GetContextKeyString(c, constant.ContextKeyPoolAccountName)
				adminInfo["pool_account_auth_type"] = common.GetContextKeyString(c, constant.ContextKeyPoolAccountAuthType)
			}
		}
		service.AppendChannelAffinityAdminInfo(c, adminInfo)
		other["admin_info"] = adminInfo
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
	}

}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "nexustok_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTaskFetch(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

func RelayTask(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}

	var result *relay.TaskSubmitResult
	var taskErr *dto.TaskError
	defer func() {
		if taskErr != nil && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:         c,
		TokenGroup:  relayInfo.TokenGroup,
		ModelName:   relayInfo.OriginModelName,
		RequestPath: c.Request.URL.Path,
		Retry:       common.GetPointer(0),
	}

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		var channel *model.Channel

		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			channel = lockedCh
			if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
				taskErr = service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError)
				break
			}
		} else {
			var channelErr *types.NexusTokError
			channel, channelErr = getChannel(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", http.StatusInternalServerError)
				break
			}
		}

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
			}
			service.ReleaseSelectedChannelAccount(c)
			service.ReleaseSelectedPoolAccount(c)
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		result, taskErr = relay.RelayTaskSubmit(c, relayInfo)
		if taskErr == nil {
			service.ReleaseSelectedChannelAccount(c)
			service.ReleaseSelectedPoolAccount(c)
			break
		}

		if !taskErr.LocalError {
			processChannelError(c,
				*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
					common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
				types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode))
			markChannelExcludedForRetry(c, channel.Id)
		}
		service.ReleaseSelectedChannelAccount(c)
		service.ReleaseSelectedPoolAccount(c)

		if !shouldRetryTaskRelay(c, channel.Id, taskErr, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// ── 成功：结算 + 日志 + 插入任务 ──
	if taskErr == nil {
		if settleErr := service.SettleBilling(c, relayInfo, result.Quota); settleErr != nil {
			common.SysError("settle task billing error: " + settleErr.Error())
		}
		service.LogTaskConsumption(c, relayInfo)

		task := model.InitTask(result.Platform, relayInfo)
		task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
		task.PrivateData.BillingSource = relayInfo.BillingSource
		task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
		task.PrivateData.TokenId = relayInfo.TokenId
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			ModelPrice:      relayInfo.PriceData.ModelPrice,
			GroupRatio:      relayInfo.PriceData.GroupRatioInfo.GroupRatio,
			ModelRatio:      relayInfo.PriceData.ModelRatio,
			OtherRatios:     relayInfo.PriceData.OtherRatios,
			OriginModelName: relayInfo.OriginModelName,
			PerCallBilling:  common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice,
		}
		task.Quota = result.Quota
		task.Data = result.TaskData
		task.Action = relayInfo.Action
		if insertErr := task.Insert(); insertErr != nil {
			common.SysError("insert task error: " + insertErr.Error())
		}
	}

	if taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

// respondTaskError 统一输出 Task 错误响应（含 429 限流提示改写）
func respondTaskError(c *gin.Context, taskErr *dto.TaskError) {
	if taskErr.StatusCode == http.StatusTooManyRequests {
		taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
	}
	c.JSON(taskErr.StatusCode, taskErr)
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode) {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}
