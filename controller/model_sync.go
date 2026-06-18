// Package controller - model_sync.go
// 该文件实现了上游模型同步功能的 API 控制器
//
// 从远程元数据仓库同步模型和供应商信息到本地数据库
// 支持：
// - 自动创建缺失的模型和供应商
// - 选择性覆盖更新本地已有模型的字段
// - ETag 缓存减少网络请求
// - 多语言支持（en、zh-CN、zh-TW、ja）
//
// 主要 API：
// - SyncUpstreamModels：同步上游模型与供应商
// - SyncUpstreamPreview：预览上游与本地的差异
package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 上游地址常量
const (
	upstreamModelsURL  = "https://basellm.github.io/llm-metadata/api/newapi/models.json"  // 模型数据 URL
	upstreamVendorsURL = "https://basellm.github.io/llm-metadata/api/newapi/vendors.json" // 供应商数据 URL
)

// 同步来源常量。
//
// official 保持现有官方元数据仓库行为；models.dev 用于每日自动同步，
// 也允许后续通过 API 显式指定，方便排查或临时手动触发同一套转换逻辑。
const (
	syncSourceOfficial  = "official"
	syncSourceModelsDev = "models.dev"

	modelsDevDefaultBaseURL = "https://models.dev"
	modelsDevCatalogPath    = "/catalog.json"
)

// normalizeLocale 标准化语言代码
//
// 支持的语言：en、zh-CN、zh-TW、ja
func normalizeLocale(locale string) (string, bool) {
	l := strings.ToLower(strings.TrimSpace(locale))
	switch l {
	case "en", "zh-CN", "zh-TW", "ja":
		return l, true
	default:
		return "", false
	}
}

// getUpstreamBase 获取上游基础 URL
//
// 可通过环境变量 SYNC_UPSTREAM_BASE 覆盖
func getUpstreamBase() string {
	return common.GetEnvOrDefaultString("SYNC_UPSTREAM_BASE", "https://basellm.github.io/llm-metadata")
}

// getUpstreamURLs 根据语言获取上游数据 URL
//
// 参数：
//   - locale: 语言代码
//
// 返回值：
//   - modelsURL: 模型数据 URL
//   - vendorsURL: 供应商数据 URL
func getUpstreamURLs(locale string) (modelsURL, vendorsURL string) {
	base := strings.TrimRight(getUpstreamBase(), "/")
	if l, ok := normalizeLocale(locale); ok && l != "" {
		return fmt.Sprintf("%s/api/i18n/%s/newapi/models.json", base, l),
			fmt.Sprintf("%s/api/i18n/%s/newapi/vendors.json", base, l)
	}
	return fmt.Sprintf("%s/api/newapi/models.json", base), fmt.Sprintf("%s/api/newapi/vendors.json", base)
}

// normalizeSyncSource 标准化同步来源。
func normalizeSyncSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", syncSourceOfficial, "repository", "upstream":
		return syncSourceOfficial
	case syncSourceModelsDev, "modelsdev", "models_dev", "models-dev", "models":
		return syncSourceModelsDev
	default:
		return syncSourceOfficial
	}
}

// getModelsDevBase 获取 models.dev 基础 URL。
//
// 环境变量 MODELS_DEV_SYNC_BASE 主要用于测试、镜像源和内网代理。
func getModelsDevBase() string {
	return common.GetEnvOrDefaultString("MODELS_DEV_SYNC_BASE", modelsDevDefaultBaseURL)
}

// getModelsDevCatalogURL 获取 models.dev catalog URL。
func getModelsDevCatalogURL() string {
	return strings.TrimRight(getModelsDevBase(), "/") + modelsDevCatalogPath
}

// upstreamEnvelope 上游 API 响应信封结构体
type upstreamEnvelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    []T    `json:"data"`
}

// upstreamModel 上游模型数据结构体
type upstreamModel struct {
	Description string          `json:"description"`
	Endpoints   json.RawMessage `json:"endpoints"`
	Icon        string          `json:"icon"`
	ModelName   string          `json:"model_name"`
	NameRule    int             `json:"name_rule"`
	Status      int             `json:"status"`
	Tags        string          `json:"tags"`
	VendorName  string          `json:"vendor_name"`
}

// upstreamVendor 上游供应商数据结构体
type upstreamVendor struct {
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Name        string `json:"name"`
	Status      int    `json:"status"`
}

// modelsDevCatalog 是 models.dev /catalog.json 的最小解析结构。
//
// catalog 同时包含 provider-agnostic 的 canonical models 和 provider 目录；
// 本项目真正发起请求时使用的是各 provider 下的模型 ID，因此自动同步以
// Providers[*].Models 为准，避免把仅用于归档/展示的规范模型名误当成可请求名称。
type modelsDevCatalog struct {
	Providers map[string]modelsDevCatalogProvider `json:"providers"`
}

