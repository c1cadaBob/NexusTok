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
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/modelcatalog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// rawJSONMessage 只作为延迟解析字段的字节容器使用，实际 JSON 解析仍通过 common.* 完成。
type rawJSONMessage []byte

// UnmarshalJSON 保留原始 JSON 字节，避免 endpoints 这类结构不固定的字段被提前解析。
func (m *rawJSONMessage) UnmarshalJSON(data []byte) error {
	if m == nil {
		return nil
	}
	*m = append((*m)[0:0], data...)
	return nil
}

// MarshalJSON 按原样输出原始 JSON 字节；空值输出 null。
func (m rawJSONMessage) MarshalJSON() ([]byte, error) {
	if len(m) == 0 {
		return []byte("null"), nil
	}
	return m, nil
}

// 同步来源常量。
//
// official 作为历史 API 值保留，但用户可见含义已经收敛为 NexusTok 项目内模型仓库；
// models.dev 仍优先读取官网 catalog，并在网络失败时降级 GitHub TOML 和内置仓库。
const (
	syncSourceOfficial  = "official"
	syncSourceModelsDev = "models.dev"
	syncSourceConfig    = "config"

	modelsDevDefaultBaseURL = "https://models.dev"
	modelsDevCatalogPath    = "/catalog.json"

	modelsDevGitHubRepo           = "anomalyco/models.dev"
	modelsDevGitHubDefaultTreeURL = "https://api.github.com/repos/anomalyco/models.dev/git/trees/dev?recursive=1"
	modelsDevGitHubDefaultRawBase = "https://raw.githubusercontent.com/anomalyco/models.dev/dev"
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

// normalizeSyncSource 标准化同步来源。
func normalizeSyncSource(source string) string {
	normalized := strings.ToLower(strings.TrimSpace(source))
	switch normalized {
	case "", syncSourceOfficial, "repository", "upstream":
		return syncSourceOfficial
	case "nexustok", "nexustok_repository", "nexustok-repository":
		return syncSourceOfficial
	case syncSourceModelsDev, "modelsdev", "models_dev", "models-dev", "models":
		return syncSourceModelsDev
	case syncSourceConfig, "configuration", "file", "config-file":
		return syncSourceConfig
	default:
		return normalized
	}
}

func validateSyncSource(source string) error {
	switch normalizeSyncSource(source) {
	case syncSourceOfficial, syncSourceModelsDev:
		return nil
	case syncSourceConfig:
		return errors.New("配置文件导入同步方式已停用，请使用 NexusTok 模型仓库或 Models.dev")
	default:
		return fmt.Errorf("不支持的模型同步来源: %s", strings.TrimSpace(source))
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

func getModelsDevGitHubTreeURL() string {
	return common.GetEnvOrDefaultString("MODELS_DEV_GITHUB_TREE_URL", modelsDevGitHubDefaultTreeURL)
}

func getModelsDevGitHubTarURL() string {
	return common.GetEnvOrDefaultString("MODELS_DEV_GITHUB_TAR_URL", modelcatalog.ModelsDevGitHubDefaultTarURL)
}

func getModelsDevGitHubZipURL() string {
	return common.GetEnvOrDefaultString("MODELS_DEV_GITHUB_ZIP_URL", modelcatalog.ModelsDevGitHubDefaultZipURL)
}

func getModelsDevGitHubRawBase() string {
	return strings.TrimRight(common.GetEnvOrDefaultString("MODELS_DEV_GITHUB_RAW_BASE", modelsDevGitHubDefaultRawBase), "/")
}

// upstreamEnvelope 上游 API 响应信封结构体
type upstreamEnvelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    []T    `json:"data"`
}

// upstreamModel 上游模型数据结构体
type upstreamModel struct {
	Description string         `json:"description"`
	Endpoints   rawJSONMessage `json:"endpoints"`
	Icon        string         `json:"icon"`
	ModelName   string         `json:"model_name"`
	NameRule    int            `json:"name_rule"`
	Status      int            `json:"status"`
	Tags        string         `json:"tags"`
	VendorName  string         `json:"vendor_name"`
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
// 本地“供应商”字段应优先跟随 canonical models 的归属方，而不是把 serving provider
// 误写成模型供应商。这样像 openai/gpt-5.5 这类 canonical 模型，即使由 Vivgrid
// 等服务商提供，也会正确归属到 OpenAI。
type modelsDevCatalog struct {
	Models    map[string]modelsDevCatalogModel    `json:"models"`
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
	Cost             modelsDevCatalogCost       `json:"cost"`
}

// modelsDevCatalogCost 是 models.dev provider 模型中的价格结构。
// 当前公开数据使用真实美元价，单位通常是 $ / 1M tokens；本地保存时会转为
// 现有 ratio 配置，保证 relay 热路径不需要变更。
type modelsDevCatalogCost struct {
	Input       *float64 `json:"input"`
	Output      *float64 `json:"output"`
	CacheRead   *float64 `json:"cache_read"`
	CacheWrite  *float64 `json:"cache_write"`
	InputAudio  *float64 `json:"input_audio"`
	OutputAudio *float64 `json:"output_audio"`
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
	Source                string `json:"source"`
	Locale                string `json:"locale,omitempty"`
	ModelsURL             string `json:"models_url,omitempty"`
	VendorsURL            string `json:"vendors_url,omitempty"`
	CatalogURL            string `json:"catalog_url,omitempty"`
	CatalogOrigin         string `json:"catalog_origin,omitempty"`
	FallbackStage         string `json:"fallback_stage,omitempty"`
	GitHubRepo            string `json:"github_repo,omitempty"`
	CatalogVersion        string `json:"catalog_version,omitempty"`
	SourceModelCount      int    `json:"source_model_count,omitempty"`
	SourceProviderCount   int    `json:"source_provider_count,omitempty"`
	EmbeddedModelCount    int    `json:"embedded_model_count,omitempty"`
	EmbeddedProviderCount int    `json:"embedded_provider_count,omitempty"`
	FallbackUsed          bool   `json:"fallback_used,omitempty"`
	FallbackReason        string `json:"fallback_reason,omitempty"`
	FallbackName          string `json:"fallback_name,omitempty"`
	FallbackGeneratedAt   string `json:"fallback_generated_at,omitempty"`
}

// catalogWriteBackResult 描述开发环境模型仓库写回状态。
//
// 同步数据库和写回 Git 工作区是两个不同动作：生产环境通常只同步数据库，开发环境
// 显式开启 MODEL_CATALOG_WRITE_BACK 后才会把公开模型 catalog 写回 repository 文件。
type catalogWriteBackResult struct {
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
	ModelCount    int    `json:"model_count,omitempty"`
	ProviderCount int    `json:"provider_count,omitempty"`
	GeneratedAt   string `json:"generated_at,omitempty"`
	RepoDir       string `json:"repo_dir,omitempty"`
}

// syncUpstreamOptions 控制内部同步范围。
type syncUpstreamOptions struct {
	// CreateAllUpstream 为 true 时按上游完整目录补齐本地缺失模型；
	// 为 false 时保持旧行为，只补齐当前系统能力表中引用但 models 表缺失的模型。
	CreateAllUpstream bool
}

// syncPricingPolicyRequest 描述模型同步时是否同时应用上游价格。
// provider_order 是管理员指定的降级链，例如 openai -> azure -> openrouter；
// overwrite_manual=false 时，用户在模型页手动确认过的价格永远优先。
type syncPricingPolicyRequest struct {
	Enabled         bool     `json:"enabled"`
	OverwriteManual bool     `json:"overwrite_manual"`
	ProviderOrder   []string `json:"provider_order"`
}

// syncUpstreamResult 是同步核心返回的结构化结果。
type syncUpstreamResult struct {
	CreatedModels    int                     `json:"created_models"`
	CreatedVendors   int                     `json:"created_vendors"`
	UpdatedModels    int                     `json:"updated_models"`
	PricingUpdated   int                     `json:"pricing_updated,omitempty"`
	PricingSkipped   int                     `json:"pricing_skipped,omitempty"`
	SkippedModels    []string                `json:"skipped_models"`
	CreatedList      []string                `json:"created_list"`
	UpdatedList      []string                `json:"updated_list"`
	PricingList      []string                `json:"pricing_list,omitempty"`
	Source           syncSourceInfo          `json:"source"`
	CatalogWriteBack *catalogWriteBackResult `json:"catalog_write_back,omitempty"`
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
	Overwrite []overwriteField         `json:"overwrite"`
	Locale    string                   `json:"locale"`
	Source    string                   `json:"source"`
	Pricing   syncPricingPolicyRequest `json:"pricing"`
	CreateAll *bool                    `json:"create_all,omitempty"`
}

type modelsDevPricingCandidate struct {
	ProviderID   string
	ProviderName string
	Input        float64
	Output       *float64
	CacheRead    *float64
	CacheWrite   *float64
	InputAudio   *float64
	OutputAudio  *float64
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
	if len(catalog.Models) == 0 && len(catalog.Providers) == 0 {
		return nil, errors.New("models.dev catalog is empty")
	}
	return &catalog, nil
}

// fetchNexusTokRepositoryCatalog 读取随构建打包的项目内模型仓库。
//
// official API 值历史上表示远程官方仓库。现在保留该 API 值做兼容，但实际数据源
// 已切换为 Git 可审查的 NexusTok 内置模型仓库，避免生产环境继续依赖旧 basellm 远程源。
func fetchNexusTokRepositoryCatalog() (*modelcatalog.Catalog, syncSourceInfo, error) {
	catalog, err := modelcatalog.LoadEmbeddedCatalog()
	manifest := modelcatalog.LoadEmbeddedManifest()
	sourceInfo := syncSourceInfo{
		Source:                syncSourceOfficial,
		CatalogOrigin:         modelcatalog.CatalogOriginNexusTokRepository,
		FallbackName:          manifest.Name,
		FallbackGeneratedAt:   manifest.GeneratedAt,
		CatalogVersion:        manifest.Version,
		EmbeddedModelCount:    manifest.ModelCount,
		EmbeddedProviderCount: manifest.ProviderCount,
	}
	if err != nil {
		return nil, sourceInfo, err
	}
	if sourceInfo.FallbackName == "" {
		sourceInfo.FallbackName = catalog.Manifest.Name
	}
	if sourceInfo.FallbackGeneratedAt == "" {
		sourceInfo.FallbackGeneratedAt = catalog.Manifest.GeneratedAt
	}
	if sourceInfo.CatalogVersion == "" {
		sourceInfo.CatalogVersion = catalog.Manifest.Version
	}
	sourceInfo.SourceModelCount = len(catalog.Models)
	sourceInfo.SourceProviderCount = len(catalog.Providers)
	if sourceInfo.EmbeddedModelCount == 0 {
		sourceInfo.EmbeddedModelCount = len(catalog.Models)
	}
	if sourceInfo.EmbeddedProviderCount == 0 {
		sourceInfo.EmbeddedProviderCount = len(catalog.Providers)
	}
	return catalog, sourceInfo, nil
}

func convertModelCatalogToModelsDevCatalog(catalog *modelcatalog.Catalog) *modelsDevCatalog {
	if catalog == nil {
		return nil
	}
	result := &modelsDevCatalog{
		Models:    make(map[string]modelsDevCatalogModel, len(catalog.Models)),
		Providers: make(map[string]modelsDevCatalogProvider, len(catalog.Providers)),
	}
	for key, item := range catalog.Models {
		result.Models[key] = convertCatalogModel(item)
	}
	for providerID, provider := range catalog.Providers {
		converted := modelsDevCatalogProvider{
			ID:     coalesce(provider.ID, providerID),
			Name:   provider.Name,
			Doc:    provider.Doc,
			Models: make(map[string]modelsDevCatalogModel, len(provider.Models)),
		}
		for modelID, item := range provider.Models {
			converted.Models[modelID] = convertCatalogModel(item)
		}
		result.Providers[providerID] = converted
	}
	return result
}

func convertModelsDevCatalogToModelCatalog(catalog *modelsDevCatalog) *modelcatalog.Catalog {
	if catalog == nil {
		return nil
	}
	result := &modelcatalog.Catalog{
		Models:    make(map[string]modelcatalog.CatalogModel, len(catalog.Models)),
		Providers: make(map[string]modelcatalog.CatalogProvider, len(catalog.Providers)),
	}
	for key, item := range catalog.Models {
		ownerID, modelID := splitModelsDevCanonicalKey(key)
		if modelID == "" {
			modelID = strings.TrimSpace(item.ID)
		}
		if ownerID == "" {
			ownerID = providerIDFromModelsDevModel(item, catalog)
		}
		if ownerID == "" || modelID == "" {
			continue
		}
		result.Models[ownerID+"/"+modelID] = convertModelsDevModelToCatalog(item, modelID)
	}
	for providerID, provider := range catalog.Providers {
		converted := modelcatalog.CatalogProvider{
			ID:     coalesce(provider.ID, providerID),
			Name:   provider.Name,
			Doc:    provider.Doc,
			Status: "active",
			Icon:   modelsDevCatalogProviderIcon(providerID, provider.Name),
			Models: make(map[string]modelcatalog.CatalogModel, len(provider.Models)),
		}
		for modelID, item := range provider.Models {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				id = modelID
			}
			converted.Models[id] = convertModelsDevModelToCatalog(item, id)
		}
		result.Providers[converted.ID] = converted
	}
	result.Manifest = modelcatalog.BuildManifest(result, "")
	return result
}

func convertModelsDevModelToCatalog(item modelsDevCatalogModel, fallbackID string) modelcatalog.CatalogModel {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		id = fallbackID
	}
	if _, modelID := splitModelsDevCanonicalKey(id); modelID != "" && strings.Contains(id, "/") {
		id = modelID
	}
	return modelcatalog.CatalogModel{
		ID:               id,
		Name:             item.Name,
		Family:           item.Family,
		Status:           coalesce(item.Status, "active"),
		Attachment:       item.Attachment,
		Reasoning:        item.Reasoning,
		ToolCall:         item.ToolCall,
		StructuredOutput: item.StructuredOutput,
		OpenWeights:      item.OpenWeights,
		Limit: modelcatalog.CatalogLimit{
			Context: item.Limit.Context,
			Input:   item.Limit.Input,
			Output:  item.Limit.Output,
		},
		Modalities: modelcatalog.CatalogModalities{
			Input:  item.Modalities.Input,
			Output: item.Modalities.Output,
		},
		Cost: modelcatalog.CatalogCost{
			Input:       item.Cost.Input,
			Output:      item.Cost.Output,
			CacheRead:   item.Cost.CacheRead,
			CacheWrite:  item.Cost.CacheWrite,
			InputAudio:  item.Cost.InputAudio,
			OutputAudio: item.Cost.OutputAudio,
		},
		Source: modelcatalog.CatalogSourceTrace{
			Origin: modelcatalog.CatalogOriginModelsDevWeb,
		},
	}
}

func providerIDFromModelsDevModel(item modelsDevCatalogModel, catalog *modelsDevCatalog) string {
	modelID := strings.TrimSpace(item.ID)
	if ownerID, _, ok := strings.Cut(modelID, "/"); ok {
		return strings.TrimSpace(ownerID)
	}
	if catalog == nil {
		return ""
	}
	for providerID, provider := range catalog.Providers {
		if _, ok := provider.Models[modelID]; ok {
			return providerID
		}
	}
	return ""
}

func convertCatalogModel(item modelcatalog.CatalogModel) modelsDevCatalogModel {
	return modelsDevCatalogModel{
		ID:               item.ID,
		Name:             item.Name,
		Family:           item.Family,
		Attachment:       item.Attachment,
		Reasoning:        item.Reasoning,
		ToolCall:         item.ToolCall,
		StructuredOutput: item.StructuredOutput,
		OpenWeights:      item.OpenWeights,
		Status:           item.Status,
		Modalities: modelsDevCatalogModalities{
			Input:  item.Modalities.Input,
			Output: item.Modalities.Output,
		},
		Limit: modelsDevCatalogLimit{
			Context: item.Limit.Context,
			Input:   item.Limit.Input,
			Output:  item.Limit.Output,
		},
		Cost: modelsDevCatalogCost{
			Input:       item.Cost.Input,
			Output:      item.Cost.Output,
			CacheRead:   item.Cost.CacheRead,
			CacheWrite:  item.Cost.CacheWrite,
			InputAudio:  item.Cost.InputAudio,
			OutputAudio: item.Cost.OutputAudio,
		},
	}
}

func convertModelCatalogToUpstream(catalog *modelcatalog.Catalog) ([]upstreamVendor, []upstreamModel) {
	if catalog == nil {
		return nil, nil
	}
	vendors := make([]upstreamVendor, 0, len(catalog.Providers))
	providerIDs := make([]string, 0, len(catalog.Providers))
	for providerID := range catalog.Providers {
		providerIDs = append(providerIDs, providerID)
	}
	sortModelsDevProviderIDs(providerIDs)
	for _, providerID := range providerIDs {
		provider := catalog.Providers[providerID]
		name := strings.TrimSpace(provider.Name)
		if name == "" {
			name = providerID
		}
		vendors = append(vendors, upstreamVendor{
			Description: provider.Description,
			Icon:        coalesce(provider.Icon, modelsDevCatalogProviderIcon(providerID, name)),
			Name:        name,
			Status:      1,
		})
	}

	modelKeys := make([]string, 0, len(catalog.Models))
	for key := range catalog.Models {
		modelKeys = append(modelKeys, key)
	}
	sort.Strings(modelKeys)
	models := make([]upstreamModel, 0, len(modelKeys))
	for _, key := range modelKeys {
		ownerID, modelName := splitModelsDevCanonicalKey(key)
		def := catalog.Models[key]
		if strings.TrimSpace(def.ID) != "" && !strings.Contains(strings.TrimSpace(def.ID), "/") {
			modelName = strings.TrimSpace(def.ID)
		}
		if modelName == "" {
			continue
		}
		provider := catalog.Providers[ownerID]
		providerName := strings.TrimSpace(provider.Name)
		if providerName == "" {
			providerName = ownerID
		}
		tags := strings.Join(uniqueNonEmptyStrings(def.Tags), ",")
		if tags == "" {
			tags = buildModelsDevModelTags(convertCatalogModel(def))
		}
		models = append(models, upstreamModel{
			Description: coalesce(def.Description, buildModelsDevModelDescription(providerName, convertCatalogModel(def))),
			Icon:        coalesce(def.Icon, coalesce(provider.Icon, modelsDevCatalogProviderIcon(ownerID, providerName))),
			ModelName:   modelName,
			NameRule:    def.NameRule,
			Status:      modelsDevModelStatus(def.Status),
			Tags:        tags,
			VendorName:  providerName,
		})
	}
	return vendors, models
}

func extractCatalogPricingCandidates(catalog *modelcatalog.Catalog) map[string][]modelsDevPricingCandidate {
	return extractModelsDevPricingCandidates(convertModelCatalogToModelsDevCatalog(catalog))
}

type modelsDevGitHubTreeResponse struct {
	Tree []modelsDevGitHubTreeItem `json:"tree"`
}

type modelsDevGitHubTreeItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// fetchModelsDevCatalogFromGitHub 从 anomalyco/models.dev 的 TOML 目录读取 catalog。
//
// 该 fallback 只在官网 catalog 不可用时触发。它仍然解析 models/providers TOML，
// 不读取仓库中的其它文件，避免把源码仓库中与模型目录无关的内容引入同步链路。
func fetchModelsDevCatalogFromGitHub(ctx context.Context) (*modelcatalog.Catalog, error) {
	treeURL := getModelsDevGitHubTreeURL()
	buf, err := fetchRawJSON(ctx, treeURL)
	if err != nil {
		return nil, err
	}
	var tree modelsDevGitHubTreeResponse
	if err := common.Unmarshal(buf, &tree); err != nil {
		return nil, err
	}
	files := make(map[string][]byte)
	for _, item := range tree.Tree {
		path := strings.TrimSpace(item.Path)
		if item.Type != "blob" || !isModelsDevCatalogTOMLPath(path) {
			continue
		}
		fileBuf, err := fetchRawJSON(ctx, buildModelsDevGitHubRawURL(path))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		files[path] = fileBuf
	}
	if len(files) == 0 {
		return nil, errors.New("models.dev GitHub fallback has no catalog TOML files")
	}
	return modelcatalog.ParseRepositoryFiles(files)
}

func isModelsDevCatalogTOMLPath(path string) bool {
	parts := strings.Split(strings.TrimSpace(path), "/")
	if len(parts) == 3 && parts[0] == "models" && strings.HasSuffix(parts[2], ".toml") {
		return true
	}
	if len(parts) == 3 && parts[0] == "providers" && parts[2] == "provider.toml" {
		return true
	}
	return len(parts) == 4 && parts[0] == "providers" && parts[2] == "models" && strings.HasSuffix(parts[3], ".toml")
}

func buildModelsDevGitHubRawURL(path string) string {
	parts := strings.Split(strings.TrimSpace(path), "/")
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return getModelsDevGitHubRawBase() + "/" + strings.Join(escaped, "/")
}

// convertModelsDevCatalog 将 models.dev catalog 转成本项目现有同步流程使用的结构。
//
// 转换规则：
// 1. 优先使用 canonical models 生成本地模型，保证供应商归属落到模型原厂；
// 2. canonical key 的前缀视为归属方，例如 openai/gpt-5.5 -> OpenAI；
// 3. 若 canonical models 缺失，则回退到 provider 目录，以兼容旧测试或降级数据；
// 4. deprecated 模型默认仍创建但状态为禁用，便于历史日志、价格表和管理员手动启用排查；
// 5. tags 和 description 只来自公开元数据，不写入价格倍率，价格由独立 ratio sync 链路处理。
func convertModelsDevCatalog(catalog *modelsDevCatalog) ([]upstreamVendor, []upstreamModel) {
	if catalog == nil {
		return nil, nil
	}

	if len(catalog.Models) > 0 {
		return convertModelsDevCatalogFromCanonical(catalog)
	}

	return convertModelsDevCatalogFromProviders(catalog)
}

// extractModelsDevPricingCandidates 从 catalog 的 provider 目录提取每个模型的价格候选。
// canonical models 只描述模型能力和归属方；实际价格在 provider 维度，必须保留
// provider 顺序供管理员配置降级策略。
func extractModelsDevPricingCandidates(catalog *modelsDevCatalog) map[string][]modelsDevPricingCandidate {
	result := make(map[string][]modelsDevPricingCandidate)
	if catalog == nil || len(catalog.Providers) == 0 {
		return result
	}

	providerIDs := make([]string, 0, len(catalog.Providers))
	for providerID := range catalog.Providers {
		providerIDs = append(providerIDs, providerID)
	}
	sortModelsDevProviderIDs(providerIDs)

	for _, providerID := range providerIDs {
		provider := catalog.Providers[providerID]
		providerName := strings.TrimSpace(provider.Name)
		if providerName == "" {
			providerName = providerID
		}
		modelIDs := make([]string, 0, len(provider.Models))
		for modelID := range provider.Models {
			modelIDs = append(modelIDs, modelID)
		}
		sort.Strings(modelIDs)

		for _, modelID := range modelIDs {
			def := provider.Models[modelID]
			modelName := strings.TrimSpace(def.ID)
			if modelName == "" {
				modelName = modelID
			}
			candidate, ok := buildModelsDevPricingCandidate(providerID, providerName, def.Cost)
			if !ok {
				continue
			}
			result[modelName] = append(result[modelName], candidate)
		}
	}
	return result
}

// buildModelsDevPricingCandidate 将 models.dev 价格转换为可应用候选。
// input 是 ratio 模式的基准价格，缺失时无法换算本地倍率；input=0 且 output>0
// 也无法用旧 ratio 结构表达，因此跳过并交给下一 provider 降级。
func buildModelsDevPricingCandidate(providerID, providerName string, cost modelsDevCatalogCost) (modelsDevPricingCandidate, bool) {
	if cost.Input == nil || !isValidSyncCost(*cost.Input) {
		return modelsDevPricingCandidate{}, false
	}
	input := *cost.Input
	output := cloneValidSyncCost(cost.Output)
	if input == 0 && output != nil && *output > 0 {
		return modelsDevPricingCandidate{}, false
	}
	inputAudio := cloneValidSyncCost(cost.InputAudio)
	outputAudio := cloneValidSyncCost(cost.OutputAudio)
	if inputAudio != nil && *inputAudio == 0 && outputAudio != nil && *outputAudio > 0 {
		return modelsDevPricingCandidate{}, false
	}

	return modelsDevPricingCandidate{
		ProviderID:   strings.TrimSpace(providerID),
		ProviderName: strings.TrimSpace(providerName),
		Input:        input,
		Output:       output,
		CacheRead:    cloneValidSyncCost(cost.CacheRead),
		CacheWrite:   cloneValidSyncCost(cost.CacheWrite),
		InputAudio:   inputAudio,
		OutputAudio:  outputAudio,
	}, true
}

func cloneValidSyncCost(value *float64) *float64 {
	if value == nil || !isValidSyncCost(*value) {
		return nil
	}
	cloned := *value
	return &cloned
}

func isValidSyncCost(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

// convertModelsDevCatalogFromCanonical 使用 canonical models 生成同步结果。
//
// canonical models 的 key 一般形如 "openai/gpt-5.5"，其中前缀是模型归属方，
// 后缀是本地实际要保存和请求的模型名。这样即使某个模型同时出现在 Vivgrid
// 之类的服务商目录里，本地供应商也会保持为 OpenAI，而不是被服务商覆盖。
func convertModelsDevCatalogFromCanonical(catalog *modelsDevCatalog) ([]upstreamVendor, []upstreamModel) {
	ownerSet := make(map[string]struct{})
	modelKeys := make([]string, 0, len(catalog.Models))
	for canonicalKey := range catalog.Models {
		modelKeys = append(modelKeys, canonicalKey)
		ownerID, _ := splitModelsDevCanonicalKey(canonicalKey)
		if ownerID != "" {
			ownerSet[ownerID] = struct{}{}
		}
	}
	sort.Slice(modelKeys, func(i, j int) bool {
		ownerI, modelI := splitModelsDevCanonicalKey(modelKeys[i])
		ownerJ, modelJ := splitModelsDevCanonicalKey(modelKeys[j])
		pi := modelsDevProviderPriority(ownerI)
		pj := modelsDevProviderPriority(ownerJ)
		if pi != pj {
			return pi < pj
		}
		if ownerI != ownerJ {
			return ownerI < ownerJ
		}
		return modelI < modelJ
	})

	ownerIDs := make([]string, 0, len(ownerSet))
	for ownerID := range ownerSet {
		ownerIDs = append(ownerIDs, ownerID)
	}
	sortModelsDevProviderIDs(ownerIDs)

	vendors := make([]upstreamVendor, 0, len(ownerIDs))
	for _, ownerID := range ownerIDs {
		provider, providerName := modelsDevCanonicalOwnerProvider(catalog, ownerID)
		vendors = append(vendors, upstreamVendor{
			Description: buildModelsDevVendorDescription(provider),
			Icon:        modelsDevCatalogProviderIcon(ownerID, providerName),
			Name:        providerName,
			Status:      1,
		})
	}

	models := make([]upstreamModel, 0, len(modelKeys))
	seenModels := make(map[string]struct{})
	for _, canonicalKey := range modelKeys {
		def := catalog.Models[canonicalKey]
		ownerID, modelName := splitModelsDevCanonicalKey(canonicalKey)
		if modelName == "" {
			modelName = strings.TrimSpace(def.ID)
		}
		if modelName == "" {
			continue
		}
		if _, exists := seenModels[modelName]; exists {
			continue
		}
		seenModels[modelName] = struct{}{}

		_, providerName := modelsDevCanonicalOwnerProvider(catalog, ownerID)
		models = append(models, upstreamModel{
			Description: buildModelsDevModelDescription(providerName, def),
			Icon:        modelsDevCatalogProviderIcon(ownerID, providerName),
			ModelName:   modelName,
			NameRule:    model.NameRuleExact,
			Status:      modelsDevModelStatus(def.Status),
			Tags:        buildModelsDevModelTags(def),
			VendorName:  providerName,
		})
	}

	return vendors, models
}

// convertModelsDevCatalogFromProviders 是 models.dev canonical 数据缺失时的降级兼容路径。
func convertModelsDevCatalogFromProviders(catalog *modelsDevCatalog) ([]upstreamVendor, []upstreamModel) {
	if len(catalog.Providers) == 0 {
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

// splitModelsDevCanonicalKey 将 canonical key 拆成归属方和模型名。
//
// 例如 openai/gpt-5.5 会拆成 (openai, gpt-5.5)；如果没有斜杠，则归属方为空，
// 模型名直接使用原始 key，方便兼容退化数据。
func splitModelsDevCanonicalKey(key string) (ownerID, modelID string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ""
	}
	if ownerID, modelID, ok := strings.Cut(key, "/"); ok {
		return strings.TrimSpace(ownerID), strings.TrimSpace(modelID)
	}
	return "", key
}

// modelsDevCanonicalOwnerProvider 返回 canonical 模型归属方的 provider 元数据与展示名。
func modelsDevCanonicalOwnerProvider(catalog *modelsDevCatalog, ownerID string) (modelsDevCatalogProvider, string) {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return modelsDevCatalogProvider{}, ""
	}

	provider, ok := catalog.Providers[ownerID]
	if !ok {
		return modelsDevCatalogProvider{ID: ownerID}, ownerID
	}
	provider.ID = strings.TrimSpace(provider.ID)
	if provider.ID == "" {
		provider.ID = ownerID
	}
	provider.Name = strings.TrimSpace(provider.Name)
	if provider.Name == "" {
		provider.Name = ownerID
	}
	return provider, provider.Name
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

	if strings.TrimSpace(providerName) == "" {
		providerName = "models.dev"
	}

	parts := []string{fmt.Sprintf("%s is an AI model from %s.", displayName, providerName)}
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
	manifest := modelcatalog.LoadEmbeddedManifest()
	switch source {
	case syncSourceModelsDev:
		return syncSourceInfo{
			Source:                syncSourceModelsDev,
			CatalogURL:            getModelsDevCatalogURL(),
			CatalogOrigin:         modelcatalog.CatalogOriginModelsDevWeb,
			EmbeddedModelCount:    manifest.ModelCount,
			EmbeddedProviderCount: manifest.ProviderCount,
		}
	case syncSourceConfig:
		return syncSourceInfo{Source: syncSourceConfig}
	default:
		return syncSourceInfo{
			Source:                syncSourceOfficial,
			Locale:                req.Locale,
			CatalogOrigin:         modelcatalog.CatalogOriginNexusTokRepository,
			FallbackName:          manifest.Name,
			FallbackGeneratedAt:   manifest.GeneratedAt,
			CatalogVersion:        manifest.Version,
			EmbeddedModelCount:    manifest.ModelCount,
			EmbeddedProviderCount: manifest.ProviderCount,
		}
	}
}

// parseOptionalBoolQuery 解析可选布尔查询参数。
//
// 仅在调用方显式传参时返回指针；无法识别的值按未传处理，避免调试链接中的拼写
// 错误把同步范围意外收窄或放大。
func parseOptionalBoolQuery(c *gin.Context, key string) *bool {
	raw, ok := c.GetQuery(key)
	if !ok {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return common.GetPointer(true)
	case "0", "false", "no", "off":
		return common.GetPointer(false)
	default:
		return nil
	}
}

// resolveSyncUpstreamOptions 统一手动同步、预览和自动任务的目标范围。
//
// models.dev 是完整公开模型目录，页面手动同步也应默认补齐 catalog 中所有本地缺失
// 模型；否则新模型系列中只有已经出现在能力表里的变体会被创建，导致 Luna/Terra
// 这类同系列模型被遗漏。official 来源沿用历史的“只补能力表缺失项”语义，避免
// 老元数据仓库一次性导入过多模型。create_all 为可选逃生开关，便于脚本显式控制。
func resolveSyncUpstreamOptions(req syncRequest, opts syncUpstreamOptions) syncUpstreamOptions {
	resolved := opts
	if req.CreateAll != nil {
		resolved.CreateAllUpstream = *req.CreateAll
		return resolved
	}
	if normalizeSyncSource(req.Source) == syncSourceModelsDev {
		resolved.CreateAllUpstream = true
	}
	return resolved
}

// fetchSyncUpstreamData 拉取指定来源的供应商和模型数据。
func fetchSyncUpstreamData(ctx context.Context, req syncRequest) ([]upstreamVendor, []upstreamModel, syncSourceInfo, error) {
	sourceInfo := buildSyncSourceInfo(req)
	if err := validateSyncSource(req.Source); err != nil {
		return nil, nil, sourceInfo, err
	}
	switch sourceInfo.Source {
	case syncSourceModelsDev:
		fetchResult, err := fetchModelsDevCatalogWithFallback(ctx, sourceInfo.CatalogURL)
		if err != nil {
			return nil, nil, sourceInfo, err
		}
		applyModelsDevFallbackSourceInfo(&sourceInfo, fetchResult)
		vendors, models := convertModelsDevCatalog(fetchResult.Catalog)
		return vendors, models, sourceInfo, nil
	default:
		catalog, info, err := fetchNexusTokRepositoryCatalog()
		if err != nil {
			return nil, nil, sourceInfo, err
		}
		sourceInfo = mergeSyncSourceInfo(sourceInfo, info)
		vendors, models := convertModelCatalogToUpstream(catalog)
		return vendors, models, sourceInfo, nil
	}
}

// fetchSyncUpstreamDataWithPricing 在普通模型元数据之外返回 models.dev 价格候选。
// 官方仓库暂无统一 provider 价格结构，因此非 models.dev 来源返回空候选。
func fetchSyncUpstreamDataWithPricing(ctx context.Context, req syncRequest) ([]upstreamVendor, []upstreamModel, map[string][]modelsDevPricingCandidate, syncSourceInfo, error) {
	sourceInfo := buildSyncSourceInfo(req)
	if err := validateSyncSource(req.Source); err != nil {
		return nil, nil, nil, sourceInfo, err
	}
	switch sourceInfo.Source {
	case syncSourceModelsDev:
		fetchResult, err := fetchModelsDevCatalogWithFallback(ctx, sourceInfo.CatalogURL)
		if err != nil {
			return nil, nil, nil, sourceInfo, err
		}
		applyModelsDevFallbackSourceInfo(&sourceInfo, fetchResult)
		vendors, models := convertModelsDevCatalog(fetchResult.Catalog)
		pricing := extractModelsDevPricingCandidates(fetchResult.Catalog)
		return vendors, models, pricing, sourceInfo, nil
	default:
		catalog, info, err := fetchNexusTokRepositoryCatalog()
		if err != nil {
			return nil, nil, nil, sourceInfo, err
		}
		sourceInfo = mergeSyncSourceInfo(sourceInfo, info)
		vendors, models := convertModelCatalogToUpstream(catalog)
		pricing := extractCatalogPricingCandidates(catalog)
		return vendors, models, pricing, sourceInfo, nil
	}
}

func mergeSyncSourceInfo(base syncSourceInfo, override syncSourceInfo) syncSourceInfo {
	if override.Source != "" {
		base.Source = override.Source
	}
	if override.CatalogOrigin != "" {
		base.CatalogOrigin = override.CatalogOrigin
	}
	if override.CatalogVersion != "" {
		base.CatalogVersion = override.CatalogVersion
	}
	if override.FallbackName != "" {
		base.FallbackName = override.FallbackName
	}
	if override.FallbackGeneratedAt != "" {
		base.FallbackGeneratedAt = override.FallbackGeneratedAt
	}
	if override.SourceModelCount > 0 {
		base.SourceModelCount = override.SourceModelCount
	}
	if override.SourceProviderCount > 0 {
		base.SourceProviderCount = override.SourceProviderCount
	}
	if override.EmbeddedModelCount > 0 {
		base.EmbeddedModelCount = override.EmbeddedModelCount
	}
	if override.EmbeddedProviderCount > 0 {
		base.EmbeddedProviderCount = override.EmbeddedProviderCount
	}
	return base
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
	if err := validateSyncSource(req.Source); err != nil {
		return nil, err
	}
	opts = resolveSyncUpstreamOptions(req, opts)
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
		// models.dev 同步除了补齐缺失模型，还承担 canonical 供应商纠偏职责。
		// 即使本地没有缺失模型，也要继续拉取上游数据，保证像 gpt-5.5 这类
		// 误绑定到 Vivgrid 的记录能在后续同步中修正回 OpenAI。
		if len(missing) == 0 && sourceInfo.Source != syncSourceModelsDev {
			return emptyResult, nil
		}
	}

	vendors, upstreamModels, pricingCandidates, sourceInfo, err := fetchSyncUpstreamDataWithPricing(ctx, req)
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
		PricingList:   make([]string, 0),
		Source:        sourceInfo,
	}

	// 本地缓存：vendorName -> id
	vendorIDCache := make(map[string]int)

	// models.dev 既要补齐缺失项，也要纠正已存在官方模型的供应商归属。
	// 这里单独扫描一遍已存在的 official 记录，避免它们因为“不是缺失模型”
	// 而被完全跳过。
	if sourceInfo.Source == syncSourceModelsDev {
		if err := syncModelsDevCanonicalVendorMappings(vendorByName, modelByName, vendorIDCache, result); err != nil {
			return nil, err
		}
	}

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
	if shouldApplySyncPricing(req, sourceInfo, pricingCandidates) {
		applyModelsDevPricingPolicy(req.Pricing, sourceInfo, pricingCandidates, result)
	}
	result.CatalogWriteBack = writeBackSyncedCatalog(sourceInfo)
	return result, nil
}

