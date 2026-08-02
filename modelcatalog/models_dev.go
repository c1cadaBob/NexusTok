package modelcatalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
)

const (
	// ModelsDevDefaultCatalogURL 是 models.dev 官网 catalog 的默认入口。
	ModelsDevDefaultCatalogURL = "https://models.dev/catalog.json"
	// ModelsDevGitHubRepo 是 models.dev 官方 GitHub 仓库标识。
	ModelsDevGitHubRepo = "anomalyco/models.dev"
	// ModelsDevGitHubDefaultTreeURL 是 models.dev dev 分支递归 tree API。
	ModelsDevGitHubDefaultTreeURL = "https://api.github.com/repos/anomalyco/models.dev/git/trees/dev?recursive=1"
	// ModelsDevGitHubDefaultRawBase 是 models.dev dev 分支 raw 文件根路径。
	ModelsDevGitHubDefaultRawBase = "https://raw.githubusercontent.com/anomalyco/models.dev/dev"

	defaultModelsDevFetchTimeout = 20 * time.Second
	defaultModelsDevMaxBytes     = int64(25 << 20)
)

// ModelsDevFetchOptions 控制 models.dev 同步源拉取行为。
//
// 这些选项只影响公开模型目录文件的读取，不会触碰数据库或运行时配置。调用方可以在
// 测试中注入本地 URL，也可以在 CLI 中沿用默认公网入口。
type ModelsDevFetchOptions struct {
	CatalogURL string
	TreeURL    string
	RawBaseURL string
	Timeout    time.Duration
	MaxBytes   int64
	Client     *http.Client
}

// ModelsDevFetchResult 描述一次公开模型目录拉取的实际来源。
type ModelsDevFetchResult struct {
	Catalog             *Catalog
	CatalogOrigin       string
	FallbackStage       string
	GitHubRepo          string
	CatalogVersion      string
	FallbackUsed        bool
	FallbackReason      string
	FallbackName        string
	FallbackGeneratedAt string
}

type modelsDevGitHubTreeResponse struct {
	Tree []modelsDevGitHubTreeItem `json:"tree"`
}

type modelsDevGitHubTreeItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// FetchModelsDevCatalogWithFallback 按官网、GitHub TOML、内置仓库三段读取 models.dev。
//
// 官网 catalog 是最新来源；GitHub fallback 用同一套 TOML parser 解析官方仓库目录；
// 两者都不可用时才回退到 NexusTok 构建时内置仓库，保证生产服务器在外网异常时仍能
// 同步到一份可审查、随镜像发布的模型目录。
func FetchModelsDevCatalogWithFallback(ctx context.Context, opts ModelsDevFetchOptions) (ModelsDevFetchResult, error) {
	opts = normalizeModelsDevFetchOptions(opts)
	catalog, err := FetchModelsDevCatalog(ctx, opts)
	if err == nil {
		return ModelsDevFetchResult{
			Catalog:        catalog,
			CatalogOrigin:  CatalogOriginModelsDevWeb,
			CatalogVersion: catalog.Manifest.Version,
		}, nil
	}

	githubCatalog, githubErr := FetchModelsDevCatalogFromGitHub(ctx, opts)
	if githubErr == nil {
		return ModelsDevFetchResult{
			Catalog:        githubCatalog,
			CatalogOrigin:  CatalogOriginModelsDevGitHub,
			FallbackUsed:   true,
			FallbackStage:  FallbackStageGitHub,
			FallbackReason: err.Error(),
			FallbackName:   ModelsDevGitHubRepo,
			GitHubRepo:     ModelsDevGitHubRepo,
			CatalogVersion: githubCatalog.Manifest.Version,
		}, nil
	}

	embedded, embeddedErr := LoadEmbeddedCatalog()
	if embeddedErr != nil {
		return ModelsDevFetchResult{}, fmt.Errorf("%w; GitHub fallback failed: %v; embedded fallback failed: %v", err, githubErr, embeddedErr)
	}
	manifest := embedded.Manifest
	return ModelsDevFetchResult{
		Catalog:             embedded,
		CatalogOrigin:       CatalogOriginNexusTokEmbedded,
		FallbackUsed:        true,
		FallbackStage:       FallbackStageEmbedded,
		FallbackReason:      fmt.Sprintf("%v; GitHub fallback failed: %v", err, githubErr),
		FallbackName:        firstNonEmpty(manifest.Name, "nexustok-embedded-model-repository"),
		FallbackGeneratedAt: manifest.GeneratedAt,
		CatalogVersion:      manifest.Version,
	}, nil
}