// modelsDevCatalogProvider 表示 models.dev 中的一个模型服务商。
type modelsDevCatalogProvider struct {
	ID     string                           `json:"id"`
	Name   string                           `json:"name"`
	Doc    string                           `json:"doc"`
	Models map[string]modelsDevCatalogModel `json:"models"`
}

// modelsDevCatalogModel 表示 models.dev provider 目录下的单个模型。
//
// 只声明同步模型元数据需要的字段，其余 pricing、experimental、reasoning_options
// 等信息由价格同步链路或上游调用链路分别处理，避免模型目录同步越权修改业务配置。
type modelsDevCatalogModel struct {
	ID               string                     `json:"id"`
	Name             string                     `json:"name"`
	Family           string                     `json:"family"`
	Attachment       bool                       `json:"attachment"`
	Reasoning        bool                       `json:"reasoning"`
	ToolCall         bool                       `json:"tool_call"`
	StructuredOutput bool                       `json:"structured_output"`
	OpenWeights      bool                       `json:"open_weights"`
	Status           string                     `json:"status"`
	Modalities       modelsDevCatalogModalities `json:"modalities"`
	Limit            modelsDevCatalogLimit      `json:"limit"`
}

// modelsDevCatalogModalities 表示模型输入输出模态。
type modelsDevCatalogModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// modelsDevCatalogLimit 表示模型上下文和输出限制。
type modelsDevCatalogLimit struct {
	Context int64 `json:"context"`
	Input   int64 `json:"input"`
	Output  int64 `json:"output"`
}

// syncSourceInfo 描述一次同步实际使用的数据来源。
type syncSourceInfo struct {
	Source     string `json:"source"`
	Locale     string `json:"locale,omitempty"`
	ModelsURL  string `json:"models_url,omitempty"`
	VendorsURL string `json:"vendors_url,omitempty"`
	CatalogURL string `json:"catalog_url,omitempty"`
}

// syncUpstreamOptions 控制内部同步范围。
type syncUpstreamOptions struct {
	// CreateAllUpstream 为 true 时按上游完整目录补齐本地缺失模型；
	// 为 false 时保持旧行为，只补齐当前系统能力表中引用但 models 表缺失的模型。
	CreateAllUpstream bool
}

// syncUpstreamResult 是同步核心返回的结构化结果。
type syncUpstreamResult struct {
	CreatedModels  int            `json:"created_models"`
	CreatedVendors int            `json:"created_vendors"`
	UpdatedModels  int            `json:"updated_models"`
	SkippedModels  []string       `json:"skipped_models"`
	CreatedList    []string       `json:"created_list"`
	UpdatedList    []string       `json:"updated_list"`
	Source         syncSourceInfo `json:"source"`
}

// ETag 和响应体缓存，用于条件请求优化
var (
	etagCache  = make(map[string]string) // URL -> ETag 映射
	bodyCache  = make(map[string][]byte) // URL -> 响应体缓存
	cacheMutex sync.RWMutex              // 缓存读写锁
)

// overwriteField 覆盖字段配置
type overwriteField struct {
	ModelName string   `json:"model_name"`
	Fields    []string `json:"fields"`
}

// syncRequest 同步请求结构体
type syncRequest struct {
	Overwrite []overwriteField `json:"overwrite"`
	Locale    string           `json:"locale"`
	Source    string           `json:"source"`
}

// newHTTPClient 创建优化的 HTTP 客户端
//
// 配置了连接池、超时和 GitHub.io 的 IPv4/IPv6 回退策略
func newHTTPClient() *http.Client {
	timeoutSec := common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", 10)
	dialer := &net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}
	transport := &http.Transport{
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   time.Duration(timeoutSec) * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: time.Duration(timeoutSec) * time.Second,
	}
	if common.TLSInsecureSkipVerify {
		transport.TLSClientConfig = common.InsecureTLSConfig
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		if strings.HasSuffix(host, "github.io") {
			if conn, err := dialer.DialContext(ctx, "tcp4", addr); err == nil {
				return conn, nil
			}
			return dialer.DialContext(ctx, "tcp6", addr)
		}
		return dialer.DialContext(ctx, network, addr)
	}
	return &http.Client{Transport: transport}
}

var (
	httpClientOnce sync.Once
	httpClient     *http.Client
)

// getHTTPClient 获取单例 HTTP 客户端
func getHTTPClient() *http.Client {
	httpClientOnce.Do(func() {
		httpClient = newHTTPClient()
	})
	return httpClient
}

