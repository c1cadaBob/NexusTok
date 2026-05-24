// 包 registry - model_definitions.go
// 该文件提供了各 AI 提供商的静态模型定义和查找辅助函数。
// 静态模型元数据从嵌入的 models.json 文件加载，也可从网络刷新。
package registry

import (
	"strings"
)

const (
	// codexBuiltinImageModelID 是 Codex 内置图像模型的 ID。
	codexBuiltinImageModelID = "gpt-image-2"
	// xaiBuiltinImageModelID 是 xAI 内置图像模型的 ID。
	xaiBuiltinImageModelID = "grok-imagine-image"
	// xaiBuiltinImageQualityModelID 是 xAI 内置高质量图像模型的 ID。
	xaiBuiltinImageQualityModelID = "grok-imagine-image-quality"
	// xaiBuiltinVideoModelID 是 xAI 内置视频模型的 ID。
	xaiBuiltinVideoModelID = "grok-imagine-video"
)

// staticModelsJSON 映射 models.json 的顶层结构。
// 每个字段对应一个提供商的模型列表。
type staticModelsJSON struct {
	Claude      []*ModelInfo `json:"claude"`
	Gemini      []*ModelInfo `json:"gemini"`
	Vertex      []*ModelInfo `json:"vertex"`
	GeminiCLI   []*ModelInfo `json:"gemini-cli"`
	AIStudio    []*ModelInfo `json:"aistudio"`
	CodexFree   []*ModelInfo `json:"codex-free"`
	CodexTeam   []*ModelInfo `json:"codex-team"`
	CodexPlus   []*ModelInfo `json:"codex-plus"`
	CodexPro    []*ModelInfo `json:"codex-pro"`
	Kimi        []*ModelInfo `json:"kimi"`
	Antigravity []*ModelInfo `json:"antigravity"`
	XAI         []*ModelInfo `json:"xai"`
}

// GetClaudeModels 返回标准 Claude 模型定义列表（深拷贝）。
func GetClaudeModels() []*ModelInfo {
	return cloneModelInfos(getModels().Claude)
}

// GetGeminiModels 返回标准 Gemini 模型定义列表（深拷贝）。
func GetGeminiModels() []*ModelInfo {
	return cloneModelInfos(getModels().Gemini)
}

// GetGeminiVertexModels 返回 Vertex AI 的 Gemini 模型定义列表（深拷贝）。
func GetGeminiVertexModels() []*ModelInfo {
	return cloneModelInfos(getModels().Vertex)
}

// GetGeminiCLIModels 返回 Gemini CLI 的模型定义列表（深拷贝）。
func GetGeminiCLIModels() []*ModelInfo {
	return cloneModelInfos(getModels().GeminiCLI)
}

// GetAIStudioModels 返回 AI Studio 的模型定义列表（深拷贝）。
func GetAIStudioModels() []*ModelInfo {
	return cloneModelInfos(getModels().AIStudio)
}

// GetCodexFreeModels 返回 Codex 免费计划的模型定义列表（深拷贝，含内置模型）。
func GetCodexFreeModels() []*ModelInfo {
	return WithCodexBuiltins(cloneModelInfos(getModels().CodexFree))
}

// GetCodexTeamModels 返回 Codex 团队计划的模型定义列表（深拷贝，含内置模型）。
func GetCodexTeamModels() []*ModelInfo {
	return WithCodexBuiltins(cloneModelInfos(getModels().CodexTeam))
}

// GetCodexPlusModels 返回 Codex Plus 计划的模型定义列表（深拷贝，含内置模型）。
func GetCodexPlusModels() []*ModelInfo {
	return WithCodexBuiltins(cloneModelInfos(getModels().CodexPlus))
}

// GetCodexProModels 返回 Codex Pro 计划的模型定义列表（深拷贝，含内置模型）。
func GetCodexProModels() []*ModelInfo {
	return WithCodexBuiltins(cloneModelInfos(getModels().CodexPro))
}

// GetKimiModels 返回 Kimi（Moonshot AI）模型定义列表（深拷贝）。
func GetKimiModels() []*ModelInfo {
	return cloneModelInfos(getModels().Kimi)
}

// GetAntigravityModels 返回 Antigravity 模型定义列表（深拷贝）。
func GetAntigravityModels() []*ModelInfo {
	return cloneModelInfos(getModels().Antigravity)
}

// GetXAIModels 返回 xAI Grok 模型定义列表（深拷贝，含内置图像/视频模型）。
func GetXAIModels() []*ModelInfo {
	return WithXAIBuiltins(cloneModelInfos(getModels().XAI))
}

// WithCodexBuiltins 向模型列表注入硬编码的 Codex 内置模型定义。
// 内置模型不依赖远程 models.json 更新。已存在的匹配 ID 会被替换。
func WithCodexBuiltins(models []*ModelInfo) []*ModelInfo {
	return upsertModelInfos(models, codexBuiltinImageModelInfo())
}

// WithXAIBuiltins 向模型列表注入硬编码的 xAI 图像/视频模型定义。
// 内置模型不依赖远程 models.json 更新。
func WithXAIBuiltins(models []*ModelInfo) []*ModelInfo {
	return upsertModelInfos(models, xaiBuiltinImageModelInfo(), xaiBuiltinImageQualityModelInfo(), xaiBuiltinVideoModelInfo())
}

