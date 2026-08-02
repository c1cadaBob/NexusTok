package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	_ "embed"

	"github.com/c1cada/NexusTok/common"
)

const (
	modelsDevFallbackCatalogName        = "nexustok-embedded-models-dev-fallback"
	modelsDevFallbackCatalogGeneratedAt = "2026-08-02"
)

// embeddedModelsDevFallbackCatalog 是随二进制和 Docker 镜像一起打包的离线模型目录。
//
// models.dev 在部分服务器网络中可能被重置或超时；同步源模型不能因此完全空白。
// 该 catalog 只作为网络失败时的安全兜底，在线同步成功时仍以 models.dev 最新数据为准。
//
//go:embed data/model-catalog/models-dev-fallback.json
var embeddedModelsDevFallbackCatalog []byte

type modelsDevCatalogFetchResult struct {
	Catalog             *modelsDevCatalog
	FallbackUsed        bool
	FallbackReason      string
	FallbackName        string
	FallbackGeneratedAt string
}

// fetchModelsDevCatalogWithFallback 先请求在线 models.dev catalog，失败后使用内置兜底。
//
// 内置兜底必须通过同一套 models.dev 转换和价格同步逻辑，避免“离线目录”和“在线目录”
// 产生两套行为。若在线和内置都失败，则返回原始在线错误并附带内置解析错误，方便排障。
func fetchModelsDevCatalogWithFallback(ctx context.Context, catalogURL string) (modelsDevCatalogFetchResult, error) {
	catalog, err := fetchModelsDevCatalog(ctx, catalogURL)
	if err == nil {
		return modelsDevCatalogFetchResult{Catalog: catalog}, nil
	}

	fallback, fallbackErr := loadEmbeddedModelsDevFallbackCatalog()
	if fallbackErr != nil {
		return modelsDevCatalogFetchResult{}, fmt.Errorf("%w; embedded fallback catalog failed: %v", err, fallbackErr)
	}
	return modelsDevCatalogFetchResult{
		Catalog:             fallback,
		FallbackUsed:        true,
		FallbackReason:      err.Error(),
		FallbackName:        modelsDevFallbackCatalogName,
		FallbackGeneratedAt: modelsDevFallbackCatalogGeneratedAt,
	}, nil
}

func loadEmbeddedModelsDevFallbackCatalog() (*modelsDevCatalog, error) {
	if len(embeddedModelsDevFallbackCatalog) == 0 {
		return nil, errors.New("embedded models.dev fallback catalog is empty")
	}
	var catalog modelsDevCatalog
	if err := common.Unmarshal(embeddedModelsDevFallbackCatalog, &catalog); err != nil {
		return nil, err
	}
	if len(catalog.Models) == 0 && len(catalog.Providers) == 0 {
		return nil, errors.New("embedded models.dev fallback catalog has no models or providers")
	}
	return &catalog, nil
}

func applyModelsDevFallbackSourceInfo(sourceInfo *syncSourceInfo, fetchResult modelsDevCatalogFetchResult) {
	if sourceInfo == nil || !fetchResult.FallbackUsed {
		return
	}
	sourceInfo.FallbackUsed = true
	sourceInfo.FallbackReason = strings.TrimSpace(fetchResult.FallbackReason)
	sourceInfo.FallbackName = fetchResult.FallbackName
	sourceInfo.FallbackGeneratedAt = fetchResult.FallbackGeneratedAt
}
