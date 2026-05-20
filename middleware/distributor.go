package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/i18n"
	"github.com/c1cada/NexusTok/model"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

type ModelRequest struct {
	Model string `json:"model"`
	Group string `json:"group,omitempty"`
}

func Distribute() func(c *gin.Context) {
	return func(c *gin.Context) {
		var channel *model.Channel
		channelId, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId)
		modelRequest, shouldSelectChannel, err := getModelRequest(c)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidRequest, map[string]any{"Error": err.Error()}))
			return
		}
		if ok {
			id, err := strconv.Atoi(channelId.(string))
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidChannelId))
				return
			}
			channel, err = model.GetChannelById(id, true)
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidChannelId))
				return
			}
			if channel.Status != common.ChannelStatusEnabled {
				abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorChannelDisabled))
				return
			}
		} else {
			// Select a channel for the user
			// check token model mapping
			modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
			if modelLimitEnable {
				s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
				if !ok {
					// token model limit is empty, all models are not allowed
					abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorTokenNoModelAccess))
					return
				}
				var tokenModelLimit map[string]bool
				tokenModelLimit, ok = s.(map[string]bool)
				if !ok {
					tokenModelLimit = map[string]bool{}
				}
				matchName := ratio_setting.FormatMatchingModelName(modelRequest.Model) // match gpts & thinking-*
				if _, ok := tokenModelLimit[matchName]; !ok {
					abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorTokenModelForbidden, map[string]any{"Model": modelRequest.Model}))
					return
				}
			}

			if shouldSelectChannel {
				if modelRequest.Model == "" {
					abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorModelNameRequired))
					return
				}
				var selectGroup string
				usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
				// check path is /pg/chat/completions
				if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") {
					playgroundRequest := &dto.PlayGroundRequest{}
					err = common.UnmarshalBodyReusable(c, playgroundRequest)
					if err != nil {
						abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidPlayground, map[string]any{"Error": err.Error()}))
						return
					}
					if playgroundRequest.Group != "" {
						if !service.GroupInUserUsableGroups(usingGroup, playgroundRequest.Group) && playgroundRequest.Group != usingGroup {
							abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorGroupAccessDenied))
							return
						}
						usingGroup = playgroundRequest.Group
						common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
					}
				}

				if preferredChannelID, found := service.GetPreferredChannelByAffinity(c, modelRequest.Model, usingGroup); found {
					preferred, err := model.CacheGetChannel(preferredChannelID)
					if err == nil && preferred != nil {
						if preferred.Status != common.ChannelStatusEnabled {
							if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
								abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorAffinityChannelDisabled))
								return
							}
						} else if usingGroup == "auto" {
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
							channel = preferred
							selectGroup = usingGroup
							service.MarkChannelAffinityUsed(c, usingGroup, preferred.Id)
						}
					}
				}

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
						// 如果错误，但是渠道不为空，说明是数据库一致性问题
						//if channel != nil {
						//	common.SysError(fmt.Sprintf("渠道不存在：%d", channel.Id))
						//	message = "数据库一致性已被破坏，请联系管理员"
						//}
						abortWithOpenAiMessage(c, http.StatusServiceUnavailable, message, types.ErrorCodeModelNotFound)
						return
					}
					if channel == nil {
						abortWithOpenAiMessage(c, http.StatusServiceUnavailable, i18n.T(c, i18n.MsgDistributorNoAvailableChannel, map[string]any{"Group": usingGroup, "Model": modelRequest.Model}), types.ErrorCodeModelNotFound)
						return
					}
				}
			}
		}
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
		if setupErr := SetupContextForSelectedChannel(c, channel, modelRequest.Model); setupErr != nil {
			abortWithOpenAiMessage(c, setupErr.StatusCode, setupErr.Error(), setupErr.GetErrorCode())
			return
		}
		defer service.ReleaseSelectedChannelAccount(c)
		defer service.ReleaseSelectedPoolAccount(c)
		c.Next()
		if channel != nil && c.Writer != nil && c.Writer.Status() < http.StatusBadRequest {
			service.RecordChannelAffinity(c, channel.Id)
		}
	}
}