func shouldApplySyncPricing(req syncRequest, sourceInfo syncSourceInfo, pricingCandidates map[string][]modelsDevPricingCandidate) bool {
	if len(pricingCandidates) == 0 {
		return false
	}
	switch sourceInfo.Source {
	case syncSourceModelsDev, syncSourceOfficial:
	default:
		return false
	}
	return req.Pricing.Enabled
}

// applyModelsDevPricingPolicy 按管理员配置的 provider 顺序将上游价格写入模型定价。
// 规则：
// 1. 只处理本地已存在的模型，避免价格孤儿键；
// 2. manual 来源默认最高优先级，除非 overwrite_manual=true；
// 3. provider_order 中先命中的 provider 生效，未配置时使用 models.dev provider 稳定排序；
// 4. 保存为 ratio 模式，保持 relay 热路径和 /api/pricing 结构不变。
func applyModelsDevPricingPolicy(policy syncPricingPolicyRequest, sourceInfo syncSourceInfo, candidates map[string][]modelsDevPricingCandidate, result *syncUpstreamResult) {
	if result == nil || len(candidates) == 0 {
		return
	}
	modelNames := make([]string, 0, len(candidates))
	for modelName := range candidates {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)

	var existing []model.Model
	if err := model.DB.Where("model_name IN ?", modelNames).Find(&existing).Error; err != nil {
		common.SysError("failed to load models for pricing sync: " + err.Error())
		return
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		existingSet[item.ModelName] = struct{}{}
	}

	currentSources := model.GetModelPricingSourceCopy()
	existingOverrides := model.GetModelPricingOverrideModelSet()
	updates := make(map[string]model.ModelPricingUpdateRequest)
	sources := make(map[string]model.ModelPricingSource)
	for _, modelName := range modelNames {
		if _, ok := existingSet[modelName]; !ok {
			result.PricingSkipped++
			continue
		}
		if shouldPreserveLocalPricing(policy, currentSources[modelName], existingOverrides, modelName) {
			result.PricingSkipped++
			continue
		}
		candidate, ok := selectModelsDevPricingCandidate(candidates[modelName], policy.ProviderOrder)
		if !ok {
			result.PricingSkipped++
			continue
		}
		update, ok := buildPricingUpdateFromModelsDevCandidate(candidate)
		if !ok {
			result.PricingSkipped++
			continue
		}
		sourceKind := model.ModelPricingSourceUpstream
		sourceName := syncSourceModelsDev
		if sourceInfo.Source == syncSourceOfficial {
			sourceKind = model.ModelPricingSourceBuiltin
			sourceName = modelcatalog.CatalogOriginNexusTokRepository
		}
		updates[modelName] = update
		sources[modelName] = model.ModelPricingSource{
			Kind:      sourceKind,
			Provider:  candidate.ProviderID,
			Source:    sourceName,
			UpdatedAt: time.Now().Unix(),
		}
	}

	if len(updates) == 0 {
		return
	}
	if err := model.SaveModelPricingConfigBatch(updates, sources); err != nil {
		common.SysError("failed to save models.dev pricing sync: " + err.Error())
		return
	}
	result.PricingUpdated += len(updates)
	for modelName := range updates {
		result.PricingList = append(result.PricingList, modelName)
	}
	sort.Strings(result.PricingList)
}

