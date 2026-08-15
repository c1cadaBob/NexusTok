// Package upstreammodel 提供上游模型列表读取能力。
//
// 该包只负责用给定的渠道配置发起一次模型列表请求，不保存任何凭据，也不参与
// 渠道账号池选号。同步密钥模型刷新、配置页“获取模型”和后台连接测试都需要确保
// 请求命中指定 key，因此这里集中处理账号级 BaseURL、Header、Setting 等覆盖逻辑。
package upstreammodel

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	relaychannel "github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/gemini"
	"github.com/c1cada/NexusTok/relay/channel/ollama"
	"github.com/c1cada/NexusTok/service"
)

type openAIModel struct {
	ID string `json:"id"`
}

type openAIModelsResponse struct {
	Data []openAIModel `json:"data"`
}

// ChannelWithAccountCredential 构造仅用于一次上游请求的渠道副本。
//
// 账号池账号允许覆盖渠道的连接参数。模型获取和密钥自动测试必须按目标账号执行，
// 不能让 Relay 的账号池随机选择其它 key；这里把账号级凭据合并到临时渠道副本中，
// 调用方随后按普通单 key 渠道请求上游即可。函数不修改传入对象，也不会落库。
func ChannelWithAccountCredential(channel *model.Channel, account *model.ChannelAccount) *model.Channel {
	if channel == nil || account == nil {
		return channel
	}

	cloned := *channel
	cloned.Key = strings.TrimSpace(account.Key)
	cloned.OpenAIOrganization = account.OpenAIOrganization
	cloned.ChannelInfo.IsMultiKey = false
	cloned.ChannelInfo.AccountPoolEnabled = false
	cloned.ChannelInfo.CredentialMode = ""
	if strings.TrimSpace(account.Other) != "" {
		cloned.Other = account.Other
	}
	if otherSettings := strings.TrimSpace(account.GetOtherSettings(channel.OtherSettings)); otherSettings != "" {
		cloned.OtherSettings = otherSettings
	}

	if baseURL := strings.TrimSpace(account.GetBaseURL(channel.GetBaseURL())); baseURL != "" {
		cloned.BaseURL = common.GetPointer(baseURL)
	}

	defaultSetting := ""
	if channel.Setting != nil {
		defaultSetting = *channel.Setting
	}
	if setting := strings.TrimSpace(account.GetSetting(defaultSetting)); setting != "" {
		cloned.Setting = common.GetPointer(setting)
	}

	if headerOverride := account.GetHeaderOverride(channel.HeaderOverride); headerOverride != nil {
		if trimmed := strings.TrimSpace(*headerOverride); trimmed != "" {
			cloned.HeaderOverride = common.GetPointer(trimmed)
		} else {
			cloned.HeaderOverride = nil
		}
	}

	return &cloned
}

// FetchChannelModelIDs 从上游获取渠道可见模型 ID 列表。
//
// 支持 Ollama、Gemini 和 OpenAI 兼容模型接口。调用方若要测试账号池中的某个指定
// ChannelAccount，应先用 ChannelWithAccountCredential 构造临时渠道，避免走账号池选号。
func FetchChannelModelIDs(channel *model.Channel) ([]string, error) {
	if channel == nil {
		return nil, fmt.Errorf("渠道不能为空")
	}
	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	if channel.Type == constant.ChannelTypeOllama {
		key := strings.TrimSpace(strings.Split(channel.Key, "\n")[0])
		models, err := ollama.FetchOllamaModels(baseURL, key)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(models))
		for _, item := range models {
			names = append(names, item.Name)
		}
		return NormalizeModelNames(names), nil
	}

	if channel.Type == constant.ChannelTypeGemini {
		key, _, apiErr := channel.GetNextEnabledKey()
		if apiErr != nil {
			return nil, fmt.Errorf("获取渠道密钥失败: %w", apiErr)
		}
		key = strings.TrimSpace(key)
		models, err := gemini.FetchGeminiModels(baseURL, key, channel.GetSetting().Proxy)
		if err != nil {
			return nil, err
		}
		return NormalizeModelNames(models), nil
	}

	url := fetchModelsURL(channel.Type, baseURL)
	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return nil, fmt.Errorf("获取渠道密钥失败: %w", apiErr)
	}
	key = strings.TrimSpace(key)

	headers, err := buildFetchModelsHeaders(channel, key)
	if err != nil {
		return nil, err
	}

	body, err := getResponseBody(http.MethodGet, url, channel, headers)
	if err != nil {
		return nil, err
	}

	var result openAIModelsResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(result.Data))
	for _, item := range result.Data {
		ids = append(ids, item.ID)
	}
	return NormalizeModelNames(ids), nil
}

func fetchModelsURL(channelType int, baseURL string) string {
	switch channelType {
	case constant.ChannelTypeAli:
		return fmt.Sprintf("%s/compatible-mode/v1/models", baseURL)
	case constant.ChannelTypeZhipu_v4:
		if plan, ok := constant.ChannelSpecialBases[baseURL]; ok && plan.OpenAIBaseURL != "" {
			return fmt.Sprintf("%s/models", plan.OpenAIBaseURL)
		}
		return fmt.Sprintf("%s/api/paas/v4/models", baseURL)
	case constant.ChannelTypeVolcEngine:
		if plan, ok := constant.ChannelSpecialBases[baseURL]; ok && plan.OpenAIBaseURL != "" {
			return fmt.Sprintf("%s/v1/models", plan.OpenAIBaseURL)
		}
		return fmt.Sprintf("%s/v1/models", baseURL)
	case constant.ChannelTypeMoonshot:
		if plan, ok := constant.ChannelSpecialBases[baseURL]; ok && plan.OpenAIBaseURL != "" {
			return fmt.Sprintf("%s/models", plan.OpenAIBaseURL)
		}
		return fmt.Sprintf("%s/v1/models", baseURL)
	default:
		return fmt.Sprintf("%s/v1/models", baseURL)
	}
}

func buildFetchModelsHeaders(channel *model.Channel, key string) (http.Header, error) {
	var headers http.Header
	switch channel.Type {
	case constant.ChannelTypeAnthropic:
		headers = http.Header{}
		headers.Add("x-api-key", key)
		headers.Add("anthropic-version", "2023-06-01")
	default:
		headers = http.Header{}
		headers.Add("Authorization", fmt.Sprintf("Bearer %s", key))
	}

	headerOverride := channel.GetHeaderOverride()
	for k, v := range headerOverride {
		if relaychannel.IsHeaderPassthroughRuleKey(k) {
			continue
		}
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid header override for key %s", k)
		}
		if strings.Contains(str, "{api_key}") {
			str = strings.ReplaceAll(str, "{api_key}", key)
		}
		headers.Set(k, str)
	}

	return headers, nil
}

func getResponseBody(method string, url string, channel *model.Channel, headers http.Header) ([]byte, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	for k := range headers {
		req.Header.Add(k, headers.Get(k))
	}
	client, err := service.NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		if len(body) > 0 {
			return nil, fmt.Errorf("status code: %d, body: %s", res.StatusCode, common.MaskSensitiveInfo(string(body)))
		}
		return nil, fmt.Errorf("status code: %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// NormalizeModelNames 规范化模型名称列表。
//
// 保留上游返回顺序，同时去除空白项和重复项。模型名大小写按上游原样保留，避免
// 把大小写敏感的自定义模型 ID 合并成错误能力。
func NormalizeModelNames(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		result = append(result, modelName)
	}
	return result
}
