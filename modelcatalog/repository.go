package modelcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/pelletier/go-toml/v2"
)

const (
	generatedCatalogFile = "catalog.generated.json"
	manifestFile         = "manifest.json"
	repositoryName       = "nexustok-model-repository"
)

// LoadRepository 从项目内模型仓库源码目录读取 TOML 文件。
//
// 该函数只读取 modelcatalog/repository 下约定的 models/providers 子目录，
// 不扫描其它运行时目录，避免误把开发机上的数据库、日志或密钥文件纳入构建输入。
func LoadRepository(dir string) (*Catalog, error) {
	dir = normalizeRepositoryDir(dir)
	files := make(map[string][]byte)
	for _, root := range []string{"models", "providers"} {
		walkRoot := filepath.Join(dir, root)
		if _, err := os.Stat(walkRoot); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		err := filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
				return nil
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			buf, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[filepath.ToSlash(rel)] = buf
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return ParseRepositoryFiles(files)
}

// ParseRepositoryFiles 从路径到 TOML 内容的映射中解析模型仓库。
//
// 该入口用于本地仓库读取，也用于 models.dev GitHub fallback：HTTP 拉取到的
// TOML 文件会先进入同一套 parser，再转换为标准 Catalog，保证三段来源行为一致。
func ParseRepositoryFiles(files map[string][]byte) (*Catalog, error) {
	catalog := &Catalog{
		Models:    make(map[string]CatalogModel),
		Providers: make(map[string]CatalogProvider),
	}
	if len(files) == 0 {
		return nil, errors.New("model catalog repository has no TOML files")
	}

	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, filepath.ToSlash(strings.TrimSpace(path)))
	}
	sort.Strings(paths)

	for _, path := range paths {
		parts := strings.Split(path, "/")
		switch {
		case len(parts) == 3 && parts[0] == "models" && strings.HasSuffix(parts[2], ".toml"):
			modelDef, err := parseCatalogModelTOML(files[path])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			owner := strings.TrimSpace(parts[1])
			modelID := strings.TrimSuffix(parts[2], ".toml")
			modelDef = normalizeCatalogModel(modelDef, modelID)
			key := canonicalModelKey(owner, modelDef.ID)
			if key != "" {
				catalog.Models[key] = modelDef
			}
		case len(parts) == 3 && parts[0] == "providers" && parts[2] == "provider.toml":
			providerID := strings.TrimSpace(parts[1])
			provider, err := parseCatalogProviderTOML(files[path])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			provider = normalizeCatalogProvider(provider, providerID)
			existing := catalog.Providers[provider.ID]
			provider.Models = existing.Models
			if provider.Models == nil {
				provider.Models = make(map[string]CatalogModel)
			}
			catalog.Providers[provider.ID] = provider
		case len(parts) == 4 && parts[0] == "providers" && parts[2] == "models" && strings.HasSuffix(parts[3], ".toml"):
			providerID := strings.TrimSpace(parts[1])
			modelID := strings.TrimSuffix(parts[3], ".toml")
			modelDef, err := parseCatalogModelTOML(files[path])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			modelDef = normalizeCatalogModel(modelDef, modelID)
			provider := catalog.Providers[providerID]
			provider = normalizeCatalogProvider(provider, providerID)
			if provider.Models == nil {
				provider.Models = make(map[string]CatalogModel)
			}
			provider.Models[modelDef.ID] = modelDef
			catalog.Providers[provider.ID] = provider
		}
	}

	if len(catalog.Models) == 0 && len(catalog.Providers) == 0 {
		return nil, errors.New("model catalog repository has no models or providers")
	}
	catalog.Manifest = BuildManifest(catalog, "")
	return catalog, nil
}

func parseCatalogModelTOML(data []byte) (CatalogModel, error) {
	var modelDef CatalogModel
	if err := toml.Unmarshal(data, &modelDef); err != nil {
		return CatalogModel{}, err
	}
	return modelDef, nil
}

func parseCatalogProviderTOML(data []byte) (CatalogProvider, error) {
	var provider CatalogProvider
	if err := toml.Unmarshal(data, &provider); err != nil {
		return CatalogProvider{}, err
	}
	return provider, nil
}