// fetchJSON 从上游获取 JSON 数据
//
// 支持：
// - ETag 条件请求（304 Not Modified）
// - 自动重试与指数退避
// - 响应体大小限制
// - 信封格式和纯数组格式自动识别
//
// 参数：
//   - ctx: 上下文
//   - url: 请求 URL
//   - out: 输出结构体
func fetchJSON[T any](ctx context.Context, url string, out *upstreamEnvelope[T]) error {
	var lastErr error
	attempts := common.GetEnvOrDefault("SYNC_HTTP_RETRY", 3)
	if attempts < 1 {
		attempts = 1
	}
	baseDelay := 200 * time.Millisecond
	maxMB := common.GetEnvOrDefault("SYNC_HTTP_MAX_MB", 10)
	maxBytes := int64(maxMB) << 20
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		// ETag conditional request
		cacheMutex.RLock()
		if et := etagCache[url]; et != "" {
			req.Header.Set("If-None-Match", et)
		}
		cacheMutex.RUnlock()

		resp, err := getHTTPClient().Do(req)
		if err != nil {
			lastErr = err
			// backoff with jitter
			sleep := baseDelay * time.Duration(1<<attempt)
			jitter := time.Duration(rand.Intn(150)) * time.Millisecond
			time.Sleep(sleep + jitter)
			continue
		}
		func() {
			defer resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusOK:
				// read body into buffer for caching and flexible decode
				limited := io.LimitReader(resp.Body, maxBytes)
				buf, err := io.ReadAll(limited)
				if err != nil {
					lastErr = err
					return
				}
				// cache body and ETag
				cacheMutex.Lock()
				if et := resp.Header.Get("ETag"); et != "" {
					etagCache[url] = et
				}
				bodyCache[url] = buf
				cacheMutex.Unlock()

				// 先按信封格式解析，失败后兼容纯数组格式。
				if err := common.Unmarshal(buf, out); err != nil {
					var arr []T
					if err2 := common.Unmarshal(buf, &arr); err2 != nil {
						lastErr = err
						return
					}
					out.Success = true
					out.Data = arr
					out.Message = ""
				} else {
					if !out.Success && len(out.Data) == 0 && out.Message == "" {
						out.Success = true
					}
				}
				lastErr = nil
			case http.StatusNotModified:
				// use cache
				cacheMutex.RLock()
				buf := bodyCache[url]
				cacheMutex.RUnlock()
				if len(buf) == 0 {
					lastErr = errors.New("cache miss for 304 response")
					return
				}
				if err := common.Unmarshal(buf, out); err != nil {
					var arr []T
					if err2 := common.Unmarshal(buf, &arr); err2 != nil {
						lastErr = err
						return
					}
					out.Success = true
					out.Data = arr
					out.Message = ""
				} else {
					if !out.Success && len(out.Data) == 0 && out.Message == "" {
						out.Success = true
					}
				}
				lastErr = nil
			default:
				lastErr = errors.New(resp.Status)
			}
		}()
		if lastErr == nil {
			return nil
		}
		sleep := baseDelay * time.Duration(1<<attempt)
		jitter := time.Duration(rand.Intn(150)) * time.Millisecond
		time.Sleep(sleep + jitter)
	}
	return lastErr
}

// fetchRawJSON 从上游获取原始 JSON 数据。
//
// 与 fetchJSON 共享 ETag 和响应体缓存，供对象结构不同于 upstreamEnvelope 的来源使用。
func fetchRawJSON(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	attempts := common.GetEnvOrDefault("SYNC_HTTP_RETRY", 3)
	if attempts < 1 {
		attempts = 1
	}
	baseDelay := 200 * time.Millisecond
	maxMB := common.GetEnvOrDefault("SYNC_HTTP_MAX_MB", 10)
	maxBytes := int64(maxMB) << 20
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		cacheMutex.RLock()
		if et := etagCache[url]; et != "" {
			req.Header.Set("If-None-Match", et)
		}
		cacheMutex.RUnlock()

		resp, err := getHTTPClient().Do(req)
		if err != nil {
			lastErr = err
			sleep := baseDelay * time.Duration(1<<attempt)
			jitter := time.Duration(rand.Intn(150)) * time.Millisecond
			time.Sleep(sleep + jitter)
			continue
		}

		var body []byte
		func() {
			defer resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusOK:
				limited := io.LimitReader(resp.Body, maxBytes)
				buf, err := io.ReadAll(limited)
				if err != nil {
					lastErr = err
					return
				}
				cacheMutex.Lock()
				if et := resp.Header.Get("ETag"); et != "" {
					etagCache[url] = et
				}
				bodyCache[url] = buf
				cacheMutex.Unlock()
				body = buf
				lastErr = nil
			case http.StatusNotModified:
				cacheMutex.RLock()
				buf := bodyCache[url]
				cacheMutex.RUnlock()
				if len(buf) == 0 {
					lastErr = errors.New("cache miss for 304 response")
					return
				}
				body = buf
				lastErr = nil
			default:
				lastErr = errors.New(resp.Status)
			}
		}()
		if lastErr == nil {
			return body, nil
		}
		sleep := baseDelay * time.Duration(1<<attempt)
		jitter := time.Duration(rand.Intn(150)) * time.Millisecond
		time.Sleep(sleep + jitter)
	}
	return nil, lastErr
}