func writeBackSyncedCatalog(sourceInfo syncSourceInfo) *catalogWriteBackResult {
	if sourceInfo.Source != syncSourceModelsDev {
		return nil
	}
	repoDir := modelcatalog.RepositoryDir()
	if !modelcatalog.WriteBackEnabled() {
		return &catalogWriteBackResult{
			Status:        "skipped",
			Reason:        "MODEL_CATALOG_WRITE_BACK is disabled; synced models were saved to the database only and will not be included in the next embedded catalog build.",
			ModelCount:    sourceInfo.SourceModelCount,
			ProviderCount: sourceInfo.SourceProviderCount,
			RepoDir:       repoDir,
		}
	}
	timeoutSec := common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", 15)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	fetchResult, err := fetchModelsDevCatalogWithFallback(ctx, getModelsDevCatalogURL())
	if err != nil {
		common.SysError("failed to write back models.dev catalog: " + err.Error())
		return &catalogWriteBackResult{
			Status:        "failed",
			Reason:        err.Error(),
			ModelCount:    sourceInfo.SourceModelCount,
			ProviderCount: sourceInfo.SourceProviderCount,
			RepoDir:       repoDir,
		}
	}
	modelCatalog := fetchResult.ModelCatalog
	if modelCatalog == nil {
		modelCatalog = convertModelsDevCatalogToModelCatalog(fetchResult.Catalog)
	}
	if modelCatalog == nil {
		return &catalogWriteBackResult{
			Status:  "failed",
			Reason:  "models.dev catalog is empty after conversion",
			RepoDir: repoDir,
		}
	}
	if err := modelcatalog.WriteBackCatalog(modelCatalog); err != nil {
		common.SysError("failed to write back model catalog repository: " + err.Error())
		return &catalogWriteBackResult{
			Status:        "failed",
			Reason:        err.Error(),
			ModelCount:    len(modelCatalog.Models),
			ProviderCount: len(modelCatalog.Providers),
			GeneratedAt:   modelCatalog.Manifest.GeneratedAt,
			RepoDir:       repoDir,
		}
	}
	manifest := modelCatalog.Manifest
	if manifest.ModelCount == 0 && manifest.ProviderCount == 0 {
		manifest = modelcatalog.BuildManifest(modelCatalog, manifest.GeneratedAt)
	}
	return &catalogWriteBackResult{
		Status:        "success",
		ModelCount:    manifest.ModelCount,
		ProviderCount: manifest.ProviderCount,
		GeneratedAt:   manifest.GeneratedAt,
		RepoDir:       repoDir,
	}
}

