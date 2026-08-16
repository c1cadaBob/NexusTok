// Package controller - channel-test.go
// 该文件实现了渠道测试功能
//
// 渠道测试用于验证渠道配置是否正确，以及上游服务是否可用
//
// 测试流程：
// 1. 构建测试请求（根据模型类型选择合适的请求格式）
// 2. 选择适配器并转换请求格式
// 3. 发送请求到上游服务
// 4. 解析响应并验证
// 5. 记录测试结果和耗时
//
// 支持的测试类型：
// - Chat Completion：文本对话测试
// - Embedding：文本嵌入测试
// - Image Generation：图像生成测试
// - Rerank：重排序测试
// - Responses：响应式 API 测试
//
// 自动测试：
// - 支持定时自动测试所有渠道
// - 测试失败可自动禁用渠道
// - 测试成功可自动启用渠道
package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/pkg/billingexpr"
	"github.com/c1cada/NexusTok/relay"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/service/upstreamaccount"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"github.com/c1cada/NexusTok/types"

	"github.com/samber/lo"
	"github.com/tidwall/gjson"

	"github.com/gin-gonic/gin"
)

// testResult 渠道测试结果
type testResult struct {
	context                 *gin.Context         // 测试上下文
	localErr                error                // 本地错误
	newAPIError             *types.NexusTokError // API 错误
	countForAutoDisable     bool                 // 是否属于可归因到指定同步密钥的真实上游失败
	autoCheckFailureCount   int                  // 指定同步密钥连续失败次数
	autoCheckDisabled       bool                 // 本次测试是否触发自动禁用
	autoCheckFailureCounted bool                 // 本次测试是否已写入自动检查失败计数
	recovery                channelTestRecovery  // 指定同步密钥测试成功后的恢复结果
}

// channelTestRecovery 描述管理员手动测试成功后恢复同步密钥和渠道的结果。
//
// 字段会同时写入接口响应和模型测试日志。旧前端可以忽略这些字段，新前端据此刷新列表
// 并给出“密钥已恢复/渠道已恢复”的提示。状态值沿用 common.ChannelStatus* 常量。
type channelTestRecovery struct {
	SelectedAccountID   int
	FailureCount        int
	Updated             bool
	AccountRecovered    bool
	ChannelRecovered    bool
	AccountStatusBefore int
	AccountStatusAfter  int
	ChannelStatusBefore int
	ChannelStatusAfter  int
}

func normalizeChannelTestEndpoint(channel *model.Channel, modelName, endpointType string) string {
	normalized := strings.TrimSpace(endpointType)
	if normalized != "" {
		return normalized
	}
	if strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		return string(constant.EndpointTypeOpenAIResponseCompact)
	}
	if channel != nil && channel.Type == constant.ChannelTypeCodex {
		return string(constant.EndpointTypeOpenAIResponse)
	}
	return normalized
}

// shouldUseStreamForChannelTest 统一决定渠道测试是否启用流式模式。
// Codex 后端的 responses 接口要求 stream=true；手动测试时前端开关可能关闭，
// 因此后端在这里强制启用，避免管理员看到“非流式测试失败”的误导性结果。
func shouldUseStreamForChannelTest(channel *model.Channel, requested bool) bool {
	if channel != nil && channel.Type == constant.ChannelTypeCodex {
		return true
	}
	return requested
}

// resolveChannelTestUserID 返回渠道测试使用的用户 ID。
//
// 手动单渠道测试优先沿用当前请求用户，保证日志、分组和上下文与操作者一致；
// 系统任务等后台批量测试没有 HTTP 用户上下文时，回退到 Root 用户。这样可以避免
// 旧实现固定使用 ID=1 时在导入数据、迁移或首个 Root 用户 ID 不为 1 的环境中失败。
func resolveChannelTestUserID(c *gin.Context) (int, error) {
	if c != nil {
		if userID := c.GetInt("id"); userID > 0 {
			return userID, nil
		}
	}

	var rootUser model.User
	if err := model.DB.Select("id").Where("role = ?", common.RoleRootUser).First(&rootUser).Error; err != nil {
		return 0, fmt.Errorf("failed to resolve channel test user: %w", err)
	}
	if rootUser.Id == 0 {
		return 0, errors.New("failed to resolve channel test user")
	}
	return rootUser.Id, nil
}

func testChannel(ctx context.Context, channel *model.Channel, testUserID int, testModel string, endpointType string, isStream bool, selectedAccountID int) testResult {
	result := testChannelOnce(ctx, channel, testUserID, testModel, endpointType, isStream, selectedAccountID, nil)
	if result.newAPIError == nil || result.context == nil {
		return result
	}
	modelName := strings.TrimSpace(testModel)
	if modelName == "" && result.context != nil {
		modelName = strings.TrimSpace(result.context.GetString("original_model"))
	}
	if !middleware.TryPrepareEndpointAutoConversionAfterFailure(result.context, modelName, result.newAPIError) {
		return result
	}
	conversion, ok := relaycommon.GetEndpointAutoConversion(result.context)
	if !ok || conversion == nil {
		return result
	}
	fallbackEndpoint := string(conversion.ToEndpoint)
	fallback := testChannelOnce(ctx, channel, testUserID, testModel, fallbackEndpoint, isStream, selectedAccountID, conversion)
	if fallback.context != nil {
		common.SetContextKey(fallback.context, constant.ContextKeyEndpointAutoConversion, conversion)
	}
	return fallback
}