// fetchModelsDevCatalog 拉取并解析 models.dev catalog。
func fetchModelsDevCatalog(ctx context.Context, catalogURL string) (*modelsDevCatalog, error) {
	buf, err := fetchRawJSON(ctx, catalogURL)
	if err != nil {
		return nil, err
	}
	var catalog modelsDevCatalog
	if err := common.Unmarshal(buf, &catalog); err != nil {
		return nil, err
	}
	if len(catalog.Providers) == 0 {
		return nil, errors.New("models.dev catalog providers is empty")
	}
	return &catalog, nil
}

// convertModelsDevCatalog 将 models.dev catalog 转成本项目现有同步流程使用的结构。
//
// 转换规则：
// 1. provider 目录中的模型 ID 才视为可请求模型名称；
// 2. 同名模型可能由多个聚合商提供，按 provider ID 字典序稳定保留第一条，避免每日同步反复改归属；
// 3. deprecated 模型默认仍创建但状态为禁用，便于历史日志、价格表和管理员手动启用排查；
// 4. tags 和 description 只来自公开元数据，不写入价格倍率，价格由独立 ratio sync 链路处理。
func convertModelsDevCatalog(catalog *modelsDevCatalog) ([]upstreamVendor, []upstreamModel) {
	if catalog == nil || len(catalog.Providers) == 0 {
		return nil, nil
	}

	providerIDs := make([]string, 0, len(catalog.Providers))
	for providerID := range catalog.Providers {
		providerIDs = append(providerIDs, providerID)
	}
	sortModelsDevProviderIDs(providerIDs)

	vendors := make([]upstreamVendor, 0, len(providerIDs))
	models := make([]upstreamModel, 0)
	seenModels := make(map[string]struct{})

	for _, providerID := range providerIDs {
		provider := catalog.Providers[providerID]
		providerName := strings.TrimSpace(provider.Name)
		if providerName == "" {
			providerName = providerID
		}
		vendors = append(vendors, upstreamVendor{
			Description: buildModelsDevVendorDescription(provider),
			Icon:        modelsDevCatalogProviderIcon(providerID, providerName),
			Name:        providerName,
			Status:      1,
		})

		modelIDs := make([]string, 0, len(provider.Models))
		for modelID := range provider.Models {
			modelIDs = append(modelIDs, modelID)
		}
		sort.Strings(modelIDs)

		for _, modelID := range modelIDs {
			def := provider.Models[modelID]
			name := strings.TrimSpace(def.ID)
			if name == "" {
				name = modelID
			}
			if name == "" {
				continue
			}
			if _, exists := seenModels[name]; exists {
				continue
			}
			seenModels[name] = struct{}{}
			models = append(models, upstreamModel{
				Description: buildModelsDevModelDescription(providerName, def),
				Icon:        modelsDevCatalogProviderIcon(providerID, providerName),
				ModelName:   name,
				NameRule:    model.NameRuleExact,
				Status:      modelsDevModelStatus(def.Status),
				Tags:        buildModelsDevModelTags(def),
				VendorName:  providerName,
			})
		}
	}

	return vendors, models
}

// sortModelsDevProviderIDs 按稳定优先级排序 provider。
//
// 多个 provider 可能暴露相同模型名。优先保留模型原厂或常见直接提供商，
// 聚合商和路由商排在后面，避免 gpt-5、claude-* 等模型被归属到随机聚合源。
func sortModelsDevProviderIDs(providerIDs []string) {
	sort.Slice(providerIDs, func(i, j int) bool {
		pi := modelsDevProviderPriority(providerIDs[i])
		pj := modelsDevProviderPriority(providerIDs[j])
		if pi != pj {
			return pi < pj
		}
		return providerIDs[i] < providerIDs[j]
	})
}

// modelsDevProviderPriority 返回 provider 归属优先级，数值越小越优先。
func modelsDevProviderPriority(providerID string) int {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "openai", "anthropic", "google", "deepseek", "xai", "moonshotai", "mistral", "cohere", "alibaba", "alibaba-cn", "perplexity", "jina", "aws", "azure", "vertex-ai":
		return 0
	case "openrouter", "requesty", "vercel", "cloudflare", "siliconflow", "replicate":
		return 20
	default:
		return 10
	}
}

