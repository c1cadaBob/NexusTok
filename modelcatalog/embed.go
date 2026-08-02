package modelcatalog

import (
	"embed"
	"errors"

	"github.com/c1cada/NexusTok/common"
)

// embeddedRepositoryFS 打包生成后的模型仓库文件。
//
// 生产容器或 release 二进制只读取这些文件，不向镜像文件系统写回；开发环境需要写回时
// 必须显式开启 MODEL_CATALOG_WRITE_BACK 并指向源码目录。
//
//go:embed repository/catalog.generated.json repository/manifest.json
var embeddedRepositoryFS embed.FS

// LoadEmbeddedCatalog 读取随二进制打包的 NexusTok 模型仓库。
func LoadEmbeddedCatalog() (*Catalog, error) {
	buf, err := embeddedRepositoryFS.ReadFile("repository/" + generatedCatalogFile)
	if err != nil {
		return nil, err
	}
	if len(buf) == 0 {
		return nil, errors.New("embedded NexusTok model catalog is empty")
	}
	var catalog Catalog
	if err := common.Unmarshal(buf, &catalog); err != nil {
		return nil, err
	}
	if len(catalog.Models) == 0 && len(catalog.Providers) == 0 {
		return nil, errors.New("embedded NexusTok model catalog has no models or providers")
	}
	if catalog.Manifest.Name == "" {
		catalog.Manifest = LoadEmbeddedManifest()
	}
	return &catalog, nil
}

// LoadEmbeddedManifest 返回内置仓库 manifest；读取失败时返回空 manifest。
func LoadEmbeddedManifest() CatalogManifest {
	buf, err := embeddedRepositoryFS.ReadFile("repository/" + manifestFile)
	if err != nil {
		return CatalogManifest{}
	}
	var manifest CatalogManifest
	if err := common.Unmarshal(buf, &manifest); err != nil {
		return CatalogManifest{}
	}
	return manifest
}
