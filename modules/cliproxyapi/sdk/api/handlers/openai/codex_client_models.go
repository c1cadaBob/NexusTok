// openai - codex_client_models.go
// 提供 Codex 客户端模型列表的构建逻辑，将全局模型注册表中的模型信息转换为 Codex 客户端期望的格式。
// 包含模型模板加载、推理级别标准化、可见性覆盖、优先级排序等功能。
package openai

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

// codexClientModelsPayload Codex 客户端模型列表的 JSON 解析载体
type codexClientModelsPayload struct {
	Models []map[string]any `json:"models"`
}

var (
	codexClientModelTemplatesOnce sync.Once                   // 保证模型模板只加载一次
	codexClientModelTemplates     map[string]map[string]any   // 按 slug 索引的模型模板映射
	codexClientDefaultTemplate    map[string]any              // 默认模型模板（gpt-5.5）
	codexClientModelTemplatesErr  error                       // 模板加载错误
)

// codexClientAllowedReasoningLevels 允许的推理级别集合
var codexClientAllowedReasoningLevels = map[string]struct{}{
	"none":   {},
	"low":    {},
	"medium": {},
	"high":   {},
	"xhigh":  {},
}

// codexClientModelsResponse 生成 Codex 客户端兼容的模型列表响应
func (h *OpenAIAPIHandler) codexClientModelsResponse() map[string]any {
	return CodexClientModelsResponse(h.Models())
}

// CodexClientModelsResponse 根据模型列表构建 Codex 客户端格式的响应体
func CodexClientModelsResponse(models []map[string]any) map[string]any {
	return map[string]any{
		"models": buildCodexClientModels(models),
	}
}

// buildCodexClientModels 将全局模型列表构建为 Codex 客户端期望的模型元数据数组，
// 优先使用预加载的模板，无模板时使用默认模板并填充模型信息
func buildCodexClientModels(models []map[string]any) []map[string]any {
	templates, defaultTemplate, err := loadCodexClientModelTemplates()
	if err != nil || defaultTemplate == nil {
		return nil
	}

	result := make([]map[string]any, 0, len(models))
	for _, model := range models {
		id := strings.TrimSpace(stringModelValue(model, "id"))
		if id == "" {
			continue
		}

		if template, ok := templates[id]; ok {
			entry := cloneCodexClientModelMap(template)
			sanitizeCodexClientReasoningMetadata(entry)
			applyCodexClientVisibilityOverride(entry, id)
			result = append(result, entry)
			continue
		}

		entry := cloneCodexClientModelMap(defaultTemplate)
		applyCodexClientModelMetadata(entry, id, model)
		sanitizeCodexClientReasoningMetadata(entry)
		applyCodexClientVisibilityOverride(entry, id)
		result = append(result, entry)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return codexClientModelPriority(result[i]) < codexClientModelPriority(result[j])
	})

	return result
}

// loadCodexClientModelTemplates 从嵌入的 JSON 资源中一次性加载模型模板，返回模板映射和默认模板
func loadCodexClientModelTemplates() (map[string]map[string]any, map[string]any, error) {
	codexClientModelTemplatesOnce.Do(func() {
		var payload codexClientModelsPayload
		codexClientModelTemplatesErr = json.Unmarshal(registry.GetCodexClientModelsJSON(), &payload)
		if codexClientModelTemplatesErr != nil {
			return
		}

		codexClientModelTemplates = make(map[string]map[string]any, len(payload.Models))
		for _, model := range payload.Models {
			slug := strings.TrimSpace(stringModelValue(model, "slug"))
			if slug == "" {
				continue
			}
			codexClientModelTemplates[slug] = cloneCodexClientModelMap(model)
			if slug == "gpt-5.5" {
				codexClientDefaultTemplate = cloneCodexClientModelMap(model)
			}
		}
	})

	return codexClientModelTemplates, codexClientDefaultTemplate, codexClientModelTemplatesErr
}

// applyCodexClientModelMetadata 将模型注册表中的元数据（显示名、描述、上下文窗口等）应用到 Codex 客户端模型条目
func applyCodexClientModelMetadata(entry map[string]any, id string, model map[string]any) {
	info := registry.LookupModelInfo(id)

	displayName := stringModelValue(model, "display_name")
	description := stringModelValue(model, "description")
	contextWindow := intModelValue(model, "context_length")

	if info != nil {
		if info.DisplayName != "" {
			displayName = info.DisplayName
		}
		if info.Description != "" {
			description = info.Description
		}
		if info.ContextLength > 0 {
			contextWindow = info.ContextLength
		}
		if info.Type == registry.OpenAIImageModelType {
			entry["visibility"] = "hide"
		}
		applyCodexClientThinkingMetadata(entry, info.Thinking)
	}

	if displayName == "" {
		displayName = id
	}
	if description == "" {
		description = id
	}

	entry["slug"] = id
	entry["display_name"] = displayName
	entry["description"] = description
	entry["priority"] = 100
	entry["prefer_websockets"] = false
	delete(entry, "apply_patch_tool_type")
	delete(entry, "upgrade")
	delete(entry, "availability_nux")

	if contextWindow > 0 {
		entry["context_window"] = contextWindow
		entry["max_context_window"] = contextWindow
	}

	if baseInstructions := stringModelValue(model, "base_instructions"); baseInstructions != "" {
		entry["base_instructions"] = baseInstructions
	}
	if plans, ok := model["available_in_plans"]; ok {
		entry["available_in_plans"] = cloneCodexClientModelValue(plans)
	}
}