// buildModelsDevVendorDescription 生成 provider 描述。
func buildModelsDevVendorDescription(provider modelsDevCatalogProvider) string {
	parts := []string{"Provider metadata synced from models.dev."}
	if strings.TrimSpace(provider.ID) != "" {
		parts = append(parts, "Provider ID: "+strings.TrimSpace(provider.ID)+".")
	}
	if strings.TrimSpace(provider.Doc) != "" {
		parts = append(parts, "Docs: "+strings.TrimSpace(provider.Doc))
	}
	return strings.Join(parts, " ")
}

// buildModelsDevModelDescription 生成模型描述。
func buildModelsDevModelDescription(providerName string, def modelsDevCatalogModel) string {
	displayName := strings.TrimSpace(def.Name)
	if displayName == "" {
		displayName = strings.TrimSpace(def.ID)
	}
	if displayName == "" {
		displayName = "This model"
	}

	parts := []string{fmt.Sprintf("%s is an AI model provided by %s.", displayName, providerName)}
	if def.Limit.Context > 0 {
		parts = append(parts, fmt.Sprintf("Context window: %d tokens.", def.Limit.Context))
	}
	if def.Limit.Output > 0 {
		parts = append(parts, fmt.Sprintf("Max output: %d tokens.", def.Limit.Output))
	}
	return strings.Join(parts, " ")
}

// buildModelsDevModelTags 根据 capabilities 生成逗号分隔标签。
func buildModelsDevModelTags(def modelsDevCatalogModel) string {
	tags := make([]string, 0, 8)
	if def.Reasoning {
		tags = append(tags, "Reasoning")
	}
	if def.ToolCall {
		tags = append(tags, "Tools")
	}
	if def.Attachment {
		tags = append(tags, "Files")
	}
	if def.StructuredOutput {
		tags = append(tags, "Structured Output")
	}
	if def.OpenWeights {
		tags = append(tags, "Open Weights")
	}
	for _, modality := range append(def.Modalities.Input, def.Modalities.Output...) {
		switch strings.ToLower(strings.TrimSpace(modality)) {
		case "image":
			tags = append(tags, "Vision")
		case "audio":
			tags = append(tags, "Audio")
		case "video":
			tags = append(tags, "Video")
		case "pdf":
			tags = append(tags, "PDF")
		}
	}
	if def.Limit.Context > 0 {
		tags = append(tags, formatContextTag(def.Limit.Context))
	}
	return strings.Join(uniqueNonEmptyStrings(tags), ",")
}

// formatContextTag 将 context tokens 格式化为后台列表里常见的短标签。
func formatContextTag(contextTokens int64) string {
	if contextTokens >= 1000000 {
		return fmt.Sprintf("%dM", contextTokens/1000000)
	}
	if contextTokens >= 1000 {
		return fmt.Sprintf("%dK", contextTokens/1000)
	}
	return fmt.Sprintf("%d", contextTokens)
}

// modelsDevModelStatus 将 models.dev 的状态映射为本地状态。
func modelsDevModelStatus(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "deprecated", "disabled", "inactive":
		return 0
	default:
		return 1
	}
}

// modelsDevCatalogProviderIcon 返回尽可能兼容 @lobehub/icons 的图标名称。
func modelsDevCatalogProviderIcon(providerID, providerName string) string {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "openai":
		return "OpenAI.Color"
	case "anthropic":
		return "Claude.Color"
	case "google":
		return "Gemini.Color"
	case "xai":
		return "XAI.Color"
	case "moonshotai":
		return "Moonshot.Color"
	case "mistral":
		return "Mistral.Color"
	case "cohere":
		return "Cohere.Color"
	case "deepseek":
		return "DeepSeek.Color"
	case "alibaba", "alibaba-cn":
		return "Qwen.Color"
	case "openrouter":
		return "OpenRouter.Color"
	case "perplexity":
		return "Perplexity.Color"
	case "siliconflow":
		return "SiliconCloud.Color"
	case "jina":
		return "Jina.Color"
	case "aws":
		return "Aws.Color"
	case "replicate":
		return "Replicate.Color"
	}
	return strings.TrimSpace(providerName)
}

// uniqueNonEmptyStrings 保留顺序去重。
func uniqueNonEmptyStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

// ensureVendorID 确保供应商存在，不存在则创建
//
// 返回供应商 ID，使用本地缓存避免重复查询
func ensureVendorID(vendorName string, vendorByName map[string]upstreamVendor, vendorIDCache map[string]int, createdVendors *int) int {
	if vendorName == "" {
		return 0
	}
	if id, ok := vendorIDCache[vendorName]; ok {
		return id
	}
	var existing model.Vendor
	if err := model.DB.Where("name = ?", vendorName).First(&existing).Error; err == nil {
		vendorIDCache[vendorName] = existing.Id
		return existing.Id
	}
	uv := vendorByName[vendorName]
	v := &model.Vendor{
		Name:        vendorName,
		Description: uv.Description,
		Icon:        coalesce(uv.Icon, ""),
		Status:      chooseStatus(uv.Status, 1),
	}
	if err := v.Insert(); err == nil {
		*createdVendors++
		vendorIDCache[vendorName] = v.Id
		return v.Id
	}
	vendorIDCache[vendorName] = 0
	return 0
}