func testChannelOnce(ctx context.Context, channel *model.Channel, testUserID int, testModel string, endpointType string, isStream bool, selectedAccountID int, presetConversion *relaycommon.EndpointAutoConversion) testResult {
	if ctx == nil {
		ctx = context.Background()
	}
	tik := time.Now()
	var unsupportedTestChannelTypes = []int{
		constant.ChannelTypeMidjourney,
		constant.ChannelTypeMidjourneyPlus,
		constant.ChannelTypeSunoAPI,
		constant.ChannelTypeKling,
		constant.ChannelTypeJimeng,
		constant.ChannelTypeDoubaoVideo,
		constant.ChannelTypeVidu,
	}
	if lo.Contains(unsupportedTestChannelTypes, channel.Type) {
		channelTypeName := constant.GetChannelTypeName(channel.Type)
		return testResult{
			localErr: fmt.Errorf("%s channel test is not supported", channelTypeName),
		}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	testModel = strings.TrimSpace(testModel)
	if testModel == "" {
		if channel.TestModel != nil && *channel.TestModel != "" {
			testModel = strings.TrimSpace(*channel.TestModel)
		} else {
			models := channel.GetModels()
			if len(models) > 0 {
				testModel = strings.TrimSpace(models[0])
			}
			if testModel == "" {
				testModel = "gpt-4o-mini"
			}
		}
	}
	isStream = shouldUseStreamForChannelTest(channel, isStream)

	endpointType = normalizeChannelTestEndpoint(channel, testModel, endpointType)

	requestPath := "/v1/chat/completions"

	// 如果指定了端点类型，使用指定的端点类型
	if endpointType != "" {
		if endpointInfo, ok := common.GetDefaultEndpointInfo(constant.EndpointType(endpointType)); ok {
			requestPath = endpointInfo.Path
		}
	} else {
		// 如果没有指定端点类型，使用原有的自动检测逻辑

		if strings.Contains(strings.ToLower(testModel), "rerank") {
			requestPath = "/v1/rerank"
		}

		// 先判断是否为 Embedding 模型
		if strings.Contains(strings.ToLower(testModel), "embedding") ||
			strings.HasPrefix(testModel, "m3e") || // m3e 系列模型
			strings.Contains(testModel, "bge-") || // bge 系列模型
			strings.Contains(testModel, "embed") ||
			channel.Type == constant.ChannelTypeMokaAI { // 其他 embedding 模型
			requestPath = "/v1/embeddings" // 修改请求路径
		}

		// VolcEngine 图像生成模型
		if channel.Type == constant.ChannelTypeVolcEngine && strings.Contains(testModel, "seedream") {
			requestPath = "/v1/images/generations"
		}

		// responses-only models
		if strings.Contains(strings.ToLower(testModel), "codex") {
			requestPath = "/v1/responses"
		}

		// responses compaction models (must use /v1/responses/compact)
		if strings.HasSuffix(testModel, ratio_setting.CompactModelSuffix) {
			requestPath = "/v1/responses/compact"
		}
	}
	if strings.HasPrefix(requestPath, "/v1/responses/compact") {
		testModel = ratio_setting.WithCompactModelSuffix(testModel)
	}

	c.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, requestPath, nil)
	if presetConversion != nil {
		common.SetContextKey(c, constant.ContextKeyEndpointAutoConversion, presetConversion)
	}

	cache, err := model.GetUserCache(testUserID)
	if err != nil {
		return testResult{
			localErr:    err,
			newAPIError: nil,
		}
	}
	cache.WriteContext(c)
	c.Set("id", testUserID)

	//c.Request.Header.Set("Authorization", "Bearer "+channel.Key)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("channel", channel.Type)
	c.Set("base_url", channel.GetBaseURL())
	group, _ := model.GetUserGroup(testUserID, false)
	c.Set("group", group)
	if selectedAccountID > 0 {
		common.SetContextKey(c, constant.ContextKeyRequestedChannelAccountId, selectedAccountID)
		common.SetContextKey(c, constant.ContextKeyAllowDisabledChannelAccountTest, true)
	}

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, testModel)
	if newAPIError != nil {
		return testResult{
			context:     c,
			localErr:    newAPIError,
			newAPIError: newAPIError,
		}
	}

	// Determine relay format based on endpoint type or request path
	var relayFormat types.RelayFormat
	if endpointType != "" {
		// 根据指定的端点类型设置 relayFormat
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeOpenAI:
			relayFormat = types.RelayFormatOpenAI
		case constant.EndpointTypeOpenAIResponse:
			relayFormat = types.RelayFormatOpenAIResponses
		case constant.EndpointTypeOpenAIResponseCompact:
			relayFormat = types.RelayFormatOpenAIResponsesCompaction
		case constant.EndpointTypeAnthropic:
			relayFormat = types.RelayFormatClaude
		case constant.EndpointTypeGemini:
			relayFormat = types.RelayFormatGemini
		case constant.EndpointTypeJinaRerank:
			relayFormat = types.RelayFormatRerank
		case constant.EndpointTypeImageGeneration:
			relayFormat = types.RelayFormatOpenAIImage
		case constant.EndpointTypeEmbeddings:
			relayFormat = types.RelayFormatEmbedding
		default:
			relayFormat = types.RelayFormatOpenAI
		}
	} else {
		// 根据请求路径自动检测
		relayFormat = types.RelayFormatOpenAI
		if c.Request.URL.Path == "/v1/embeddings" {
			relayFormat = types.RelayFormatEmbedding
		}
		if c.Request.URL.Path == "/v1/images/generations" {
			relayFormat = types.RelayFormatOpenAIImage
		}
		if c.Request.URL.Path == "/v1/messages" {
			relayFormat = types.RelayFormatClaude
		}
		if strings.Contains(c.Request.URL.Path, "/v1beta/models") {
			relayFormat = types.RelayFormatGemini
		}
		if c.Request.URL.Path == "/v1/rerank" || c.Request.URL.Path == "/rerank" {
			relayFormat = types.RelayFormatRerank
		}
		if c.Request.URL.Path == "/v1/responses" {
			relayFormat = types.RelayFormatOpenAIResponses
		}
		if strings.HasPrefix(c.Request.URL.Path, "/v1/responses/compact") {
			relayFormat = types.RelayFormatOpenAIResponsesCompaction
		}
	}

	request := buildTestRequest(testModel, endpointType, channel, isStream)

	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, nil)

	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeGenRelayInfoFailed),
		}
	}

	info.IsChannelTest = true
	info.InitChannelMeta(c)

	err = attachTestBillingRequestInput(info, request)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeJsonMarshalFailed),
		}
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeChannelModelMappedError),
		}
	}

	testModel = info.UpstreamModelName
	// 更新请求中的模型名称
	request.SetModelName(testModel)

	apiType, _ := common.ChannelType2APIType(channel.Type)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact &&
		apiType != constant.APITypeOpenAI &&
		apiType != constant.APITypeCodex {
		return testResult{
			context:     c,
			localErr:    fmt.Errorf("responses compaction test only supports openai/codex channels, got api type %d", apiType),
			newAPIError: types.NewError(fmt.Errorf("unsupported api type: %d", apiType), types.ErrorCodeInvalidApiType),
		}
	}
	adaptor := relay.GetAdaptor(apiType)
	if adaptor == nil {
		return testResult{
			context:     c,
			localErr:    fmt.Errorf("invalid api type: %d, adaptor is nil", apiType),
			newAPIError: types.NewError(fmt.Errorf("invalid api type: %d, adaptor is nil", apiType), types.ErrorCodeInvalidApiType),
		}
	}

	//// 创建一个用于日志的 info 副本，移除 ApiKey
	//logInfo := info
	//logInfo.ApiKey = ""
	common.SysLog(fmt.Sprintf("testing channel %d with model %s , info %+v ", channel.Id, testModel, info.ToString()))

	priceData, err := helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest)),
		}
	}

	adaptor.Init(info)

	var convertedRequest any
	// 根据 RelayMode 选择正确的转换函数
	switch info.RelayMode {
	case relayconstant.RelayModeEmbeddings:
		// Embedding 请求 - request 已经是正确的类型
		if embeddingReq, ok := request.(*dto.EmbeddingRequest); ok {
			convertedRequest, err = adaptor.ConvertEmbeddingRequest(c, info, *embeddingReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid embedding request type"),
				newAPIError: types.NewError(errors.New("invalid embedding request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeImagesGenerations:
		// 图像生成请求 - request 已经是正确的类型
		if imageReq, ok := request.(*dto.ImageRequest); ok {
			convertedRequest, err = adaptor.ConvertImageRequest(c, info, *imageReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid image request type"),
				newAPIError: types.NewError(errors.New("invalid image request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeRerank:
		// Rerank 请求 - request 已经是正确的类型
		if rerankReq, ok := request.(*dto.RerankRequest); ok {
			convertedRequest, err = adaptor.ConvertRerankRequest(c, info.RelayMode, *rerankReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid rerank request type"),
				newAPIError: types.NewError(errors.New("invalid rerank request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeResponses:
		// Response 请求 - request 已经是正确的类型
		if responseReq, ok := request.(*dto.OpenAIResponsesRequest); ok {
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, info, *responseReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid response request type"),
				newAPIError: types.NewError(errors.New("invalid response request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeResponsesCompact:
		// Response compaction request - convert to OpenAIResponsesRequest before adapting
		switch req := request.(type) {
		case *dto.OpenAIResponsesCompactionRequest:
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
				Model:              req.Model,
				Input:              req.Input,
				Instructions:       req.Instructions,
				PreviousResponseID: req.PreviousResponseID,
			})
		case *dto.OpenAIResponsesRequest:
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, info, *req)
		default:
			return testResult{
				context:     c,
				localErr:    errors.New("invalid response compaction request type"),
				newAPIError: types.NewError(errors.New("invalid response compaction request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	default:
		// Chat/Completion 等其他请求类型
		if generalReq, ok := request.(*dto.GeneralOpenAIRequest); ok {
			convertedRequest, err = adaptor.ConvertOpenAIRequest(c, info, generalReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid general request type"),
				newAPIError: types.NewError(errors.New("invalid general request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	}

	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeConvertRequestFailed),
		}
	}
	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeJsonMarshalFailed),
		}
	}

	//jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings)
	//if err != nil {
	//	return testResult{
	//		context:     c,
	//		localErr:    err,
	//		newAPIError: types.NewError(err, types.ErrorCodeConvertRequestFailed),
	//	}
	//}

	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			if fixedErr, ok := relaycommon.AsParamOverrideReturnError(err); ok {
				return testResult{
					context:     c,
					localErr:    fixedErr,
					newAPIError: relaycommon.NexusTokErrorFromParamOverride(fixedErr),
				}
			}
			return testResult{
				context:     c,
				localErr:    err,
				newAPIError: types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid),
			}
		}
	}

	requestBody := bytes.NewBuffer(jsonData)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return testResult{
			context:             c,
			localErr:            err,
			newAPIError:         types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError),
			countForAutoDisable: true,
		}
	}
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode != http.StatusOK {
			err := service.RelayErrorHandler(c.Request.Context(), httpResp, true)
			common.SysError(fmt.Sprintf(
				"channel test bad response: channel_id=%d name=%s type=%d model=%s endpoint_type=%s status=%d err=%v",
				channel.Id,
				channel.Name,
				channel.Type,
				testModel,
				endpointType,
				httpResp.StatusCode,
				err,
			))
			return testResult{
				context:             c,
				localErr:            err,
				newAPIError:         types.NewOpenAIError(err, types.ErrorCodeBadResponse, httpResp.StatusCode),
				countForAutoDisable: true,
			}
		}
	}
	usageA, respErr := adaptor.DoResponse(c, httpResp, info)
	if respErr != nil {
		return testResult{
			context:             c,
			localErr:            respErr,
			newAPIError:         respErr,
			countForAutoDisable: true,
		}
	}
	usage, usageErr := coerceTestUsage(usageA, isStream, info.GetEstimatePromptTokens())
	if usageErr != nil {
		return testResult{
			context:             c,
			localErr:            usageErr,
			newAPIError:         types.NewOpenAIError(usageErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError),
			countForAutoDisable: true,
		}
	}
	result := w.Result()
	respBody, err := readTestResponseBody(result.Body, isStream)
	if err != nil {
		return testResult{
			context:             c,
			localErr:            err,
			newAPIError:         types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError),
			countForAutoDisable: true,
		}
	}
	if bodyErr := validateTestResponseBody(respBody, isStream); bodyErr != nil {
		return testResult{
			context:             c,
			localErr:            bodyErr,
			newAPIError:         types.NewOpenAIError(bodyErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError),
			countForAutoDisable: true,
		}
	}
	info.SetEstimatePromptTokens(usage.PromptTokens)

	quota, tieredResult := settleTestQuota(info, priceData, usage)
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	consumedTime := float64(milliseconds) / 1000.0
	other := buildTestLogOther(c, info, priceData, usage, tieredResult)
	recovery := applyManualChannelAccountTestSuccess(channel.Id, selectedAccountID)
	attachChannelTestLogMetadata(other, channelTestLogMetadata{
		Status:                 "success",
		Model:                  info.OriginModelName,
		EndpointType:           endpointType,
		Stream:                 info.IsStream,
		SelectedAccountID:      common.GetContextKeyInt(c, constant.ContextKeyChannelAccountId),
		SelectedAccountName:    common.GetContextKeyString(c, constant.ContextKeyChannelAccountName),
		CountedForAutoDisable:  false,
		AutoCheckFailureCount:  recovery.FailureCount,
		AutoCheckResultUpdated: recovery.Updated,
		Recovery:               recovery,
	})
	model.RecordConsumeLog(c, testUserID, model.RecordConsumeLogParams{
		ChannelId:        channel.Id,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ModelName:        info.OriginModelName,
		TokenName:        "模型测试",
		Quota:            quota,
		Content:          "模型测试",
		UseTimeSeconds:   int(consumedTime),
		IsStream:         info.IsStream,
		Group:            info.UsingGroup,
		Other:            other,
	})
	common.SysLog(fmt.Sprintf("testing channel #%d, response: \n%s", channel.Id, string(respBody)))
	return testResult{
		context:     c,
		localErr:    nil,
		newAPIError: nil,
		recovery:    recovery,
	}
}

func attachTestBillingRequestInput(info *relaycommon.RelayInfo, request dto.Request) error {
	if info == nil {
		return nil
	}

	input, err := helper.BuildBillingExprRequestInputFromRequest(request, info.RequestHeaders)
	if err != nil {
		return err
	}
	info.BillingRequestInput = &input
	return nil
}

func settleTestQuota(info *relaycommon.RelayInfo, priceData types.PriceData, usage *dto.Usage) (int, *billingexpr.TieredResult) {
	if usage != nil && info != nil && info.TieredBillingSnapshot != nil {
		isClaudeUsageSemantic := usage.UsageSemantic == "anthropic" || info.GetFinalRequestRelayFormat() == types.RelayFormatClaude
		usedVars := billingexpr.UsedVars(info.TieredBillingSnapshot.ExprString)
		if ok, quota, result := service.TryTieredSettle(info, service.BuildTieredTokenParams(usage, isClaudeUsageSemantic, usedVars)); ok {
			return quota, result
		}
	}

	quota := 0
	if !priceData.UsePrice {
		completionQuota, clamp := common.QuotaRoundChecked(float64(usage.CompletionTokens) * priceData.CompletionRatio)
		if info != nil {
			info.NoteQuotaClamp(clamp)
		}
		quota, clamp = common.QuotaRoundChecked(float64(usage.PromptTokens + completionQuota))
		if info != nil {
			info.NoteQuotaClamp(clamp)
		}
		quota, clamp = common.QuotaRoundChecked(float64(quota) * priceData.ModelRatio)
		if info != nil {
			info.NoteQuotaClamp(clamp)
		}
		if priceData.ModelRatio != 0 && quota <= 0 {
			quota = 1
		}
		return quota, nil
	}

	quota, clamp := common.QuotaRoundChecked(priceData.ModelPrice * common.QuotaPerUnit)
	if info != nil {
		info.NoteQuotaClamp(clamp)
	}
	return quota, nil
}

func buildTestLogOther(c *gin.Context, info *relaycommon.RelayInfo, priceData types.PriceData, usage *dto.Usage, tieredResult *billingexpr.TieredResult) map[string]interface{} {
	other := service.GenerateTextOtherInfo(c, info, priceData.ModelRatio, priceData.GroupRatioInfo.GroupRatio, priceData.CompletionRatio,
		usage.PromptTokensDetails.CachedTokens, priceData.CacheRatio, priceData.ModelPrice, priceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		service.InjectTieredBillingInfo(other, info, tieredResult)
	}
	service.AttachQuotaSaturation(c, info, other)
	return other
}

type channelTestLogMetadata struct {
	Status                 string
	Model                  string
	EndpointType           string
	Stream                 bool
	SelectedAccountID      int
	SelectedAccountName    string
	ErrorCode              string
	Error                  string
	CountedForAutoDisable  bool
	AutoCheckFailureCount  int
	AutoCheckDisabled      bool
	AutoCheckResultUpdated bool
	Recovery               channelTestRecovery
}

// attachChannelTestLogMetadata 将模型测试结果写入消费日志的结构化字段。
//
// 该字段只保存测试状态、模型、选中的同步密钥展示信息和脱敏错误摘要，不保存明文
// key、token、Cookie 或可恢复登录态。前端用量日志依赖该字段区分真实调用和管理员
// 手动模型测试，失败时也能直接展示错误摘要。
func attachChannelTestLogMetadata(other map[string]interface{}, metadata channelTestLogMetadata) {
	if other == nil {
		return
	}
	endpointType := strings.TrimSpace(metadata.EndpointType)
	if endpointType == "" {
		endpointType = "auto"
	}
	payload := map[string]interface{}{
		"status":                    strings.TrimSpace(metadata.Status),
		"model":                     strings.TrimSpace(metadata.Model),
		"endpoint_type":             endpointType,
		"stream":                    metadata.Stream,
		"selected_account_id":       metadata.SelectedAccountID,
		"selected_account_name":     strings.TrimSpace(metadata.SelectedAccountName),
		"counted_for_auto_disable":  metadata.CountedForAutoDisable,
		"failure_count":             metadata.AutoCheckFailureCount,
		"auto_disabled":             metadata.AutoCheckDisabled,
		"auto_check_result_updated": metadata.AutoCheckResultUpdated,
	}
	if metadata.ErrorCode != "" {
		payload["error_code"] = metadata.ErrorCode
	}
	if metadata.Error != "" {
		payload["error"] = metadata.Error
	}
	if metadata.Recovery.SelectedAccountID > 0 {
		payload["recovery"] = buildChannelTestRecoveryResponse(metadata.Recovery)
	}
	other["channel_test"] = payload
}

func buildChannelTestRecoveryResponse(recovery channelTestRecovery) gin.H {
	return gin.H{
		"selected_account_id":      recovery.SelectedAccountID,
		"account_recovered":        recovery.AccountRecovered,
		"channel_recovered":        recovery.ChannelRecovered,
		"account_status_before":    recovery.AccountStatusBefore,
		"account_status_after":     recovery.AccountStatusAfter,
		"channel_status_before":    recovery.ChannelStatusBefore,
		"channel_status_after":     recovery.ChannelStatusAfter,
		"auto_check_updated":       recovery.Updated,
		"auto_check_failure_count": recovery.FailureCount,
	}
}

func attachChannelTestRecoveryResponse(resp gin.H, recovery channelTestRecovery) {
	if recovery.SelectedAccountID <= 0 {
		return
	}
	recoveryResp := buildChannelTestRecoveryResponse(recovery)
	for key, value := range recoveryResp {
		resp[key] = value
	}
}

func channelTestErrorText(result testResult) string {
	if result.newAPIError != nil {
		return result.newAPIError.Error()
	}
	if result.localErr != nil {
		return result.localErr.Error()
	}
	return "渠道模型测试失败"
}

func channelTestErrorCode(result testResult) string {
	if result.newAPIError == nil {
		return ""
	}
	return string(result.newAPIError.GetErrorCode())
}

func channelTestSelectedAccountForLog(ctx *gin.Context, channelID int, selectedAccountID int) (int, string, *model.ChannelAccount) {
	accountID := 0
	accountName := ""
	if ctx != nil {
		accountID = common.GetContextKeyInt(ctx, constant.ContextKeyChannelAccountId)
		accountName = common.GetContextKeyString(ctx, constant.ContextKeyChannelAccountName)
	}
	if accountID <= 0 {
		accountID = selectedAccountID
	}
	if accountID <= 0 {
		return 0, "", nil
	}
	account, err := model.GetChannelAccountById(channelID, accountID)
	if err != nil {
		return accountID, accountName, nil
	}
	if accountName == "" {
		accountName = account.Name
	}
	return accountID, accountName, account
}

func sanitizeChannelTestLogError(errText string, account *model.ChannelAccount) string {
	errText = strings.TrimSpace(errText)
	if errText == "" {
		errText = "渠道模型测试失败"
	}
	errText = common.MaskSensitiveInfo(errText)
	if account != nil {
		if key := strings.TrimSpace(account.Key); key != "" {
			errText = strings.ReplaceAll(errText, key, "[redacted-key]")
		}
	}
	return errText
}

func applyManualChannelAccountTestSuccess(channelID int, selectedAccountID int) channelTestRecovery {
	recovery := channelTestRecovery{SelectedAccountID: selectedAccountID}
	if channelID <= 0 || selectedAccountID <= 0 {
		return recovery
	}
	account, err := model.GetChannelAccountById(channelID, selectedAccountID)
	if err != nil || account == nil || !upstreamaccount.HasAccountSyncMetadata(account.OtherSettings) {
		return recovery
	}
	recovery.AccountStatusBefore = account.Status
	recovery.AccountStatusAfter = account.Status
	accountHadRuntimeBlock := account.Status != common.ChannelStatusEnabled ||
		account.IsCoolingDown(common.GetTimestamp()) ||
		strings.TrimSpace(account.DisabledReason) != "" ||
		strings.TrimSpace(account.LastError) != ""
	recovery.ChannelStatusBefore = common.ChannelStatusEnabled
	recovery.ChannelStatusAfter = common.ChannelStatusEnabled
	settings := upstreamaccount.ApplyAccountAutoCheckSuccess(account.OtherSettings)
	updates := map[string]any{
		"settings":            settings,
		"status":              common.ChannelStatusEnabled,
		"disabled_reason":     "",
		"last_error":          "",
		"rate_limited_until":  0,
		"overload_until":      0,
		"temp_disabled_until": 0,
	}
	if err := model.DB.Model(&model.ChannelAccount{}).Where("channel_id = ? AND id = ?", channelID, selectedAccountID).Updates(updates).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to mark channel account model test success: channel_id=%d account_id=%d error=%v", channelID, selectedAccountID, err))
		recovery.FailureCount = upstreamaccount.ReadAccountAutoCheckMetadata(account.OtherSettings).FailureCount
		return recovery
	}
	account.OtherSettings = settings
	account.Status = common.ChannelStatusEnabled
	recovery.FailureCount = upstreamaccount.ReadAccountAutoCheckMetadata(settings).FailureCount
	recovery.Updated = true
	recovery.AccountStatusAfter = common.ChannelStatusEnabled
	recovery.AccountRecovered = accountHadRuntimeBlock

	if channel, err := model.GetChannelById(channelID, true); err == nil && channel != nil {
		recovery.ChannelStatusBefore = channel.Status
		recovery.ChannelStatusAfter = channel.Status
		if channel.Status != common.ChannelStatusEnabled {
			service.EnableChannel(channelID, "", channel.Name)
			recovery.ChannelRecovered = true
			recovery.ChannelStatusAfter = common.ChannelStatusEnabled
		}
	}
	if refreshErr := refreshChangedAccountChannels(map[int]struct{}{channelID: {}}); refreshErr != nil {
		common.SysLog(fmt.Sprintf("failed to refresh channel account capabilities after model test success: channel_id=%d account_id=%d error=%v", channelID, selectedAccountID, refreshErr))
	}
	return recovery
}

func applyManualChannelAccountTestFailure(channelID int, selectedAccountID int, errText string, countFailure bool) (failureCount int, counted bool, disabled bool) {
	if channelID <= 0 || selectedAccountID <= 0 || !countFailure {
		return 0, false, false
	}
	account, err := model.GetChannelAccountById(channelID, selectedAccountID)
	if err != nil || account == nil || !upstreamaccount.HasAccountSyncMetadata(account.OtherSettings) {
		return 0, false, false
	}
	if account.Status != common.ChannelStatusEnabled {
		metadata := upstreamaccount.ReadAccountAutoCheckMetadata(account.OtherSettings)
		settings := upstreamaccount.ApplyAccountAutoCheckFailure(account.OtherSettings, metadata.FailureCount, errText, metadata.DisabledByAutoCheck)
		updates := map[string]any{
			"settings":   settings,
			"last_error": errText,
		}
		if err := model.DB.Model(&model.ChannelAccount{}).Where("id = ?", account.Id).Updates(updates).Error; err != nil {
			common.SysLog(fmt.Sprintf("failed to mark disabled channel account model test failure: channel_id=%d account_id=%d error=%v", channelID, selectedAccountID, err))
			return metadata.FailureCount, false, false
		}
		return metadata.FailureCount, false, false
	}
	before := upstreamaccount.ReadAccountAutoCheckMetadata(account.OtherSettings).FailureCount
	setting := operation_setting.GetUpstreamAccountKeyCheckSetting()
	_, disabled, _ = applyUpstreamAccountKeyCheckFailure(setting, account, errors.New(errText))
	after := upstreamaccount.ReadAccountAutoCheckMetadata(account.OtherSettings)
	if after.FailureCount <= before {
		common.SysLog(fmt.Sprintf("failed to mark channel account model test failure: channel_id=%d account_id=%d error=%s", channelID, selectedAccountID, errText))
		return before, false, false
	}
	if disabled {
		if refreshErr := refreshChangedAccountChannels(map[int]struct{}{channelID: {}}); refreshErr != nil {
			common.SysLog(fmt.Sprintf("failed to refresh channel account capabilities after model test disable: channel_id=%d account_id=%d error=%v", channelID, selectedAccountID, refreshErr))
		}
	}
	return after.FailureCount, true, disabled
}

func channelTestLogGroup(ctx *gin.Context, userID int) string {
	if ctx != nil {
		if group := strings.TrimSpace(ctx.GetString("group")); group != "" {
			return group
		}
		if group := common.GetContextKeyString(ctx, constant.ContextKeyUserGroup); group != "" {
			return group
		}
	}
	group, _ := model.GetUserGroup(userID, false)
	return group
}

func buildChannelTestFailureLogOther(ctx *gin.Context) map[string]interface{} {
	other := map[string]interface{}{}
	adminInfo := map[string]interface{}{}
	other["admin_info"] = adminInfo
	if ctx == nil {
		return other
	}
	adminInfo["use_channel"] = ctx.GetStringSlice("use_channel")
	if credentialMode := common.GetContextKeyString(ctx, constant.ContextKeyChannelCredentialMode); credentialMode != "" {
		adminInfo["credential_mode"] = credentialMode
	}
	if common.GetContextKeyBool(ctx, constant.ContextKeyChannelAccountPool) {
		adminInfo["account_pool"] = true
		adminInfo["channel_account_id"] = common.GetContextKeyInt(ctx, constant.ContextKeyChannelAccountId)
		adminInfo["channel_account_name"] = common.GetContextKeyString(ctx, constant.ContextKeyChannelAccountName)
	}
	if ctx.Request != nil && ctx.Request.URL != nil {
		other["request_path"] = ctx.Request.URL.Path
	}
	if conversion, ok := relaycommon.GetEndpointAutoConversion(ctx); ok && conversion != nil {
		other["endpoint_auto_conversion"] = conversion.AuditMap()
	}
	service.AttachUpstreamRatioConversionToOther(ctx, other)
	return other
}

func recordManualChannelTestFailureLog(
	requestCtx *gin.Context,
	channel *model.Channel,
	testUserID int,
	testModel string,
	endpointType string,
	isStream bool,
	selectedAccountID int,
	consumedTime float64,
	result testResult,
) testResult {
	if channel == nil {
		return result
	}
	logCtx := result.context
	if logCtx == nil {
		logCtx = requestCtx
	}
	modelName := strings.TrimSpace(testModel)
	if logCtx != nil {
		if originalModel := strings.TrimSpace(logCtx.GetString("original_model")); originalModel != "" {
			modelName = originalModel
		}
	}
	if modelName == "" {
		models := channel.GetModels()
		if len(models) > 0 {
			modelName = strings.TrimSpace(models[0])
		}
	}
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}

	accountID, accountName, account := channelTestSelectedAccountForLog(logCtx, channel.Id, selectedAccountID)
	errText := sanitizeChannelTestLogError(channelTestErrorText(result), account)
	failureCount, counted, disabled := applyManualChannelAccountTestFailure(channel.Id, selectedAccountID, errText, result.countForAutoDisable)
	result.autoCheckFailureCount = failureCount
	result.autoCheckFailureCounted = counted
	result.autoCheckDisabled = disabled

	other := buildChannelTestFailureLogOther(logCtx)
	attachChannelTestLogMetadata(other, channelTestLogMetadata{
		Status:                 "failed",
		Model:                  modelName,
		EndpointType:           normalizeChannelTestEndpoint(channel, modelName, endpointType),
		Stream:                 shouldUseStreamForChannelTest(channel, isStream),
		SelectedAccountID:      accountID,
		SelectedAccountName:    accountName,
		ErrorCode:              channelTestErrorCode(result),
		Error:                  errText,
		CountedForAutoDisable:  counted,
		AutoCheckFailureCount:  failureCount,
		AutoCheckDisabled:      disabled,
		AutoCheckResultUpdated: counted,
	})
	model.RecordConsumeLog(logCtx, testUserID, model.RecordConsumeLogParams{
		ChannelId:        channel.Id,
		PromptTokens:     0,
		CompletionTokens: 0,
		ModelName:        modelName,
		TokenName:        "模型测试",
		Quota:            0,
		Content:          "模型测试",
		UseTimeSeconds:   int(consumedTime),
		IsStream:         shouldUseStreamForChannelTest(channel, isStream),
		Group:            channelTestLogGroup(logCtx, testUserID),
		Other:            other,
	})
	return result
}

func coerceTestUsage(usageAny any, isStream bool, estimatePromptTokens int) (*dto.Usage, error) {
	switch u := usageAny.(type) {
	case *dto.Usage:
		return u, nil
	case dto.Usage:
		return &u, nil
	case nil:
		if !isStream {
			return nil, errors.New("usage is nil")
		}
		usage := &dto.Usage{
			PromptTokens: estimatePromptTokens,
		}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	default:
		if !isStream {
			return nil, fmt.Errorf("invalid usage type: %T", usageAny)
		}
		usage := &dto.Usage{
			PromptTokens: estimatePromptTokens,
		}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	}
}

func readTestResponseBody(body io.ReadCloser, isStream bool) ([]byte, error) {
	defer func() { _ = body.Close() }()
	const maxStreamLogBytes = 8 << 10
	if isStream {
		return io.ReadAll(io.LimitReader(body, maxStreamLogBytes))
	}
	return io.ReadAll(body)
}

func detectErrorFromTestResponseBody(respBody []byte) error {
	b := bytes.TrimSpace(respBody)
	if len(b) == 0 {
		return nil
	}
	if message := detectErrorMessageFromJSONBytes(b); message != "" {
		return fmt.Errorf("upstream error: %s", message)
	}

	for _, line := range bytes.Split(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		if message := detectErrorMessageFromJSONBytes(payload); message != "" {
			return fmt.Errorf("upstream error: %s", message)
		}
	}

	return nil
}

func validateStreamTestResponseBody(respBody []byte) error {
	b := bytes.TrimSpace(respBody)
	if len(b) == 0 {
		return errors.New("stream response body is empty")
	}

	for _, line := range bytes.Split(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}

		return nil
	}

	return errors.New("stream response body does not contain a valid stream event")
}

func validateTestResponseBody(respBody []byte, isStream bool) error {
	if bodyErr := detectErrorFromTestResponseBody(respBody); bodyErr != nil {
		return bodyErr
	}
	if isStream {
		return validateStreamTestResponseBody(respBody)
	}
	return nil
}

func shouldUseStreamForAutomaticChannelTest(channel *model.Channel) bool {
	return channel != nil && channel.Type == constant.ChannelTypeCodex
}

// formatChannelTestFailureMessage 将上游或账号池底层错误转换成管理员可操作的提示。
//
// 渠道测试经常会经过账号池或上游模型服务。底层错误体可能是多层
// JSON 转义后的 `auth_unavailable`、`Unauthorized` 等信息，直接展示会让管理员难以判断
// 是 NexusTok 登录态问题、渠道 Key 问题，还是账号池授权问题。这里仅改写展示文案，
// 不改变原始日志和错误码，便于排查时仍能从日志看到完整响应。
func formatChannelTestFailureMessage(channel *model.Channel, modelName string, err error) string {
	if err == nil {
		return ""
	}
	rawMessage := strings.TrimSpace(err.Error())
	lowerMessage := strings.ToLower(rawMessage)
	if rawMessage == "" {
		return "渠道测试失败：上游返回空错误"
	}

	isAuthUnavailable := strings.Contains(lowerMessage, "auth_unavailable") ||
		strings.Contains(lowerMessage, "authentication_error") ||
		strings.Contains(lowerMessage, "unauthorized") ||
		strings.Contains(lowerMessage, "bad response status code 401")
	if !isAuthUnavailable {
		return rawMessage
	}

	modelPart := strings.TrimSpace(modelName)
	if modelPart == "" && channel != nil {
		models := channel.GetModels()
		if len(models) > 0 {
			modelPart = strings.TrimSpace(models[0])
		}
	}
	if modelPart == "" {
		modelPart = "当前模型"
	}

	if channel != nil && channel.IsGlobalAccountPoolEnabled() {
		groupHint := ""
		if channel.ChannelInfo.AccountPoolGroupId > 0 {
			groupHint = fmt.Sprintf("，账号池组 ID：%d", channel.ChannelInfo.AccountPoolGroupId)
		}
		return fmt.Sprintf(
			"渠道测试失败：%s 在全局账号池中没有可用授权%s。请在账号池管理器中检查该分组是否有启用且未过期、未被标记为不可用的账号；如果账号本应支持该模型，优先考虑重新登录或刷新授权后再试。",
			modelPart,
			groupHint,
		)
	}

	if channel != nil && channel.IsChannelAccountPoolEnabled() {
		return fmt.Sprintf(
			"渠道测试失败：%s 在渠道账号池中没有可用授权。请检查账号池账号是否启用、未过期、未被标记为不可用；如果账号本应支持该模型，优先考虑重新登录或刷新授权后再试。",
			modelPart,
		)
	}

	return fmt.Sprintf(
		"渠道测试失败：上游认证失败，模型 %s 返回 401 Unauthorized。请检查渠道 API Key、Base URL、模型权限或上游账号状态。",
		modelPart,
	)
}

func detectErrorMessageFromJSONBytes(jsonBytes []byte) string {
	if len(jsonBytes) == 0 {
		return ""
	}
	if jsonBytes[0] != '{' && jsonBytes[0] != '[' {
		return ""
	}
	errVal := gjson.GetBytes(jsonBytes, "error")
	if !errVal.Exists() || errVal.Type == gjson.Null {
		return ""
	}

	message := gjson.GetBytes(jsonBytes, "error.message").String()
	if message == "" {
		message = gjson.GetBytes(jsonBytes, "error.error.message").String()
	}
	if message == "" && errVal.Type == gjson.String {
		message = errVal.String()
	}
	if message == "" {
		message = errVal.Raw
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "upstream returned error payload"
	}
	return message
}

func buildTestRequest(model string, endpointType string, channel *model.Channel, isStream bool) dto.Request {
	testResponsesInput := json.RawMessage(`[{"role":"user","content":"hi"}]`)

	// 根据端点类型构建不同的测试请求
	if endpointType != "" {
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeEmbeddings:
			// 返回 EmbeddingRequest
			return &dto.EmbeddingRequest{
				Model: model,
				Input: []any{"hello world"},
			}
		case constant.EndpointTypeImageGeneration:
			// 返回 ImageRequest
			return &dto.ImageRequest{
				Model:  model,
				Prompt: "a cute cat",
				N:      lo.ToPtr(uint(1)),
				Size:   "1024x1024",
			}
		case constant.EndpointTypeJinaRerank:
			// 返回 RerankRequest
			return &dto.RerankRequest{
				Model:     model,
				Query:     "What is Deep Learning?",
				Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
				TopN:      lo.ToPtr(2),
			}
		case constant.EndpointTypeOpenAIResponse:
			// 返回 OpenAIResponsesRequest
			return &dto.OpenAIResponsesRequest{
				Model:  model,
				Input:  json.RawMessage(`[{"role":"user","content":"hi"}]`),
				Stream: lo.ToPtr(isStream),
			}
		case constant.EndpointTypeOpenAIResponseCompact:
			// 返回 OpenAIResponsesCompactionRequest
			return &dto.OpenAIResponsesCompactionRequest{
				Model: model,
				Input: testResponsesInput,
			}
		case constant.EndpointTypeAnthropic, constant.EndpointTypeGemini, constant.EndpointTypeOpenAI:
			// 返回 GeneralOpenAIRequest
			maxTokens := uint(16)
			if constant.EndpointType(endpointType) == constant.EndpointTypeGemini {
				maxTokens = 3000
			}
			req := &dto.GeneralOpenAIRequest{
				Model:  model,
				Stream: lo.ToPtr(isStream),
				Messages: []dto.Message{
					{
						Role:    "user",
						Content: "hi",
					},
				},
				MaxTokens: lo.ToPtr(maxTokens),
			}
			if isStream {
				req.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
			}
			return req
		}
	}

	// 自动检测逻辑（保持原有行为）
	if strings.Contains(strings.ToLower(model), "rerank") {
		return &dto.RerankRequest{
			Model:     model,
			Query:     "What is Deep Learning?",
			Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
			TopN:      lo.ToPtr(2),
		}
	}

	// 先判断是否为 Embedding 模型
	if strings.Contains(strings.ToLower(model), "embedding") ||
		strings.HasPrefix(model, "m3e") ||
		strings.Contains(model, "bge-") {
		// 返回 EmbeddingRequest
		return &dto.EmbeddingRequest{
			Model: model,
			Input: []any{"hello world"},
		}
	}

	// Responses compaction models (must use /v1/responses/compact)
	if strings.HasSuffix(model, ratio_setting.CompactModelSuffix) {
		return &dto.OpenAIResponsesCompactionRequest{
			Model: model,
			Input: testResponsesInput,
		}
	}

	// Responses-only models (e.g. codex series)
	if strings.Contains(strings.ToLower(model), "codex") {
		return &dto.OpenAIResponsesRequest{
			Model:  model,
			Input:  json.RawMessage(`[{"role":"user","content":"hi"}]`),
			Stream: lo.ToPtr(isStream),
		}
	}

	// Chat/Completion 请求 - 返回 GeneralOpenAIRequest
	testRequest := &dto.GeneralOpenAIRequest{
		Model:  model,
		Stream: lo.ToPtr(isStream),
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
	}
	if isStream {
		testRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}

	if strings.HasPrefix(model, "o") {
		testRequest.MaxCompletionTokens = lo.ToPtr(uint(16))
	} else if strings.Contains(model, "thinking") {
		if !strings.Contains(model, "claude") {
			testRequest.MaxTokens = lo.ToPtr(uint(50))
		}
	} else if strings.Contains(model, "gemini") {
		testRequest.MaxTokens = lo.ToPtr(uint(3000))
	} else {
		testRequest.MaxTokens = lo.ToPtr(uint(16))
	}

	return testRequest
}

func TestChannel(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(channelId)
	if err != nil {
		channel, err = model.GetChannelById(channelId, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	//defer func() {
	//	if channel.ChannelInfo.IsMultiKey {
	//		go func() { _ = channel.SaveChannelInfo() }()
	//	}
	//}()
	testModel := c.Query("model")
	endpointType := c.Query("endpoint_type")
	isStream, _ := strconv.ParseBool(c.Query("stream"))
	selectedAccountID := 0
	if rawAccountID := strings.TrimSpace(c.Query("account_id")); rawAccountID != "" {
		selectedAccountID, err = strconv.Atoi(rawAccountID)
		if err != nil || selectedAccountID <= 0 {
			common.ApiErrorMsg(c, "无效的上游密钥账号 ID")
			return
		}
		if channel.GetCredentialMode() != constant.ChannelCredentialModeAccountPool ||
			!channel.HasUpstreamAccountSyncMetadata() {
			common.ApiErrorMsg(c, "指定上游密钥仅支持上游同步账号池渠道")
			return
		}
	}
	testUserID, err := resolveChannelTestUserID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tik := time.Now()
	result := testChannel(c.Request.Context(), channel, testUserID, testModel, endpointType, isStream, selectedAccountID)
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	consumedTime := float64(milliseconds) / 1000.0
	if result.localErr != nil {
		result = recordManualChannelTestFailureLog(c, channel, testUserID, testModel, endpointType, isStream, selectedAccountID, consumedTime, result)
		resp := gin.H{
			"success": false,
			"message": formatChannelTestFailureMessage(channel, testModel, result.localErr),
			"time":    consumedTime,
		}
		if result.newAPIError != nil {
			resp["error_code"] = result.newAPIError.GetErrorCode()
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	go channel.UpdateResponseTime(milliseconds)
	if result.newAPIError != nil {
		result = recordManualChannelTestFailureLog(c, channel, testUserID, testModel, endpointType, isStream, selectedAccountID, consumedTime, result)
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    formatChannelTestFailureMessage(channel, testModel, result.newAPIError),
			"time":       consumedTime,
			"error_code": result.newAPIError.GetErrorCode(),
		})
		return
	}
	resp := gin.H{
		"success": true,
		"message": "",
		"time":    consumedTime,
	}
	attachChannelTestRecoveryResponse(resp, result.recovery)
	c.JSON(http.StatusOK, resp)
}

// channelTestSummary 记录一次批量渠道测试的结果，作为 SystemTask 历史结果保存。
type channelTestSummary struct {
	Tested    int `json:"tested"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Disabled  int `json:"disabled"`
	Enabled   int `json:"enabled"`
}

// performChannelTests 同步执行批量渠道测试。
//
// report 回调接收“已处理数量/总数量”，用于系统任务进度展示；ctx 取消时会尽快停止，
// 避免 runner 租约丢失后旧节点继续改写渠道状态或任务进度。
func performChannelTests(ctx context.Context, channels []*model.Channel, testUserID int, allowDisable bool, report func(processed, total int)) channelTestSummary {
	summary := channelTestSummary{}
	var disableThreshold = int64(common.ChannelDisableThreshold * 1000)
	if disableThreshold == 0 {
		disableThreshold = 10000000 // 不可能触发的阈值，保持旧行为。
	}

	total := len(channels)
	for index, channel := range channels {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		if report != nil {
			report(index, total)
		}
		if channel.Status == common.ChannelStatusManuallyDisabled {
			continue
		}
		isChannelEnabled := channel.Status == common.ChannelStatusEnabled
		tik := time.Now()
		result := testChannel(ctx, channel, testUserID, "", "", shouldUseStreamForAutomaticChannelTest(channel), 0)
		tok := time.Now()
		milliseconds := tok.Sub(tik).Milliseconds()
		if ctx != nil && ctx.Err() != nil {
			break
		}

		summary.Tested++

		shouldBanChannel := false
		newAPIError := result.newAPIError
		// 请求错误是否禁用渠道仍沿用 service 的统一判定，避免批量测试与真实 Relay 热路径规则分叉。
		if newAPIError != nil {
			shouldBanChannel = service.ShouldDisableChannel(result.newAPIError)
		}

		// 当错误检查通过，才检查响应时间。
		if common.AutomaticDisableChannelEnabled && !shouldBanChannel {
			if milliseconds > disableThreshold {
				err := fmt.Errorf("响应时间 %.2fs 超过阈值 %.2fs", float64(milliseconds)/1000.0, float64(disableThreshold)/1000.0)
				newAPIError = types.NewOpenAIError(err, types.ErrorCodeChannelResponseTimeExceeded, http.StatusRequestTimeout)
				shouldBanChannel = true
			}
		}

		if newAPIError == nil {
			summary.Succeeded++
		} else {
			summary.Failed++
		}

		// 被动恢复模式只尝试恢复自动禁用渠道，不再二次自动禁用渠道。
		if allowDisable && isChannelEnabled && shouldBanChannel && channel.GetAutoBan() {
			processChannelError(result.context, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)
			summary.Disabled++
		}

		if result.localErr == nil && !isChannelEnabled && service.ShouldEnableChannel(newAPIError, channel.Status) {
			service.EnableChannel(channel.Id, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.Name)
			summary.Enabled++
		}

		channel.UpdateResponseTime(milliseconds)
		if common.RequestInterval > 0 {
			if ctx == nil {
				time.Sleep(common.RequestInterval)
			} else {
				select {
				case <-ctx.Done():
					return summary
				case <-time.After(common.RequestInterval):
				}
			}
		}
	}
	if report != nil && (ctx == nil || ctx.Err() == nil) {
		report(total, total)
	}
	return summary
}

func runChannelTestTask(ctx context.Context, mode string, notify bool, report func(processed, total int)) (channelTestSummary, error) {
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return channelTestSummary{}, err
	}
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return channelTestSummary{}, err
	}
	if strings.TrimSpace(mode) == "" {
		mode = operation_setting.GetMonitorSetting().ChannelTestMode
	}
	selected := selectChannelsForAutomaticTest(channels, mode)
	allowDisable := mode != operation_setting.ChannelTestModePassiveRecovery
	summary := performChannelTests(ctx, selected, testUserID, allowDisable, report)
	if notify && (ctx == nil || ctx.Err() == nil) {
		service.NotifyRootUser(dto.NotifyTypeChannelTest, "通道测试完成", "所有通道测试已完成")
	}
	return summary, nil
}

func selectChannelsForAutomaticTest(channels []*model.Channel, mode string) []*model.Channel {
	selected := make([]*model.Channel, 0, len(channels))
	for _, channel := range channels {
		if channel.Status == common.ChannelStatusManuallyDisabled {
			continue
		}
		if mode == operation_setting.ChannelTestModePassiveRecovery && channel.Status != common.ChannelStatusAutoDisabled {
			continue
		}
		selected = append(selected, channel)
	}
	return selected
}

func TestAllChannels(c *gin.Context) {
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeChannelTest, channelTestTaskPayload{
		Mode:   operation_setting.ChannelTestModeScheduledAll,
		Notify: true,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "已有通道测试任务正在运行或等待中，不能启动本次手动任务",
			"data": gin.H{
				"task_id": task.TaskID,
				"status":  task.Status,
				"type":    task.Type,
			},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"task_id": task.TaskID,
			"status":  task.Status,
		},
	})
}
