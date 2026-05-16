package service

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"
)

// ApplyRequestRuleOverrides 应用匹配到的请求规则覆写
// 在渠道级别的 ParamOverride 之后调用，作为全局规则的补充
// 返回覆写后的 jsonData 和匹配到的规则列表（用于后续请求记录）
func ApplyRequestRuleOverrides(jsonData []byte, info *relaycommon.RelayInfo) ([]byte, []*model.RequestRule, error) {
	rules := model.GetEnabledRequestRules()
	if len(rules) == 0 {
		return jsonData, nil, nil
	}

	// 获取当前请求的 RelayFormat 和模型名
	relayFormat := string(info.RelayFormat)
	modelName := info.UpstreamModelName
	if modelName == "" && info.ChannelMeta != nil {
		modelName = info.ChannelMeta.UpstreamModelName
	}

	var matchedRules []*model.RequestRule
	result := jsonData

	for _, rule := range rules {
		// 检查 RelayFormat 匹配（空=匹配全部）
		if rule.RelayFormat != "" && !strings.EqualFold(rule.RelayFormat, relayFormat) {
			continue
		}
		// 检查模型名匹配
		if !rule.MatchModel(modelName) {
			continue
		}

		matchedRules = append(matchedRules, rule)

		// 应用参数覆写
		paramOverride := rule.GetParamOverride()
		if len(paramOverride) > 0 {
			overrideCtx := relaycommon.BuildParamOverrideContext(info)
			var err error
			result, err = relaycommon.ApplyParamOverride(result, paramOverride, overrideCtx)
			if err != nil {
				return nil, matchedRules, fmt.Errorf("请求规则 '%s' (id=%d) 覆写失败: %w", rule.Name, rule.Id, err)
			}
			// 同步运行时请求头覆写
			relaycommon.SyncRuntimeHeaderOverrideFromContext(info, overrideCtx)
		}
	}

	return result, matchedRules, nil
}

// RecordRequestLogAsync 异步记录匹配到规则的请求内容
func RecordRequestLogAsync(info *relaycommon.RelayInfo, matchedRules []*model.RequestRule, requestBody []byte, relayFormat types.RelayFormat) {
	if len(matchedRules) == 0 {
		return
	}

	for _, rule := range matchedRules {
		if !rule.LogRequest && !rule.LogResponse {
			continue
		}

		log := &model.RequestLog{
			RequestRuleId: rule.Id,
			RequestId:     info.RequestId,
			UserId:        info.UserId,
			TokenId:       info.TokenId,
			ChannelId:     0,
			ModelName:     info.UpstreamModelName,
			RelayFormat:   string(relayFormat),
			CreatedAt:     common.GetTimestamp(),
		}
		if info.ChannelMeta != nil {
			log.ChannelId = info.ChannelMeta.ChannelId
		}

		// 记录请求体（按规则配置的最大大小截断）
		if rule.LogRequest && len(requestBody) > 0 {
			body := string(requestBody)
			if rule.LogMaxSize > 0 && len(body) > rule.LogMaxSize {
				body = body[:rule.LogMaxSize] + "...[truncated]"
			}
			log.RequestBody = body
		}

		// 异步写入日志库
		go model.InsertRequestLog(log)
	}
}