// buildSyncSourceInfo 根据请求生成来源信息。
func buildSyncSourceInfo(req syncRequest) syncSourceInfo {
	source := normalizeSyncSource(req.Source)
	switch source {
	case syncSourceModelsDev:
		return syncSourceInfo{
			Source:     syncSourceModelsDev,
			CatalogURL: getModelsDevCatalogURL(),
		}
	default:
		modelsURL, vendorsURL := getUpstreamURLs(req.Locale)
		return syncSourceInfo{
			Source:     syncSourceOfficial,
			Locale:     req.Locale,
			ModelsURL:  modelsURL,
			VendorsURL: vendorsURL,
		}
	}
}

// fetchSyncUpstreamData 拉取指定来源的供应商和模型数据。
func fetchSyncUpstreamData(ctx context.Context, req syncRequest) ([]upstreamVendor, []upstreamModel, syncSourceInfo, error) {
	sourceInfo := buildSyncSourceInfo(req)
	switch sourceInfo.Source {
	case syncSourceModelsDev:
		catalog, err := fetchModelsDevCatalog(ctx, sourceInfo.CatalogURL)
		if err != nil {
			return nil, nil, sourceInfo, err
		}
		vendors, models := convertModelsDevCatalog(catalog)
		return vendors, models, sourceInfo, nil
	default:
		var vendorsEnv upstreamEnvelope[upstreamVendor]
		var modelsEnv upstreamEnvelope[upstreamModel]
		var fetchErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			// vendor 失败不拦截，缺少 vendor 时模型仍可创建，只是不绑定供应商。
			_ = fetchJSON(ctx, sourceInfo.VendorsURL, &vendorsEnv)
		}()
		go func() {
			defer wg.Done()
			if err := fetchJSON(ctx, sourceInfo.ModelsURL, &modelsEnv); err != nil {
				fetchErr = err
			}
		}()
		wg.Wait()
		if fetchErr != nil {
			return nil, nil, sourceInfo, fetchErr
		}
		return vendorsEnv.Data, modelsEnv.Data, sourceInfo, nil
	}
}

