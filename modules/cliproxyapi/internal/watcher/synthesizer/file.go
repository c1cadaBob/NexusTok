// synthesizer - file.go
// 本文件实现了从 OAuth JSON 文件生成 Auth 条目的合成器。
// 处理基于文件的认证和 Gemini 虚拟认证生成。
package synthesizer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/codex"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/geminicli"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// FileSynthesizer 从 OAuth JSON 文件生成 Auth 条目。
// 处理基于文件的认证和 Gemini 虚拟认证生成。
type FileSynthesizer struct{}

// NewFileSynthesizer creates a new FileSynthesizer instance.
func NewFileSynthesizer() *FileSynthesizer {
	return &FileSynthesizer{}
}

func normalizeAccountGroupsFromMetadata(metadata map[string]any) []string {
	if metadata == nil {
		return nil
	}
	return normalizeAccountGroupValues(
		metadata["account_groups"],
		metadata["accountGroups"],
		metadata["account_group"],
		metadata["accountGroup"],
	)
}

func normalizeAccountGroupValues(values ...any) []string {
	seen := make(map[string]struct{})
	groups := make([]string, 0, len(values))
	add := func(value string) {
		for _, part := range normalizeAccountGroupString(value) {
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			groups = append(groups, part)
		}
	}

	for _, value := range values {
		switch v := value.(type) {
		case string:
			add(v)
		case []string:
			for _, item := range v {
				add(item)
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					add(s)
				}
			}
		}
	}
	return groups
}

func normalizeAccountGroupString(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var decoded []string
		if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
			return normalizeAccountGroupValues(decoded)
		}
	}
	if strings.Contains(trimmed, "\n") {
		parts := strings.Split(trimmed, "\n")
		groups := make([]string, 0, len(parts))
		for _, part := range parts {
			if group := strings.TrimSpace(part); group != "" {
				groups = append(groups, group)
			}
		}
		return groups
	}
	return []string{strings.Join(strings.Fields(trimmed), " ")}
}

// Synthesize generates Auth entries from auth files in the auth directory.
func (s *FileSynthesizer) Synthesize(ctx *SynthesisContext) ([]*coreauth.Auth, error) {
	out := make([]*coreauth.Auth, 0, 16)
	if ctx == nil || ctx.AuthDir == "" {
		return out, nil
	}

	entries, err := os.ReadDir(ctx.AuthDir)
	if err != nil {
		// Not an error if directory doesn't exist
		return out, nil
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}
		full := filepath.Join(ctx.AuthDir, name)
		data, errRead := os.ReadFile(full)
		if errRead != nil || len(data) == 0 {
			continue
		}
		auths := synthesizeFileAuths(ctx, full, data)
		if len(auths) == 0 {
			continue
		}
		out = append(out, auths...)
	}
	return out, nil
}

// SynthesizeAuthFile generates Auth entries for one auth JSON file payload.
// It shares exactly the same mapping behavior as FileSynthesizer.Synthesize.
func SynthesizeAuthFile(ctx *SynthesisContext, fullPath string, data []byte) []*coreauth.Auth {
	return synthesizeFileAuths(ctx, fullPath, data)
}

