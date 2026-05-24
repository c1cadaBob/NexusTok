// Package helper - model_mapped.go
// 本文件提供了模型名称映射（Model Mapping）的辅助函数。
// 模型映射允许管理员通过 JSON 配置将客户端请求的模型名称重定向到上游实际模型名称，
// 支持链式重定向（A -> B -> C）和循环检测。
// 该机制常用于将通用模型别名映射到具体的上游模型，或在不同模型版本之间进行切换。
// 同时支持 Responses API Compact 模式的模型名称后缀处理。
package helper

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/common"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// ModelMappedHelper 执行模型名称映射逻辑。
// 根据渠道配置的 model_mapping（JSON 格式的模型名映射表），将客户端请求的原始模型名称
// 重定向到上游实际使用的模型名称。支持以下特性：
//   - 链式重定向：支持 A -> B -> C 的多级映射，最终使用链尾的模型名称
//   - 循环检测：通过 visitedModels 集合检测映射链中的循环，避免无限循环
//   - Responses Compact 模式：处理带有 CompactModelSuffix 后缀的模型名称
//   - 自引用检测：当模型映射到自身时，视为未映射
//
// 参数：
//   - c: Gin 上下文，用于获取 model_mapping 配置
//   - info: 中继信息，包含原始模型名称，映射结果将写入 UpstreamModelName 和 IsModelMapped
//   - request: 请求对象，映射完成后会更新其中的模型名称
//
// 返回值：
//   - error: 映射过程中的错误（如 JSON 解析失败、映射链存在循环等）
func ModelMappedHelper(c *gin.Context, info *common.RelayInfo, request dto.Request) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &common.ChannelMeta{}
	}

	isResponsesCompact := info.RelayMode == relayconstant.RelayModeResponsesCompact
	originModelName := info.OriginModelName
	mappingModelName := originModelName
	if isResponsesCompact && strings.HasSuffix(originModelName, ratio_setting.CompactModelSuffix) {
		mappingModelName = strings.TrimSuffix(originModelName, ratio_setting.CompactModelSuffix)
	}

	// map model name
	modelMapping := c.GetString("model_mapping")
	if modelMapping != "" && modelMapping != "{}" {
		modelMap := make(map[string]string)
		err := json.Unmarshal([]byte(modelMapping), &modelMap)
		if err != nil {
			return fmt.Errorf("unmarshal_model_mapping_failed")
		}

		// 支持链式模型重定向，最终使用链尾的模型
		currentModel := mappingModelName
		visitedModels := map[string]bool{
			currentModel: true,
		}
		for {
			if mappedModel, exists := modelMap[currentModel]; exists && mappedModel != "" {
				// 模型重定向循环检测，避免无限循环
				if visitedModels[mappedModel] {
					if mappedModel == currentModel {
						if currentModel == info.OriginModelName {
							info.IsModelMapped = false
							return nil
						} else {
							info.IsModelMapped = true
							break
						}
					}
					return errors.New("model_mapping_contains_cycle")
				}
				visitedModels[mappedModel] = true
				currentModel = mappedModel
				info.IsModelMapped = true
			} else {
				break
			}
		}
		if info.IsModelMapped {
			info.UpstreamModelName = currentModel
		}
	}

	if isResponsesCompact {
		finalUpstreamModelName := mappingModelName
		if info.IsModelMapped && info.UpstreamModelName != "" {
			finalUpstreamModelName = info.UpstreamModelName
		}
		info.UpstreamModelName = finalUpstreamModelName
		info.OriginModelName = ratio_setting.WithCompactModelSuffix(finalUpstreamModelName)
	}
	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}