// syncUpstreamModelsCore 执行模型目录同步。
//
// HTTP 手动同步和每日 models.dev 自动同步共用该函数，确保写库规则一致：
// - 默认手动同步保持旧行为，只补齐当前能力表引用但缺少元数据的模型；
// - 自动同步可以设置 CreateAllUpstream，按上游完整目录补齐本地不存在的模型；
// - 不覆盖用户手动编辑，除非请求显式指定 overwrite 字段；
// - 创建出来的模型明确设置 sync_official=1，后续仍可参与官方数据差异预览。
func syncUpstreamModelsCore(ctx context.Context, req syncRequest, opts syncUpstreamOptions) (*syncUpstreamResult, error) {
	req.Source = normalizeSyncSource(req.Source)
	sourceInfo := buildSyncSourceInfo(req)
	emptyResult := &syncUpstreamResult{
		SkippedModels: []string{},
		CreatedList:   []string{},
		UpdatedList:   []string{},
		Source:        sourceInfo,
	}

	if !opts.CreateAllUpstream && len(req.Overwrite) == 0 {
		missing, err := model.GetMissingModels()
		if err != nil {
			return nil, err
		}
		if len(missing) == 0 {
			return emptyResult, nil
		}
	}

	vendors, upstreamModels, sourceInfo, err := fetchSyncUpstreamData(ctx, req)
	if err != nil {
		return nil, err
	}

	vendorByName := make(map[string]upstreamVendor)
	for _, v := range vendors {
		if v.Name != "" {
			vendorByName[v.Name] = v
		}
	}
	modelByName := make(map[string]upstreamModel)
	for _, m := range upstreamModels {
		if m.ModelName != "" {
			modelByName[m.ModelName] = m
		}
	}

	targetNames, err := buildSyncTargetModelNames(modelByName, opts)
	if err != nil {
		return nil, err
	}

	result := &syncUpstreamResult{
		SkippedModels: make([]string, 0),
		CreatedList:   make([]string, 0),
		UpdatedList:   make([]string, 0),
		Source:        sourceInfo,
	}

	// 本地缓存：vendorName -> id
	vendorIDCache := make(map[string]int)

	for _, name := range targetNames {
		if len(name) > 128 {
			result.SkippedModels = append(result.SkippedModels, name)
			continue
		}
		up, ok := modelByName[name]
		if !ok {
			result.SkippedModels = append(result.SkippedModels, name)
			continue
		}

		// 若本地已存在且设置为不同步，则跳过（极端情况：缺失列表与本地状态不同步时）
		var existing model.Model
		if err := model.DB.Where("model_name = ?", name).First(&existing).Error; err == nil {
			if existing.SyncOfficial == 0 {
				result.SkippedModels = append(result.SkippedModels, name)
				continue
			}
			continue
		}

		// 确保 vendor 存在
		vendorID := ensureVendorID(up.VendorName, vendorByName, vendorIDCache, &result.CreatedVendors)

		// 创建模型
		mi := &model.Model{
			ModelName:    name,
			Description:  up.Description,
			Icon:         up.Icon,
			Tags:         up.Tags,
			VendorID:     vendorID,
			Status:       chooseSyncModelStatus(up.Status, sourceInfo.Source, 1),
			SyncOfficial: 1,
			NameRule:     up.NameRule,
		}
		if err := mi.Insert(); err == nil {
			result.CreatedModels++
			result.CreatedList = append(result.CreatedList, name)
		} else {
			common.SysError("failed to insert synced model " + name + ": " + err.Error())
			result.SkippedModels = append(result.SkippedModels, name)
		}
	}

	// 处理可选覆盖（更新本地已有模型的差异字段）
	if len(req.Overwrite) > 0 {
		// vendorIDCache 已用于创建阶段，可复用
		for _, ow := range req.Overwrite {
			up, ok := modelByName[ow.ModelName]
			if !ok {
				continue
			}
			var local model.Model
			if err := model.DB.Where("model_name = ?", ow.ModelName).First(&local).Error; err != nil {
				continue
			}

			// 跳过被禁用官方同步的模型
			if local.SyncOfficial == 0 {
				continue
			}

			// 映射 vendor
			newVendorID := ensureVendorID(up.VendorName, vendorByName, vendorIDCache, &result.CreatedVendors)

			// 应用字段覆盖（事务）
			_ = model.DB.Transaction(func(tx *gorm.DB) error {
				needUpdate := false
				if containsField(ow.Fields, "description") {
					local.Description = up.Description
					needUpdate = true
				}
				if containsField(ow.Fields, "icon") {
					local.Icon = up.Icon
					needUpdate = true
				}
				if containsField(ow.Fields, "tags") {
					local.Tags = up.Tags
					needUpdate = true
				}
				if containsField(ow.Fields, "vendor") {
					local.VendorID = newVendorID
					needUpdate = true
				}
				if containsField(ow.Fields, "name_rule") {
					local.NameRule = up.NameRule
					needUpdate = true
				}
				if containsField(ow.Fields, "status") {
					local.Status = chooseSyncModelStatus(up.Status, sourceInfo.Source, local.Status)
					needUpdate = true
				}
				if !needUpdate {
					return nil
				}
				if err := tx.Save(&local).Error; err != nil {
					return err
				}
				result.UpdatedModels++
				result.UpdatedList = append(result.UpdatedList, ow.ModelName)
				return nil
			})
		}
	}
	return result, nil
}