func shouldPreserveLocalPricing(policy syncPricingPolicyRequest, source model.ModelPricingSource, existingOverrides map[string]struct{}, modelName string) bool {
	if policy.OverwriteManual {
		return false
	}
	switch strings.TrimSpace(source.Kind) {
	case model.ModelPricingSourceManual:
		return true
	case model.ModelPricingSourceUpstream:
		return false
	case "":
		// 历史版本没有来源元数据，但只要 options 已经有模型级定价覆盖，
		// 就按管理员手工配置保护，避免每日同步上线后静默改价。
		_, ok := existingOverrides[modelName]
		return ok
	default:
		// 未知来源可能来自后续导入器或第三方扩展，默认不让上游自动覆盖。
		return true
	}
}

func selectModelsDevPricingCandidate(candidates []modelsDevPricingCandidate, providerOrder []string) (modelsDevPricingCandidate, bool) {
	if len(candidates) == 0 {
		return modelsDevPricingCandidate{}, false
	}
	for _, provider := range normalizeProviderOrder(providerOrder) {
		for _, candidate := range candidates {
			if providerMatches(candidate, provider) {
				return candidate, true
			}
		}
	}
	return candidates[0], true
}

func normalizeProviderOrder(providerOrder []string) []string {
	seen := make(map[string]struct{}, len(providerOrder))
	result := make([]string, 0, len(providerOrder))
	for _, item := range providerOrder {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}

func providerMatches(candidate modelsDevPricingCandidate, provider string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return false
	}
	return strings.ToLower(strings.TrimSpace(candidate.ProviderID)) == provider ||
		strings.ToLower(strings.TrimSpace(candidate.ProviderName)) == provider
}