// FetchModelsDevCatalog 从官网 catalog.json 读取公开模型目录。
func FetchModelsDevCatalog(ctx context.Context, opts ModelsDevFetchOptions) (*Catalog, error) {
	opts = normalizeModelsDevFetchOptions(opts)
	buf, err := fetchModelsDevBytes(ctx, opts.Client, opts.CatalogURL, opts.MaxBytes)
	if err != nil {
		return nil, err
	}
	var catalog Catalog
	if err := commonUnmarshal(buf, &catalog); err != nil {
		return nil, err
	}
	if len(catalog.Models) == 0 && len(catalog.Providers) == 0 {
		return nil, errors.New("models.dev catalog is empty")
	}
	catalog = normalizeCatalogForSource(&catalog, CatalogOriginModelsDevWeb)
	catalog.Manifest = BuildManifest(&catalog, "")
	return &catalog, nil
}

// FetchModelsDevCatalogFromGitHub 从 anomalyco/models.dev 的 TOML 目录读取公开模型目录。
//
// 只接受 models/ 和 providers/ 下的 TOML 文件，避免把官方仓库中与模型目录无关的
// CI、脚本或文档文件带入 NexusTok 内置仓库。
func FetchModelsDevCatalogFromGitHub(ctx context.Context, opts ModelsDevFetchOptions) (*Catalog, error) {
	opts = normalizeModelsDevFetchOptions(opts)
	buf, err := fetchModelsDevBytes(ctx, opts.Client, opts.TreeURL, opts.MaxBytes)
	if err != nil {
		return nil, err
	}
	var tree modelsDevGitHubTreeResponse
	if err := commonUnmarshal(buf, &tree); err != nil {
		return nil, err
	}
	files := make(map[string][]byte)
	for _, item := range tree.Tree {
		path := strings.TrimSpace(item.Path)
		if item.Type != "blob" || !IsModelsDevCatalogTOMLPath(path) {
			continue
		}
		fileBuf, err := fetchModelsDevBytes(ctx, opts.Client, BuildModelsDevGitHubRawURL(opts.RawBaseURL, path), opts.MaxBytes)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		files[path] = fileBuf
	}
	if len(files) == 0 {
		return nil, errors.New("models.dev GitHub fallback has no catalog TOML files")
	}
	catalog, err := ParseRepositoryFiles(files)
	if err != nil {
		return nil, err
	}
	normalized := normalizeCatalogForSource(catalog, CatalogOriginModelsDevGitHub)
	normalized.Manifest = BuildManifest(&normalized, "")
	return &normalized, nil
}

// IsModelsDevCatalogTOMLPath 判断 GitHub tree 中的路径是否属于公开模型目录。
func IsModelsDevCatalogTOMLPath(path string) bool {
	parts := strings.Split(strings.TrimSpace(path), "/")
	if len(parts) == 3 && parts[0] == "models" && strings.HasSuffix(parts[2], ".toml") {
		return true
	}
	if len(parts) == 3 && parts[0] == "providers" && parts[2] == "provider.toml" {
		return true
	}
	return len(parts) == 4 && parts[0] == "providers" && parts[2] == "models" && strings.HasSuffix(parts[3], ".toml")
}

// BuildModelsDevGitHubRawURL 根据 raw base 和 tree path 构造安全的 raw URL。
func BuildModelsDevGitHubRawURL(rawBase string, path string) string {
	rawBase = strings.TrimRight(strings.TrimSpace(rawBase), "/")
	if rawBase == "" {
		rawBase = ModelsDevGitHubDefaultRawBase
	}
	parts := strings.Split(strings.TrimSpace(path), "/")
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return rawBase + "/" + strings.Join(escaped, "/")
}