// buildSyncTargetModelNames 计算本轮需要创建的模型名称。
func buildSyncTargetModelNames(modelByName map[string]upstreamModel, opts syncUpstreamOptions) ([]string, error) {
	if !opts.CreateAllUpstream {
		return model.GetMissingModels()
	}

	var existing []string
	if err := model.DB.Model(&model.Model{}).Pluck("model_name", &existing).Error; err != nil {
		return nil, err
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, name := range existing {
		existingSet[name] = struct{}{}
	}

	names := make([]string, 0, len(modelByName))
	for name := range modelByName {
		if _, exists := existingSet[name]; !exists {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// chooseSyncModelStatus 根据来源选择模型状态。
//
// 旧官方仓库中 status=0 代表字段缺省，沿用历史逻辑回退为启用；
// models.dev 中 deprecated 会被转换为 0，必须显式保留为禁用状态。
func chooseSyncModelStatus(status int, source string, fallback int) int {
	if normalizeSyncSource(source) == syncSourceModelsDev {
		return status
	}
	return chooseStatus(status, fallback)
}

// SyncUpstreamModels 同步上游模型与供应商：
// - 默认仅创建「未配置模型」
// - 可通过 overwrite 选择性覆盖更新本地已有模型的字段（前提：sync_official <> 0）
func SyncUpstreamModels(c *gin.Context) {
	var req syncRequest
	// 允许空体
	_ = c.ShouldBindJSON(&req)

	timeoutSec := common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", 15)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	result, err := syncUpstreamModelsCore(ctx, req, syncUpstreamOptions{})
	if err != nil {
		common.SysError("failed to sync upstream models: " + err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取上游模型失败: " + err.Error(),
			"source":  buildSyncSourceInfo(req),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// containsField 检查字段列表中是否包含指定字段（忽略大小写）
func containsField(fields []string, key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, f := range fields {
		if strings.ToLower(strings.TrimSpace(f)) == key {
			return true
		}
	}
	return false
}

// coalesce 返回第一个非空字符串
func coalesce(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// chooseStatus 选择状态值，优先使用主值，主值为 0 时使用备选值
func chooseStatus(primary, fallback int) int {
	if primary == 0 && fallback != 0 {
		return fallback
	}
	if primary != 0 {
		return primary
	}
	return 1
}

// SyncUpstreamPreview 预览上游与本地的差异
//
// 返回缺失模型列表和冲突字段详情，用于同步前的确认弹窗
func SyncUpstreamPreview(c *gin.Context) {
	// 1) 拉取上游数据
	timeoutSec := common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", 15)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	locale := c.Query("locale")
	source := c.Query("source")
	req := syncRequest{Locale: locale, Source: source}
	vendors, upstreamModels, sourceInfo, fetchErr := fetchSyncUpstreamData(ctx, req)
	if fetchErr != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取上游模型失败: " + fetchErr.Error(), "source": sourceInfo})
		return
	}

	vendorByName := make(map[string]upstreamVendor)
	for _, v := range vendors {
		if v.Name != "" {
			vendorByName[v.Name] = v
		}
	}
	modelByName := make(map[string]upstreamModel)
	upstreamNames := make([]string, 0, len(upstreamModels))
	for _, m := range upstreamModels {
		if m.ModelName != "" {
			modelByName[m.ModelName] = m
			upstreamNames = append(upstreamNames, m.ModelName)
		}
	}

	// 2) 本地已有模型
	var locals []model.Model
	if len(upstreamNames) > 0 {
		_ = model.DB.Where("model_name IN ? AND sync_official <> 0", upstreamNames).Find(&locals).Error
	}

	// 本地 vendor 名称映射
	vendorIdSet := make(map[int]struct{})
	for _, m := range locals {
		if m.VendorID != 0 {
			vendorIdSet[m.VendorID] = struct{}{}
		}
	}
	vendorIDs := make([]int, 0, len(vendorIdSet))
	for id := range vendorIdSet {
		vendorIDs = append(vendorIDs, id)
	}
	idToVendorName := make(map[int]string)
	if len(vendorIDs) > 0 {
		var dbVendors []model.Vendor
		_ = model.DB.Where("id IN ?", vendorIDs).Find(&dbVendors).Error
		for _, v := range dbVendors {
			idToVendorName[v.Id] = v.Name
		}
	}

	// 3) 缺失且上游存在的模型
	missingList, _ := model.GetMissingModels()
	var missing []string
	for _, name := range missingList {
		if _, ok := modelByName[name]; ok {
			missing = append(missing, name)
		}
	}

	// 4) 计算冲突字段
	type conflictField struct {
		Field    string      `json:"field"`
		Local    interface{} `json:"local"`
		Upstream interface{} `json:"upstream"`
	}
	type conflictItem struct {
		ModelName string          `json:"model_name"`
		Fields    []conflictField `json:"fields"`
	}

	var conflicts []conflictItem
	for _, local := range locals {
		up, ok := modelByName[local.ModelName]
		if !ok {
			continue
		}
		fields := make([]conflictField, 0, 6)
		if strings.TrimSpace(local.Description) != strings.TrimSpace(up.Description) {
			fields = append(fields, conflictField{Field: "description", Local: local.Description, Upstream: up.Description})
		}
		if strings.TrimSpace(local.Icon) != strings.TrimSpace(up.Icon) {
			fields = append(fields, conflictField{Field: "icon", Local: local.Icon, Upstream: up.Icon})
		}
		if strings.TrimSpace(local.Tags) != strings.TrimSpace(up.Tags) {
			fields = append(fields, conflictField{Field: "tags", Local: local.Tags, Upstream: up.Tags})
		}
		// vendor 对比使用名称
		localVendor := idToVendorName[local.VendorID]
		if strings.TrimSpace(localVendor) != strings.TrimSpace(up.VendorName) {
			fields = append(fields, conflictField{Field: "vendor", Local: localVendor, Upstream: up.VendorName})
		}
		if local.NameRule != up.NameRule {
			fields = append(fields, conflictField{Field: "name_rule", Local: local.NameRule, Upstream: up.NameRule})
		}
		if local.Status != chooseStatus(up.Status, local.Status) {
			fields = append(fields, conflictField{Field: "status", Local: local.Status, Upstream: up.Status})
		}
		if len(fields) > 0 {
			conflicts = append(conflicts, conflictItem{ModelName: local.ModelName, Fields: fields})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"missing":   missing,
			"conflicts": conflicts,
			"source":    sourceInfo,
		},
	})
}