// getModelFromRequest 从请求中读取模型信息
// 根据 Content-Type 自动处理：
// - application/json
// - application/x-www-form-urlencoded
// - multipart/form-data
func getModelFromRequest(c *gin.Context) (*ModelRequest, error) {
	var modelRequest ModelRequest
	err := common.UnmarshalBodyReusable(c, &modelRequest)
	if err != nil {
		return nil, errors.New(i18n.T(c, i18n.MsgDistributorInvalidRequest, map[string]any{"Error": err.Error()}))
	}
	return &modelRequest, nil
}

func getModelRequest(c *gin.Context) (*ModelRequest, bool, error) {
	var modelRequest ModelRequest
	shouldSelectChannel := true
	var err error
	if strings.Contains(c.Request.URL.Path, "/mj/") {
		relayMode := relayconstant.Path2RelayModeMidjourney(c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeMidjourneyTaskFetch ||
			relayMode == relayconstant.RelayModeMidjourneyTaskFetchByCondition ||
			relayMode == relayconstant.RelayModeMidjourneyNotify ||
			relayMode == relayconstant.RelayModeMidjourneyTaskImageSeed {
			shouldSelectChannel = false
		} else {
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
					// task fetch, task fetch by condition, notify
					shouldSelectChannel = false
				}
			}
			modelRequest.Model = midjourneyModel
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/suno/") {
		relayMode := relayconstant.Path2RelaySuno(c.Request.Method, c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeSunoFetch ||
			relayMode == relayconstant.RelayModeSunoFetchByID {
			shouldSelectChannel = false
		} else {
			modelName := service.CoverTaskActionToModelName(constant.TaskPlatformSuno, c.Param("action"))
			modelRequest.Model = modelName
		}
		c.Set("platform", string(constant.TaskPlatformSuno))
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos/") && strings.HasSuffix(c.Request.URL.Path, "/remix") {
		relayMode := relayconstant.RelayModeVideoSubmit
		c.Set("relay_mode", relayMode)
		shouldSelectChannel = false
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos") {
		//curl https://api.openai.com/v1/videos \
		//  -H "Authorization: Bearer $OPENAI_API_KEY" \
		//  -F "model=sora-2" \
		//  -F "prompt=A calico cat playing a piano on stage"
		//	-F input_reference="@image.jpg"
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
		// Gemini API 路径处理: /v1beta/models/gemini-2.0-flash:generateContent
		relayMode := relayconstant.RelayModeGemini
		modelName := extractModelNameFromGeminiPath(c.Request.URL.Path)
		if modelName != "" {
			modelRequest.Model = modelName
		}
		c.Set("relay_mode", relayMode)
	} else if !strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") && !strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/realtime") {
		//wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01
		modelRequest.Model = c.Query("model")
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/moderations") {
		if modelRequest.Model == "" {
			modelRequest.Model = "text-moderation-stable"
		}
	}
	if strings.HasSuffix(c.Request.URL.Path, "embeddings") {
		if modelRequest.Model == "" {
			modelRequest.Model = c.Param("model")
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/images/generations") {
		modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "dall-e")
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1/images/edits") {
		//modelRequest.Model = common.GetStringIfEmpty(c.PostForm("model"), "gpt-image-1")
		contentType := c.ContentType()
		if slices.Contains([]string{gin.MIMEPOSTForm, gin.MIMEMultipartPOSTForm}, contentType) {
			req, err := getModelFromRequest(c)
			if err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/audio") {
		relayMode := relayconstant.RelayModeAudioSpeech
		if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/speech") {

			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "tts-1")
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/translations") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranslation
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranscription
		}
		c.Set("relay_mode", relayMode)
	}
	if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") {
		// playground chat completions
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
		modelRequest.Group = req.Group
		common.SetContextKey(c, constant.ContextKeyTokenGroup, modelRequest.Group)
	}

	if strings.HasPrefix(c.Request.URL.Path, "/v1/responses/compact") && modelRequest.Model != "" {
		modelRequest.Model = ratio_setting.WithCompactModelSuffix(modelRequest.Model)
	}
	return &modelRequest, shouldSelectChannel, nil
}

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