// WriteCatalogToRepository 将标准 Catalog 写回 TOML 仓库并重新生成 manifest/catalog。
//
// 写入使用临时文件 + rename，确保生成过程被中断时不会留下半截 TOML。该函数只接收
// 已经脱敏后的 Catalog；调用方必须保证不要把运行时敏感字段放进 Catalog。
func WriteCatalogToRepository(dir string, catalog *Catalog) error {
	if catalog == nil {
		return errors.New("model catalog is nil")
	}
	dir = normalizeRepositoryDir(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := cleanRepositorySourceDirs(dir); err != nil {
		return err
	}

	if err := writeCanonicalModels(dir, catalog); err != nil {
		return err
	}
	if err := writeProviders(dir, catalog); err != nil {
		return err
	}
	catalog.Manifest = BuildManifest(catalog, readExistingGeneratedAt(dir))
	if err := WriteGeneratedFiles(dir, catalog); err != nil {
		return err
	}
	return nil
}

func writeCanonicalModels(dir string, catalog *Catalog) error {
	keys := sortedModelKeys(catalog.Models)
	for _, key := range keys {
		owner, modelID := splitCanonicalKey(key)
		if owner == "" || modelID == "" {
			continue
		}
		modelDef := normalizeCatalogModel(catalog.Models[key], modelID)
		path := filepath.Join(dir, "models", safePathSegment(owner), safePathSegment(modelDef.ID)+".toml")
		if err := writeTOMLFile(path, modelDef); err != nil {
			return err
		}
	}
	return nil
}

func writeProviders(dir string, catalog *Catalog) error {
	providerIDs := sortedProviderKeys(catalog.Providers)
	for _, providerID := range providerIDs {
		provider := normalizeCatalogProvider(catalog.Providers[providerID], providerID)
		providerPath := filepath.Join(dir, "providers", safePathSegment(provider.ID), "provider.toml")
		providerHeader := provider
		providerHeader.Models = nil
		if err := writeTOMLFile(providerPath, providerHeader); err != nil {
			return err
		}
		modelIDs := sortedModelKeys(provider.Models)
		for _, modelID := range modelIDs {
			modelDef := normalizeCatalogModel(provider.Models[modelID], modelID)
			path := filepath.Join(dir, "providers", safePathSegment(provider.ID), "models", safePathSegment(modelDef.ID)+".toml")
			if err := writeTOMLFile(path, modelDef); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeTOMLFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf, err := toml.Marshal(value)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(buf, '\n'))
}

// WriteGeneratedFiles 写入 catalog.generated.json 和 manifest.json。
func WriteGeneratedFiles(dir string, catalog *Catalog) error {
	if catalog == nil {
		return errors.New("model catalog is nil")
	}
	dir = normalizeRepositoryDir(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	catalog.Manifest = BuildManifest(catalog, readExistingGeneratedAt(dir))
	catalogJSON, err := common.Marshal(catalog)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(dir, generatedCatalogFile), append(catalogJSON, '\n')); err != nil {
		return err
	}
	manifestJSON, err := common.Marshal(catalog.Manifest)
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, manifestFile), append(manifestJSON, '\n'))
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// cleanRepositorySourceDirs 清理源码 TOML 目录，确保重新生成时不会保留旧模型文件。
//
// models.dev 中存在带冒号的模型 ID，旧版本会直接写成文件名；Windows runner 无法
// checkout 这类路径。写回前先清理 models/providers，再用安全文件名重建目录，可以
// 避免发布流水线继续被旧非法文件卡住。
func cleanRepositorySourceDirs(dir string) error {
	for _, root := range []string{"models", "providers"} {
		if err := os.RemoveAll(filepath.Join(dir, root)); err != nil {
			return err
		}
	}
	return nil
}

// BuildManifest 为当前 catalog 构造稳定 manifest。
func BuildManifest(catalog *Catalog, generatedAt string) CatalogManifest {
	if catalog == nil {
		return CatalogManifest{Name: repositoryName}
	}
	catalogCopy := *catalog
	catalogCopy.Manifest = CatalogManifest{}
	payload, err := common.Marshal(catalogCopy)
	if err != nil {
		payload = []byte(fmt.Sprintf("%#v", catalogCopy))
	}
	sum := sha256.Sum256(payload)
	hash := "sha256:" + hex.EncodeToString(sum[:])
	generatedAt = strings.TrimSpace(generatedAt)
	if generatedAt == "" {
		generatedAt = time.Now().UTC().Format("2006-01-02")
	}
	return CatalogManifest{
		Name:          repositoryName,
		Version:       hash[:19],
		Hash:          hash,
		GeneratedAt:   generatedAt,
		ModelCount:    len(catalog.Models),
		ProviderCount: len(catalog.Providers),
	}
}

func readExistingGeneratedAt(dir string) string {
	buf, err := os.ReadFile(filepath.Join(normalizeRepositoryDir(dir), manifestFile))
	if err != nil {
		return ""
	}
	var manifest CatalogManifest
	if err := common.Unmarshal(buf, &manifest); err != nil {
		return ""
	}
	return strings.TrimSpace(manifest.GeneratedAt)
}

func normalizeRepositoryDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return RepositoryDefaultDir
	}
	return dir
}