// codexBuiltinImageModelInfo 返回 Codex 内置图像模型 gpt-image-2 的 ModelInfo。
func codexBuiltinImageModelInfo() *ModelInfo {
	return &ModelInfo{
		ID:          codexBuiltinImageModelID,
		Object:      "model",
		Created:     1704067200, // 2024-01-01
		OwnedBy:     "openai",
		Type:        "openai",
		DisplayName: "GPT Image 2",
		Version:     codexBuiltinImageModelID,
	}
}

// xaiBuiltinImageModelInfo 返回 xAI 内置图像模型 grok-imagine-image 的 ModelInfo。
func xaiBuiltinImageModelInfo() *ModelInfo {
	return &ModelInfo{
		ID:          xaiBuiltinImageModelID,
		Object:      "model",
		Created:     1735689600, // 2025-01-01
		OwnedBy:     "xai",
		Type:        "xai",
		DisplayName: "Grok Imagine Image",
		Name:        xaiBuiltinImageModelID,
		Description: "xAI Grok image generation model.",
	}
}

// xaiBuiltinImageQualityModelInfo 返回 xAI 内置高质量图像模型 grok-imagine-image-quality 的 ModelInfo。
func xaiBuiltinImageQualityModelInfo() *ModelInfo {
	return &ModelInfo{
		ID:          xaiBuiltinImageQualityModelID,
		Object:      "model",
		Created:     1735689600, // 2025-01-01
		OwnedBy:     "xai",
		Type:        "xai",
		DisplayName: "Grok Imagine Image Quality",
		Name:        xaiBuiltinImageQualityModelID,
		Description: "xAI Grok higher-fidelity image generation model.",
	}
}

// xaiBuiltinVideoModelInfo 返回 xAI 内置视频模型 grok-imagine-video 的 ModelInfo。
func xaiBuiltinVideoModelInfo() *ModelInfo {
	return &ModelInfo{
		ID:          xaiBuiltinVideoModelID,
		Object:      "model",
		Created:     1735689600, // 2025-01-01
		OwnedBy:     "xai",
		Type:        "xai",
		DisplayName: "Grok Imagine Video",
		Name:        xaiBuiltinVideoModelID,
		Description: "xAI Grok video generation model.",
	}
}

// upsertModelInfos 将额外的模型定义插入到列表中，替换已存在的匹配 ID。
// 返回合并后的新列表，不修改原始切片。
func upsertModelInfos(models []*ModelInfo, extras ...*ModelInfo) []*ModelInfo {
	if len(extras) == 0 {
		return models
	}

	extraIDs := make(map[string]struct{}, len(extras))
	extraList := make([]*ModelInfo, 0, len(extras))
	for _, extra := range extras {
		if extra == nil {
			continue
		}
		id := strings.TrimSpace(extra.ID)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, exists := extraIDs[key]; exists {
			continue
		}
		extraIDs[key] = struct{}{}
		extraList = append(extraList, cloneModelInfo(extra))
	}

	if len(extraList) == 0 {
		return models
	}

	filtered := make([]*ModelInfo, 0, len(models)+len(extraList))
	for _, model := range models {
		if model == nil {
			continue
		}
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		if _, exists := extraIDs[strings.ToLower(id)]; exists {
			continue
		}
		filtered = append(filtered, model)
	}

	filtered = append(filtered, extraList...)
	return filtered
}

// cloneModelInfos 返回切片的浅拷贝，每个元素进行深拷贝。
func cloneModelInfos(models []*ModelInfo) []*ModelInfo {
	if len(models) == 0 {
		return nil
	}
	out := make([]*ModelInfo, len(models))
	for i, m := range models {
		out[i] = cloneModelInfo(m)
	}
	return out
}

// GetStaticModelDefinitionsByChannel 根据渠道/提供商名称返回静态模型定义。
// 未知渠道返回 nil。
//
// 支持的渠道：claude、gemini、vertex、gemini-cli、aistudio、codex、kimi、antigravity、xai
func GetStaticModelDefinitionsByChannel(channel string) []*ModelInfo {
	key := strings.ToLower(strings.TrimSpace(channel))
	switch key {
	case "claude":
		return GetClaudeModels()
	case "gemini":
		return GetGeminiModels()
	case "vertex":
		return GetGeminiVertexModels()
	case "gemini-cli":
		return GetGeminiCLIModels()
	case "aistudio":
		return GetAIStudioModels()
	case "codex":
		return GetCodexProModels()
	case "kimi":
		return GetKimiModels()
	case "antigravity":
		return GetAntigravityModels()
	case "xai", "x-ai", "grok":
		return GetXAIModels()
	default:
		return nil
	}
}

// LookupStaticModelInfo 在所有静态模型定义中按 ID 搜索模型。
// 未找到时返回 nil。
func LookupStaticModelInfo(modelID string) *ModelInfo {
	if modelID == "" {
		return nil
	}

	data := getModels()
	allModels := [][]*ModelInfo{
		data.Claude,
		data.Gemini,
		data.Vertex,
		data.GeminiCLI,
		data.AIStudio,
		data.CodexPro,
		data.Kimi,
		data.Antigravity,
		data.XAI,
	}
	for _, models := range allModels {
		for _, m := range models {
			if m != nil && m.ID == modelID {
				return cloneModelInfo(m)
			}
		}
	}

	return nil
}