func synthesizeFileAuths(ctx *SynthesisContext, fullPath string, data []byte) []*coreauth.Auth {
	if ctx == nil || len(data) == 0 {
		return nil
	}
	now := ctx.Now
	cfg := ctx.Config
	var metadata map[string]any
	if errUnmarshal := json.Unmarshal(data, &metadata); errUnmarshal != nil {
		return nil
	}
	t, _ := metadata["type"].(string)
	if t == "" {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(t), "codex") {
		metadata = codex.NormalizeMetadata(metadata)
	}
	provider := strings.ToLower(t)
	if provider == "gemini" {
		provider = "gemini-cli"
	}
	label := provider
	if email, _ := metadata["email"].(string); email != "" {
		label = email
	}
	if provider == "codex" {
		if email := codex.ExtractEmail(metadata); email != "" {
			label = email
		}
	}
	// Use relative path under authDir as ID to stay consistent with the file-based token store.
	id := fullPath
	if strings.TrimSpace(ctx.AuthDir) != "" {
		if rel, errRel := filepath.Rel(ctx.AuthDir, fullPath); errRel == nil && rel != "" {
			id = rel
		}
	}
	if runtime.GOOS == "windows" {
		id = strings.ToLower(id)
	}

	proxyURL := ""
	if p, ok := metadata["proxy_url"].(string); ok {
		proxyURL = p
	}

	prefix := ""
	if rawPrefix, ok := metadata["prefix"].(string); ok {
		trimmed := strings.TrimSpace(rawPrefix)
		trimmed = strings.Trim(trimmed, "/")
		if trimmed != "" && !strings.Contains(trimmed, "/") {
			prefix = trimmed
		}
	}

	disabled, _ := metadata["disabled"].(bool)
	status := coreauth.StatusActive
	if disabled {
		status = coreauth.StatusDisabled
	}
	statusMessage := ""
	if provider == "codex" && codex.MissingRefreshTokenForOAuth(metadata) {
		// 旧的 ChatGPT session 导出文件可能只有 access_token/session_token，没有 refresh_token。
		// 这类 OAuth Bearer 凭据无法自动刷新，重启扫描时必须从调度器中移除，同时在管理页保留
		// 明确原因，避免账号看起来 active 但第一次请求就 Unauthorized。
		disabled = true
		status = coreauth.StatusDisabled
		statusMessage = codex.MissingRefreshTokenMessage
	}

	// Read per-account excluded models from the OAuth JSON file.
	perAccountExcluded := extractExcludedModelsFromMetadata(metadata)

	a := &coreauth.Auth{
		ID:            id,
		Provider:      provider,
		Label:         label,
		Prefix:        prefix,
		Status:        status,
		StatusMessage: statusMessage,
		Disabled:      disabled,
		Attributes: map[string]string{
			"source": fullPath,
			"path":   fullPath,
		},
		ProxyURL:  proxyURL,
		Metadata:  metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
	// Read priority from auth file.
	if rawPriority, ok := metadata["priority"]; ok {
		switch v := rawPriority.(type) {
		case float64:
			a.Attributes["priority"] = strconv.Itoa(int(v))
		case string:
			priority := strings.TrimSpace(v)
			if _, errAtoi := strconv.Atoi(priority); errAtoi == nil {
				a.Attributes["priority"] = priority
			}
		}
	}
	// Read note from auth file.
	if rawNote, ok := metadata["note"]; ok {
		if note, isStr := rawNote.(string); isStr {
			if trimmed := strings.TrimSpace(note); trimmed != "" {
				a.Attributes["note"] = trimmed
			}
		}
	}
	// Account groups are exposed for management and NexusTok sidecar filtering.
	if groups := normalizeAccountGroupsFromMetadata(metadata); len(groups) > 0 {
		a.Attributes["account_groups"] = strings.Join(groups, "\n")
		a.Attributes["account_group"] = groups[0]
	}
	coreauth.ApplyCustomHeadersFromMetadata(a)
	ApplyAuthExcludedModelsMeta(a, cfg, perAccountExcluded, "oauth")
	// For codex auth files, extract plan_type from the JWT id_token.
	if provider == "codex" {
		if planType := codex.ExtractPlanType(metadata); planType != "" {
			a.Attributes["plan_type"] = planType
		}
	}
	if provider == "gemini-cli" {
		if virtuals := SynthesizeGeminiVirtualAuths(a, metadata, now); len(virtuals) > 0 {
			for _, v := range virtuals {
				ApplyAuthExcludedModelsMeta(v, cfg, perAccountExcluded, "oauth")
			}
			out := make([]*coreauth.Auth, 0, 1+len(virtuals))
			out = append(out, a)
			out = append(out, virtuals...)
			return out
		}
	}
	return []*coreauth.Auth{a}
}

// SynthesizeGeminiVirtualAuths creates virtual Auth entries for multi-project Gemini credentials.
// It disables the primary auth and creates one virtual auth per project.
func SynthesizeGeminiVirtualAuths(primary *coreauth.Auth, metadata map[string]any, now time.Time) []*coreauth.Auth {
	if primary == nil || metadata == nil {
		return nil
	}
	projects := splitGeminiProjectIDs(metadata)
	if len(projects) <= 1 {
		return nil
	}
	email, _ := metadata["email"].(string)
	shared := geminicli.NewSharedCredential(primary.ID, email, metadata, projects)
	primary.Disabled = true
	primary.Status = coreauth.StatusDisabled
	primary.Runtime = shared
	if primary.Attributes == nil {
		primary.Attributes = make(map[string]string)
	}
	primary.Attributes["gemini_virtual_primary"] = "true"
	primary.Attributes["virtual_children"] = strings.Join(projects, ",")
	source := primary.Attributes["source"]
	authPath := primary.Attributes["path"]
	originalProvider := primary.Provider
	if originalProvider == "" {
		originalProvider = "gemini-cli"
	}
	label := primary.Label
	if label == "" {
		label = originalProvider
	}
	virtuals := make([]*coreauth.Auth, 0, len(projects))
	for _, projectID := range projects {
		attrs := map[string]string{
			"runtime_only":           "true",
			"gemini_virtual_parent":  primary.ID,
			"gemini_virtual_project": projectID,
		}
		if source != "" {
			attrs["source"] = source
		}
		if authPath != "" {
			attrs["path"] = authPath
		}
		// Propagate priority from primary auth to virtual auths
		if priorityVal, hasPriority := primary.Attributes["priority"]; hasPriority && priorityVal != "" {
			attrs["priority"] = priorityVal
		}
		// Propagate note from primary auth to virtual auths
		if noteVal, hasNote := primary.Attributes["note"]; hasNote && noteVal != "" {
			attrs["note"] = noteVal
		}
		if groups := normalizeAccountGroupValues(primary.Attributes["account_groups"], primary.Attributes["account_group"]); len(groups) > 0 {
			attrs["account_groups"] = strings.Join(groups, "\n")
			attrs["account_group"] = groups[0]
		}
		for k, v := range primary.Attributes {
			if strings.HasPrefix(k, "header:") && strings.TrimSpace(v) != "" {
				attrs[k] = v
			}
		}
		metadataCopy := map[string]any{
			"email":             email,
			"project_id":        projectID,
			"virtual":           true,
			"virtual_parent_id": primary.ID,
			"type":              metadata["type"],
		}
		if v, ok := metadata["disable_cooling"]; ok {
			metadataCopy["disable_cooling"] = v
		} else if v, ok := metadata["disable-cooling"]; ok {
			metadataCopy["disable_cooling"] = v
		}
		if v, ok := metadata["request_retry"]; ok {
			metadataCopy["request_retry"] = v
		} else if v, ok := metadata["request-retry"]; ok {
			metadataCopy["request_retry"] = v
		}
		proxy := strings.TrimSpace(primary.ProxyURL)
		if proxy != "" {
			metadataCopy["proxy_url"] = proxy
		}
		if groups := normalizeAccountGroupValues(attrs["account_groups"], attrs["account_group"]); len(groups) > 0 {
			metadataCopy["account_groups"] = append([]string(nil), groups...)
			metadataCopy["account_group"] = groups[0]
		}
		virtual := &coreauth.Auth{
			ID:         buildGeminiVirtualID(primary.ID, projectID),
			Provider:   originalProvider,
			Label:      fmt.Sprintf("%s [%s]", label, projectID),
			Status:     coreauth.StatusActive,
			Attributes: attrs,
			Metadata:   metadataCopy,
			ProxyURL:   primary.ProxyURL,
			Prefix:     primary.Prefix,
			CreatedAt:  primary.CreatedAt,
			UpdatedAt:  primary.UpdatedAt,
			Runtime:    geminicli.NewVirtualCredential(projectID, shared),
		}
		virtuals = append(virtuals, virtual)
	}
	return virtuals
}

// splitGeminiProjectIDs extracts and deduplicates project IDs from metadata.
func splitGeminiProjectIDs(metadata map[string]any) []string {
	raw, _ := metadata["project_id"].(string)
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// buildGeminiVirtualID constructs a virtual auth ID from base ID and project ID.
func buildGeminiVirtualID(baseID, projectID string) string {
	project := strings.TrimSpace(projectID)
	if project == "" {
		project = "project"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_")
	return fmt.Sprintf("%s::%s", baseID, replacer.Replace(project))
}

// extractExcludedModelsFromMetadata reads per-account excluded models from the OAuth JSON metadata.
// Supports both "excluded_models" and "excluded-models" keys, and accepts both []string and []interface{}.
func extractExcludedModelsFromMetadata(metadata map[string]any) []string {
	if metadata == nil {
		return nil
	}
	// Try both key formats
	raw, ok := metadata["excluded_models"]
	if !ok {
		raw, ok = metadata["excluded-models"]
	}
	if !ok || raw == nil {
		return nil
	}
	var stringSlice []string
	switch v := raw.(type) {
	case []string:
		stringSlice = v
	case []interface{}:
		stringSlice = make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				stringSlice = append(stringSlice, s)
			}
		}
	default:
		return nil
	}
	result := make([]string, 0, len(stringSlice))
	for _, s := range stringSlice {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