func buildPricingUpdateFromModelsDevCandidate(candidate modelsDevPricingCandidate) (model.ModelPricingUpdateRequest, bool) {
	input := candidate.Input
	if !isValidSyncCost(input) {
		return model.ModelPricingUpdateRequest{}, false
	}
	update := model.ModelPricingUpdateRequest{
		BillingMode:          model.ModelPricingModeRatio,
		InputPricePerMillion: &input,
	}
	if candidate.Output != nil {
		output := *candidate.Output
		update.OutputPricePerMillion = &output
	}
	if ratio := relativePricingRatio(candidate.CacheRead, input); ratio != nil {
		update.CacheRatio = ratio
	}
	if ratio := relativePricingRatio(candidate.CacheWrite, input); ratio != nil {
		update.CreateCacheRatio = ratio
	}
	if ratio := relativePricingRatio(candidate.InputAudio, input); ratio != nil {
		update.AudioRatio = ratio
	}
	if candidate.OutputAudio != nil && candidate.InputAudio != nil && *candidate.InputAudio > 0 {
		audioCompletionRatio := roundSyncRatio(*candidate.OutputAudio / *candidate.InputAudio)
		update.AudioCompletionRatio = &audioCompletionRatio
	}
	return update, true
}

func relativePricingRatio(value *float64, base float64) *float64 {
	if value == nil {
		return nil
	}
	var ratio float64
	if base == 0 {
		if *value != 0 {
			return nil
		}
		ratio = 0
	} else {
		ratio = *value / base
	}
	ratio = roundSyncRatio(ratio)
	return &ratio
}