func applyChannelContext(c *gin.Context, channel *model.Channel, account *model.ChannelAccount) {
	common.SetContextKey(c, constant.ContextKeyChannelSetting, resolveChannelSetting(channel, account))
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, resolveChannelOtherSettings(channel, account))
	paramOverride := resolveChannelParamOverride(channel, account)
	headerOverride := resolveChannelHeaderOverride(channel, account)
	if mergedParam, applied := service.ApplyChannelAffinityOverrideTemplate(c, paramOverride); applied {
		paramOverride = mergedParam
	}
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, paramOverride)
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, headerOverride)
	common.SetContextKey(c, constant.ContextKeyChannelOrganization, resolveChannelOrganization(channel, account))
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, resolveChannelModelMapping(channel, account))
	common.SetContextKey(c, constant.ContextKeyChannelStatusCodeMapping, resolveChannelStatusCodeMapping(channel, account))
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, resolveChannelBaseURL(channel, account))

	// TODO: api_version统一
	channelOther := resolveChannelOther(channel, account)
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

func resolveChannelOrganization(channel *model.Channel, account *model.ChannelAccount) string {
	if account != nil && account.OpenAIOrganization != nil && strings.TrimSpace(*account.OpenAIOrganization) != "" {
		return *account.OpenAIOrganization
	}
	if channel.OpenAIOrganization != nil {
		return *channel.OpenAIOrganization
	}
	return ""
}

func resolveChannelModelMapping(channel *model.Channel, account *model.ChannelAccount) string {
	if account != nil && account.ModelMapping != nil && strings.TrimSpace(*account.ModelMapping) != "" {
		return *account.ModelMapping
	}
	return channel.GetModelMapping()
}

func resolveChannelStatusCodeMapping(channel *model.Channel, account *model.ChannelAccount) string {
	if account != nil && account.StatusCodeMapping != nil && strings.TrimSpace(*account.StatusCodeMapping) != "" {
		return *account.StatusCodeMapping
	}
	return channel.GetStatusCodeMapping()
}

func resolveChannelBaseURL(channel *model.Channel, account *model.ChannelAccount) string {
	if account != nil && account.BaseURL != nil && strings.TrimSpace(*account.BaseURL) != "" {
		return *account.BaseURL
	}
	return channel.GetBaseURL()
}

func resolveChannelOther(channel *model.Channel, account *model.ChannelAccount) string {
	if account != nil && strings.TrimSpace(account.Other) != "" {
		return account.Other
	}
	return channel.Other
}

func resolvePoolChannelSetting(channel *model.Channel, group *model.AccountPoolGroup, account *model.PoolAccount) dto.ChannelSettings {
	setting := channel.GetSetting()
	if group != nil && strings.TrimSpace(group.Settings) != "" {
		if err := common.UnmarshalJsonStr(group.Settings, &setting); err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal account pool group setting: group_id=%d, error=%v", group.Id, err))
		}
	}
	if account == nil || account.Setting == nil || strings.TrimSpace(*account.Setting) == "" {
		return setting
	}
	if err := common.Unmarshal([]byte(*account.Setting), &setting); err != nil {
		common.SysLog(fmt.Sprintf("failed to unmarshal pool account setting: account_id=%d, error=%v", account.Id, err))
	}
	return setting
}

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

func resolvePoolChannelOrganization(channel *model.Channel, account *model.PoolAccount) string {
	if account != nil && account.OpenAIOrganization != nil && strings.TrimSpace(*account.OpenAIOrganization) != "" {
		return *account.OpenAIOrganization
	}
	if channel.OpenAIOrganization != nil {
		return *channel.OpenAIOrganization
	}
	return ""
}

func resolvePoolChannelModelMapping(channel *model.Channel, group *model.AccountPoolGroup, account *model.PoolAccount) string {
	if account != nil && account.ModelMapping != nil && strings.TrimSpace(*account.ModelMapping) != "" {
		return *account.ModelMapping
	}
	if group != nil && group.ModelMapping != nil && strings.TrimSpace(*group.ModelMapping) != "" {
		return *group.ModelMapping
	}
	return channel.GetModelMapping()
}

func resolvePoolChannelStatusCodeMapping(channel *model.Channel, account *model.PoolAccount) string {
	if account != nil && account.StatusCodeMapping != nil && strings.TrimSpace(*account.StatusCodeMapping) != "" {
		return *account.StatusCodeMapping
	}
	return channel.GetStatusCodeMapping()
}

func resolvePoolChannelBaseURL(channel *model.Channel, account *model.PoolAccount) string {
	if account != nil && account.BaseURL != nil && strings.TrimSpace(*account.BaseURL) != "" {
		return *account.BaseURL
	}
	return channel.GetBaseURL()
}

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
