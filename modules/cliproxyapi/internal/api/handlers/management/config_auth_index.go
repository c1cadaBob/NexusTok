// management - config_auth_index.go
// 配置 API Key 到运行时认证索引的映射。
// 该模块将配置文件中的 API Key 条目与运行时认证管理器中的记录关联起来，
// 为每个配置条目附加对应的 auth-index，便于管理面板展示认证状态。
// 支持 Gemini、Claude、Codex、Vertex 和 OpenAI 兼容等所有提供者类型。
package management

import (
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/watcher/synthesizer"
)

// geminiKeyWithAuthIndex 带认证索引的 Gemini API Key 条目。
type geminiKeyWithAuthIndex struct {
	config.GeminiKey                    // 嵌入原始 Gemini Key 配置
	AuthIndex       string `json:"auth-index,omitempty"` // 对应的运行时认证索引
}

// claudeKeyWithAuthIndex 带认证索引的 Claude API Key 条目。
type claudeKeyWithAuthIndex struct {
	config.ClaudeKey                    // 嵌入原始 Claude Key 配置
	AuthIndex        string `json:"auth-index,omitempty"` // 对应的运行时认证索引
}

// codexKeyWithAuthIndex 带认证索引的 Codex API Key 条目。
type codexKeyWithAuthIndex struct {
	config.CodexKey                    // 嵌入原始 Codex Key 配置
	AuthIndex       string `json:"auth-index,omitempty"` // 对应的运行时认证索引
}

// vertexCompatKeyWithAuthIndex 带认证索引的 Vertex 兼容 API Key 条目。
type vertexCompatKeyWithAuthIndex struct {
	config.VertexCompatKey                    // 嵌入原始 Vertex 兼容 Key 配置
	AuthIndex              string `json:"auth-index,omitempty"` // 对应的运行时认证索引
}

// openAICompatibilityAPIKeyWithAuthIndex 带认证索引的 OpenAI 兼容 API Key 条目。
type openAICompatibilityAPIKeyWithAuthIndex struct {
	config.OpenAICompatibilityAPIKey                    // 嵌入原始 OpenAI 兼容 API Key 配置
	AuthIndex                       string `json:"auth-index,omitempty"` // 对应的运行时认证索引
}

// openAICompatibilityWithAuthIndex 带认证索引的 OpenAI 兼容配置条目。
type openAICompatibilityWithAuthIndex struct {
	Name          string                                   `json:"name"`                      // 提供者名称
	Priority      int                                      `json:"priority,omitempty"`        // 优先级
	Disabled      bool                                     `json:"disabled"`                  // 是否禁用
	Prefix        string                                   `json:"prefix,omitempty"`          // 路径前缀
	BaseURL       string                                   `json:"base-url"`                  // 基础 URL
	APIKeyEntries []openAICompatibilityAPIKeyWithAuthIndex `json:"api-key-entries,omitempty"` // API Key 条目列表
	Models        []config.OpenAICompatibilityModel        `json:"models,omitempty"`          // 模型列表
	Headers       map[string]string                        `json:"headers,omitempty"`         // 自定义头部
	AuthIndex     string                                   `json:"auth-index,omitempty"`      // 对应的运行时认证索引
}

// liveAuthIndexByID 构建认证 ID 到运行时认证索引的映射表。
// 遍历认证管理器中的所有记录，提取 ID 和 Index 的对应关系。
// 用于将配置文件中的 API Key 关联到运行时的认证索引。
func (h *Handler) liveAuthIndexByID() map[string]string {
	out := map[string]string{}
	if h == nil {
		return out
	}
	h.mu.Lock()
	manager := h.authManager
	h.mu.Unlock()
	if manager == nil {
		return out
	}
	// authManager.List() returns clones, so EnsureIndex only affects these copies.
	for _, auth := range manager.List() {
		if auth == nil {
			continue
		}
		id := strings.TrimSpace(auth.ID)
		if id == "" {
			continue
		}
		idx := strings.TrimSpace(auth.Index)
		if idx == "" {
			idx = auth.EnsureIndex()
		}
		if idx == "" {
			continue
		}
		out[id] = idx
	}
	return out
}

// geminiKeysWithAuthIndex 返回带认证索引的 Gemini API Key 列表。
// 使用稳定的 ID 生成器将配置中的 API Key 映射到运行时认证索引。
func (h *Handler) geminiKeysWithAuthIndex() []geminiKeyWithAuthIndex {
	if h == nil {
		return nil
	}
	liveIndexByID := h.liveAuthIndexByID()

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil {
		return nil
	}

	idGen := synthesizer.NewStableIDGenerator()
	out := make([]geminiKeyWithAuthIndex, len(h.cfg.GeminiKey))
	for i := range h.cfg.GeminiKey {
		entry := h.cfg.GeminiKey[i]
		authIndex := ""
		if key := strings.TrimSpace(entry.APIKey); key != "" {
			id, _ := idGen.Next("gemini:apikey", key, entry.BaseURL)
			authIndex = liveIndexByID[id]
		}
		out[i] = geminiKeyWithAuthIndex{
			GeminiKey: entry,
			AuthIndex: authIndex,
		}
	}
	return out
}