// NewModelsDevHTTPClient 创建带超时和连接池的公开目录 HTTP client。
func NewModelsDevHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultModelsDevFetchTimeout
	}
	dialer := &net.Dialer{Timeout: timeout}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			MaxIdleConns:          50,
			IdleConnTimeout:       60 * time.Second,
			TLSHandshakeTimeout:   timeout,
			ResponseHeaderTimeout: timeout,
			ExpectContinueTimeout: time.Second,
		},
	}
}

func normalizeModelsDevFetchOptions(opts ModelsDevFetchOptions) ModelsDevFetchOptions {
	if strings.TrimSpace(opts.CatalogURL) == "" {
		opts.CatalogURL = ModelsDevDefaultCatalogURL
	}
	if strings.TrimSpace(opts.TreeURL) == "" {
		opts.TreeURL = ModelsDevGitHubDefaultTreeURL
	}
	if strings.TrimSpace(opts.RawBaseURL) == "" {
		opts.RawBaseURL = ModelsDevGitHubDefaultRawBase
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultModelsDevFetchTimeout
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = defaultModelsDevMaxBytes
	}
	if opts.Client == nil {
		opts.Client = NewModelsDevHTTPClient(opts.Timeout)
	}
	return opts
}

func fetchModelsDevBytes(ctx context.Context, client *http.Client, targetURL string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	limited := io.LimitReader(resp.Body, maxBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > maxBytes {
		return nil, fmt.Errorf("models.dev response exceeds %d bytes", maxBytes)
	}
	return buf, nil
}

func normalizeCatalogForSource(catalog *Catalog, origin string) Catalog {
	if catalog == nil {
		return Catalog{Models: map[string]CatalogModel{}, Providers: map[string]CatalogProvider{}}
	}
	result := Catalog{
		Models:    make(map[string]CatalogModel, len(catalog.Models)),
		Providers: make(map[string]CatalogProvider, len(catalog.Providers)),
	}
	for key, item := range catalog.Models {
		owner, modelID := splitCanonicalKey(key)
		if modelID == "" {
			modelID = strings.TrimSpace(item.ID)
		}
		if owner == "" {
			owner = ownerFromProviderModels(modelID, catalog.Providers)
		}
		if owner == "" || modelID == "" {
			continue
		}
		item = normalizeCatalogModel(item, modelID)
		if strings.Contains(item.ID, "/") {
			_, id := splitCanonicalKey(item.ID)
			item.ID = id
		}
		item.Source.Origin = origin
		result.Models[canonicalModelKey(owner, item.ID)] = item
	}
	for providerID, provider := range catalog.Providers {
		provider = normalizeCatalogProvider(provider, providerID)
		provider.ID = firstNonEmpty(provider.ID, providerID)
		if provider.Icon == "" {
			provider.Icon = providerIcon(provider.ID, provider.Name)
		}
		if provider.Models == nil {
			provider.Models = map[string]CatalogModel{}
		}
		modelIDs := sortedCatalogModelMapKeys(provider.Models)
		normalizedModels := make(map[string]CatalogModel, len(modelIDs))
		for _, modelID := range modelIDs {
			item := normalizeCatalogModel(provider.Models[modelID], modelID)
			if strings.Contains(item.ID, "/") {
				_, id := splitCanonicalKey(item.ID)
				item.ID = id
			}
			item.Source.Origin = origin
			item.Source.Provider = provider.ID
			normalizedModels[item.ID] = item
		}
		provider.Models = normalizedModels
		result.Providers[provider.ID] = provider
	}
	result.Manifest = BuildManifest(&result, "")
	return result
}

func ownerFromProviderModels(modelID string, providers map[string]CatalogProvider) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	providerIDs := make([]string, 0, len(providers))
	for providerID := range providers {
		providerIDs = append(providerIDs, providerID)
	}
	sort.Strings(providerIDs)
	for _, providerID := range providerIDs {
		if _, ok := providers[providerID].Models[modelID]; ok {
			return providerID
		}
	}
	return ""
}

func sortedCatalogModelMapKeys(items map[string]CatalogModel) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

// commonUnmarshal 集中经过 common.Unmarshal，避免业务代码直接调用 encoding/json。
func commonUnmarshal(data []byte, v any) error {
	return common.Unmarshal(data, v)
}