func normalizeCatalogModel(modelDef CatalogModel, fallbackID string) CatalogModel {
	modelDef.ID = strings.TrimSpace(modelDef.ID)
	if modelDef.ID == "" {
		modelDef.ID = strings.TrimSpace(fallbackID)
	}
	modelDef.Name = strings.TrimSpace(modelDef.Name)
	if modelDef.Status == "" {
		modelDef.Status = "active"
	}
	modelDef.Tags = uniqueStrings(modelDef.Tags)
	modelDef.Endpoints = uniqueStrings(modelDef.Endpoints)
	modelDef.Modalities.Input = uniqueStrings(modelDef.Modalities.Input)
	modelDef.Modalities.Output = uniqueStrings(modelDef.Modalities.Output)
	return modelDef
}

func normalizeCatalogProvider(provider CatalogProvider, fallbackID string) CatalogProvider {
	provider.ID = strings.TrimSpace(provider.ID)
	if provider.ID == "" {
		provider.ID = strings.TrimSpace(fallbackID)
	}
	provider.Name = strings.TrimSpace(provider.Name)
	if provider.Name == "" {
		provider.Name = provider.ID
	}
	if provider.Status == "" {
		provider.Status = "active"
	}
	if provider.Models == nil {
		provider.Models = make(map[string]CatalogModel)
	}
	return provider
}

func sortedModelKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedProviderKeys(items map[string]CatalogProvider) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func canonicalModelKey(owner, modelID string) string {
	owner = strings.TrimSpace(owner)
	modelID = strings.TrimSpace(modelID)
	if owner == "" {
		return modelID
	}
	if modelID == "" {
		return ""
	}
	return owner + "/" + modelID
}

func splitCanonicalKey(key string) (string, string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ""
	}
	if owner, modelID, ok := strings.Cut(key, "/"); ok {
		return strings.TrimSpace(owner), strings.TrimSpace(modelID)
	}
	return "", key
}

// safePathSegment 将模型或 provider ID 转成跨平台安全的单级路径名。
//
// TOML 文件内部仍保留原始 ID，因此这里可以只处理文件系统兼容性。只要发生替换、
// 裁剪或命中 Windows 保留名，就追加短 hash，避免 gpt:a 与 gpt-a 这类名称碰撞。
func safePathSegment(value string) string {
	original := strings.TrimSpace(value)
	if original == "" {
		return "unknown"
	}
	var builder strings.Builder
	changed := false
	for _, item := range original {
		if item < 32 || strings.ContainsRune(`<>:"/\|?*`, item) {
			builder.WriteByte('-')
			changed = true
			continue
		}
		builder.WriteRune(item)
	}
	safe := strings.Trim(builder.String(), " .")
	if safe == "" {
		safe = "unknown"
		changed = true
	}
	if isWindowsReservedPathSegment(safe) {
		safe = "_" + safe
		changed = true
	}
	if changed || safe != original {
		return safe + "--" + shortPathSegmentHash(original)
	}
	return safe
}

// isWindowsReservedPathSegment 判断路径段是否命中 Windows 保留设备名。
func isWindowsReservedPathSegment(value string) bool {
	base := strings.ToLower(strings.TrimSpace(value))
	base, _, _ = strings.Cut(base, ".")
	switch base {
	case "con", "prn", "aux", "nul":
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) {
		return base[3] >= '1' && base[3] <= '9'
	}
	return false
}

// shortPathSegmentHash 为改写后的文件名提供稳定短后缀，降低名称碰撞概率。
func shortPathSegmentHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}
