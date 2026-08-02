package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/modelcatalog"
)

const (
	modelsDevEmbeddedFallbackName = "nexustok-embedded-model-repository"
)

type modelsDevCatalogFetchResult struct {
	Catalog             *modelsDevCatalog
	ModelCatalog        *modelcatalog.Catalog
	CatalogOrigin       string
	FallbackStage       string
	GitHubRepo          string
	CatalogVersion      string
	FallbackUsed        bool
	FallbackReason      string
	FallbackName        string
	FallbackGeneratedAt string
}

// fetchModelsDevCatalogWithFallback 按官网、GitHub TOML、内置仓库的顺序读取 models.dev catalog。
//
// models.dev 官网在部分服务器网络中可能被重置或超时；此时先尝试官方 GitHub TOML
// 仓库，仍失败才使用 NexusTok 内置仓库。内置仓库是构建时打包的只读兜底，保证模型
// 页面不会因为外网不可用而完全空白。
func fetchModelsDevCatalogWithFallback(ctx context.Context, catalogURL string) (modelsDevCatalogFetchResult, error) {
	fetchResult, err := modelcatalog.FetchModelsDevCatalogWithFallback(ctx, modelcatalog.ModelsDevFetchOptions{
		CatalogURL: catalogURL,
		TreeURL:    getModelsDevGitHubTreeURL(),
		RawBaseURL: getModelsDevGitHubRawBase(),
		Client:     getHTTPClient(),
	})
	if err != nil {
		return modelsDevCatalogFetchResult{}, err
	}
	catalog := fetchResult.Catalog
	if catalog == nil {
		return modelsDevCatalogFetchResult{}, fmt.Errorf("models.dev catalog result is empty")
	}
	return modelsDevCatalogFetchResult{
		Catalog:             convertModelCatalogToModelsDevCatalog(catalog),
		ModelCatalog:        catalog,
		CatalogOrigin:       fetchResult.CatalogOrigin,
		FallbackStage:       fetchResult.FallbackStage,
		GitHubRepo:          fetchResult.GitHubRepo,
		CatalogVersion:      fetchResult.CatalogVersion,
		FallbackUsed:        fetchResult.FallbackUsed,
		FallbackReason:      fetchResult.FallbackReason,
		FallbackName:        coalesce(fetchResult.FallbackName, modelsDevEmbeddedFallbackName),
		FallbackGeneratedAt: fetchResult.FallbackGeneratedAt,
	}, nil
}

func applyModelsDevFallbackSourceInfo(sourceInfo *syncSourceInfo, fetchResult modelsDevCatalogFetchResult) {
	if sourceInfo == nil {
		return
	}
	sourceInfo.CatalogOrigin = fetchResult.CatalogOrigin
	sourceInfo.CatalogVersion = strings.TrimSpace(fetchResult.CatalogVersion)
	sourceInfo.FallbackStage = strings.TrimSpace(fetchResult.FallbackStage)
	sourceInfo.GitHubRepo = strings.TrimSpace(fetchResult.GitHubRepo)
	if fetchResult.Catalog != nil {
		sourceInfo.SourceModelCount = len(fetchResult.Catalog.Models)
		sourceInfo.SourceProviderCount = len(fetchResult.Catalog.Providers)
	}
	if !fetchResult.FallbackUsed {
		return
	}
	sourceInfo.FallbackUsed = true
	sourceInfo.FallbackReason = strings.TrimSpace(fetchResult.FallbackReason)
	sourceInfo.FallbackName = fetchResult.FallbackName
	sourceInfo.FallbackGeneratedAt = fetchResult.FallbackGeneratedAt
}