func roundSyncRatio(value float64) float64 {
	return math.Round(value*1e6) / 1e6
}

// syncModelsDevCanonicalVendorMappings 纠正 models.dev 已存在官方模型的供应商归属。
//
// 这个步骤只作用于 sync_official != 0 的记录，避免覆盖管理员明确关闭官方同步的模型。
// 当前仅修正供应商字段，因为 models.dev 的 canonical 归属语义是“原厂归属方”，
// 不是服务商；其它字段仍保持原有同步规则，避免不必要地扩大覆盖面。
func syncModelsDevCanonicalVendorMappings(
	vendorByName map[string]upstreamVendor,
	modelByName map[string]upstreamModel,
	vendorIDCache map[string]int,
	result *syncUpstreamResult,
) error {
	for name, up := range modelByName {
		var local model.Model
		if err := model.DB.Where("model_name = ?", name).First(&local).Error; err != nil {
			continue
		}
		if local.SyncOfficial == 0 {
			continue
		}

		newVendorID := ensureVendorID(up.VendorName, vendorByName, vendorIDCache, &result.CreatedVendors)
		if newVendorID == 0 || local.VendorID == newVendorID {
			continue
		}

		local.VendorID = newVendorID
		if err := local.Update(); err != nil {
			common.SysError("failed to correct models.dev synced model vendor " + name + ": " + err.Error())
			continue
		}

		result.UpdatedModels++
		result.UpdatedList = append(result.UpdatedList, name)
	}
	return nil
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
	req := syncRequest{Locale: locale, Source: source, CreateAll: parseOptionalBoolQuery(c, "create_all")}
	opts := resolveSyncUpstreamOptions(req, syncUpstreamOptions{})
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

	// 3) 缺失且上游存在的模型。models.dev 使用完整 catalog，official 保留旧能力表范围。
	missingList, _ := buildSyncTargetModelNames(modelByName, opts)
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
		if local.Status != chooseSyncModelStatus(up.Status, sourceInfo.Source, local.Status) {
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