// applyCodexClientVisibilityOverride 对特定模型强制隐藏（如图片生成模型在 Codex 客户端中不可见）
func applyCodexClientVisibilityOverride(entry map[string]any, id string) {
	switch strings.TrimSpace(id) {
	case "grok-imagine-image-quality", "gpt-image-2", "grok-imagine-image", "grok-imagine-video":
		entry["visibility"] = "hide"
	}
}

// applyCodexClientThinkingMetadata 将模型的思维推理支持信息应用到 Codex 客户端模型条目
func applyCodexClientThinkingMetadata(entry map[string]any, thinking *registry.ThinkingSupport) {
	if thinking == nil || len(thinking.Levels) == 0 {
		return
	}

	levels := make([]any, 0, len(thinking.Levels))
	defaultLevel := ""
	firstLevel := ""
	for _, rawLevel := range thinking.Levels {
		level := normalizeCodexClientReasoningLevel(rawLevel)
		if level == "" {
			continue
		}
		if firstLevel == "" {
			firstLevel = level
		}
		if (defaultLevel == "" && level != "none") || level == "medium" {
			defaultLevel = level
		}
		levels = append(levels, map[string]any{
			"effort":      level,
			"description": codexClientReasoningDescription(level),
		})
	}
	if len(levels) == 0 {
		return
	}
	if defaultLevel == "" {
		defaultLevel = firstLevel
	}

	entry["supported_reasoning_levels"] = levels
	entry["default_reasoning_level"] = defaultLevel
}

// sanitizeCodexClientReasoningMetadata 清洗并规范化推理级别元数据，移除无效级别并修正默认级别
func sanitizeCodexClientReasoningMetadata(entry map[string]any) {
	rawLevels, ok := entry["supported_reasoning_levels"].([]any)
	if !ok {
		return
	}

	levels := make([]any, 0, len(rawLevels))
	allowedDefaults := make(map[string]struct{}, len(rawLevels))
	for _, rawLevelEntry := range rawLevels {
		levelEntry, ok := rawLevelEntry.(map[string]any)
		if !ok {
			continue
		}
		level := normalizeCodexClientReasoningLevel(stringModelValue(levelEntry, "effort"))
		if level == "" {
			continue
		}
		clonedEntry := cloneCodexClientModelMap(levelEntry)
		clonedEntry["effort"] = level
		levels = append(levels, clonedEntry)
		allowedDefaults[level] = struct{}{}
	}

	if len(levels) == 0 {
		delete(entry, "supported_reasoning_levels")
		delete(entry, "default_reasoning_level")
		return
	}

	defaultLevel := normalizeCodexClientReasoningLevel(stringModelValue(entry, "default_reasoning_level"))
	if _, ok := allowedDefaults[defaultLevel]; !ok {
		defaultLevel = stringModelValue(levels[0].(map[string]any), "effort")
	}

	entry["supported_reasoning_levels"] = levels
	entry["default_reasoning_level"] = defaultLevel
}

// normalizeCodexClientReasoningLevel 将推理级别字符串标准化为允许的级别值，无效值返回空字符串
func normalizeCodexClientReasoningLevel(rawLevel string) string {
	level := strings.ToLower(strings.TrimSpace(rawLevel))
	if _, ok := codexClientAllowedReasoningLevels[level]; !ok {
		return ""
	}
	return level
}

// codexClientReasoningDescription 返回推理级别的人类可读描述
func codexClientReasoningDescription(level string) string {
	switch level {
	case "none":
		return "No reasoning"
	case "low":
		return "Fast responses with lighter reasoning"
	case "medium":
		return "Balances speed and reasoning depth for everyday tasks"
	case "high":
		return "Greater reasoning depth for complex problems"
	case "xhigh":
		return "Extra high reasoning depth for complex problems"
	default:
		return level
	}
}

// codexClientModelPriority 提取模型的优先级数值，未设置时默认为 100
func codexClientModelPriority(model map[string]any) int {
	if priority, ok := model["priority"].(int); ok {
		return priority
	}
	if priority, ok := model["priority"].(float64); ok {
		return int(priority)
	}
	return 100
}

// stringModelValue 从模型 map 中提取字符串字段值，自动去除首尾空白
func stringModelValue(model map[string]any, key string) string {
	if model == nil {
		return ""
	}
	value, ok := model[key]
	if !ok {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// intModelValue 从模型 map 中提取整数字段值，支持 int、int64 和 float64 类型
func intModelValue(model map[string]any, key string) int {
	if model == nil {
		return 0
	}
	switch value := model[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

// cloneCodexClientModelMap 深拷贝模型 map，包括嵌套的 map 和 slice
func cloneCodexClientModelMap(model map[string]any) map[string]any {
	if model == nil {
		return nil
	}
	cloned := make(map[string]any, len(model))
	for key, value := range model {
		cloned[key] = cloneCodexClientModelValue(value)
	}
	return cloned
}

// cloneCodexClientModelValue 递归深拷贝任意模型值（map、slice 或原始类型）
func cloneCodexClientModelValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneCodexClientModelMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, entry := range typed {
			cloned[i] = cloneCodexClientModelValue(entry)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