// claudeKeysWithAuthIndex 返回带认证索引的 Claude API Key 列表。
func (h *Handler) claudeKeysWithAuthIndex() []claudeKeyWithAuthIndex {
	if h == nil {
		return nil
	}
	liveIndexByID := h.liveAuthIndexByID()

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil {
		return nil
	}

	idGen := synthesizer.NewStableIDGenerator()
	out := make([]claudeKeyWithAuthIndex, len(h.cfg.ClaudeKey))
	for i := range h.cfg.ClaudeKey {
		entry := h.cfg.ClaudeKey[i]
		authIndex := ""
		if key := strings.TrimSpace(entry.APIKey); key != "" {
			id, _ := idGen.Next("claude:apikey", key, entry.BaseURL)
			authIndex = liveIndexByID[id]
		}
		out[i] = claudeKeyWithAuthIndex{
			ClaudeKey: entry,
			AuthIndex: authIndex,
		}
	}
	return out
}

// codexKeysWithAuthIndex 返回带认证索引的 Codex API Key 列表。
func (h *Handler) codexKeysWithAuthIndex() []codexKeyWithAuthIndex {
	if h == nil {
		return nil
	}
	liveIndexByID := h.liveAuthIndexByID()

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil {
		return nil
	}

	idGen := synthesizer.NewStableIDGenerator()
	out := make([]codexKeyWithAuthIndex, len(h.cfg.CodexKey))
	for i := range h.cfg.CodexKey {
		entry := h.cfg.CodexKey[i]
		authIndex := ""
		if key := strings.TrimSpace(entry.APIKey); key != "" {
			id, _ := idGen.Next("codex:apikey", key, entry.BaseURL)
			authIndex = liveIndexByID[id]
		}
		out[i] = codexKeyWithAuthIndex{
			CodexKey:  entry,
			AuthIndex: authIndex,
		}
	}
	return out
}

// vertexCompatKeysWithAuthIndex 返回带认证索引的 Vertex 兼容 API Key 列表。
func (h *Handler) vertexCompatKeysWithAuthIndex() []vertexCompatKeyWithAuthIndex {
	if h == nil {
		return nil
	}
	liveIndexByID := h.liveAuthIndexByID()

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil {
		return nil
	}

	idGen := synthesizer.NewStableIDGenerator()
	out := make([]vertexCompatKeyWithAuthIndex, len(h.cfg.VertexCompatAPIKey))
	for i := range h.cfg.VertexCompatAPIKey {
		entry := h.cfg.VertexCompatAPIKey[i]
		id, _ := idGen.Next("vertex:apikey", entry.APIKey, entry.BaseURL, entry.ProxyURL)
		authIndex := liveIndexByID[id]
		out[i] = vertexCompatKeyWithAuthIndex{
			VertexCompatKey: entry,
			AuthIndex:       authIndex,
		}
	}
	return out
}

// openAICompatibilityWithAuthIndex 返回带认证索引的 OpenAI 兼容配置列表。
// 对于有 API Key 条目的配置，每个条目都会单独映射认证索引。
func (h *Handler) openAICompatibilityWithAuthIndex() []openAICompatibilityWithAuthIndex {
	if h == nil {
		return nil
	}
	liveIndexByID := h.liveAuthIndexByID()

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil {
		return nil
	}

	normalized := normalizedOpenAICompatibilityEntries(h.cfg.OpenAICompatibility)
	out := make([]openAICompatibilityWithAuthIndex, len(normalized))
	idGen := synthesizer.NewStableIDGenerator()
	for i := range normalized {
		entry := normalized[i]
		providerName := strings.ToLower(strings.TrimSpace(entry.Name))
		if providerName == "" {
			providerName = "openai-compatibility"
		}
		idKind := fmt.Sprintf("openai-compatibility:%s", providerName)

		response := openAICompatibilityWithAuthIndex{
			Name:      entry.Name,
			Priority:  entry.Priority,
			Disabled:  entry.Disabled,
			Prefix:    entry.Prefix,
			BaseURL:   entry.BaseURL,
			Models:    entry.Models,
			Headers:   entry.Headers,
			AuthIndex: "",
		}
		if len(entry.APIKeyEntries) == 0 {
			id, _ := idGen.Next(idKind, entry.BaseURL)
			response.AuthIndex = liveIndexByID[id]
		} else {
			response.APIKeyEntries = make([]openAICompatibilityAPIKeyWithAuthIndex, len(entry.APIKeyEntries))
			for j := range entry.APIKeyEntries {
				apiKeyEntry := entry.APIKeyEntries[j]
				id, _ := idGen.Next(idKind, apiKeyEntry.APIKey, entry.BaseURL, apiKeyEntry.ProxyURL)
				response.APIKeyEntries[j] = openAICompatibilityAPIKeyWithAuthIndex{
					OpenAICompatibilityAPIKey: apiKeyEntry,
					AuthIndex:                 liveIndexByID[id],
				}
			}
		}
		out[i] = response
	}
	return out
}
