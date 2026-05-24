// amp - secret.go
// Amp API 密钥管理模块。
// 该模块提供多种密钥获取策略，支持配置优先级和缓存机制：
//   - MultiSourceSecret: 多源密钥（配置 > 环境变量 > 文件），带 TTL 缓存
//   - StaticSecretSource: 固定密钥（用于测试）
//   - MappedSecretSource: 客户端到上游密钥映射，支持每个客户端使用不同的上游密钥
package amp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	log "github.com/sirupsen/logrus"
)

// SecretSource 定义了 Amp API 密钥获取的接口。
// 所有密钥源实现都必须实现此接口。
type SecretSource interface {
	// Get 获取 Amp API 密钥，支持上下文取消。
	Get(ctx context.Context) (string, error)
}

// cachedSecret 表示带过期时间的缓存密钥。
type cachedSecret struct {
	value     string    // 缓存的密钥值
	expiresAt time.Time // 缓存过期时间
}

// MultiSourceSecret 实现基于优先级的密钥查找，优先级从高到低为：
//  1. 显式配置值（最高优先级）
//  2. 环境变量 AMP_API_KEY
//  3. 文件密钥（最低优先级，带缓存）
type MultiSourceSecret struct {
	explicitKey string        // 配置文件中显式指定的密钥
	envKey      string        // 环境变量名称
	filePath    string        // 密钥文件路径
	cacheTTL    time.Duration // 缓存存活时间

	mu    sync.RWMutex  // 保护缓存的读写锁
	cache *cachedSecret // 当前缓存的密钥
}

// NewMultiSourceSecret 创建一个多源密钥实例，默认从 ~/.local/share/amp/secrets.json 读取文件密钥。
// cacheTTL 为 0 时默认使用 5 分钟缓存。
func NewMultiSourceSecret(explicitKey string, cacheTTL time.Duration) *MultiSourceSecret {
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute // Default 5 minute cache
	}

	home, _ := os.UserHomeDir()
	filePath := filepath.Join(home, ".local", "share", "amp", "secrets.json")

	return &MultiSourceSecret{
		explicitKey: strings.TrimSpace(explicitKey),
		envKey:      "AMP_API_KEY",
		filePath:    filePath,
		cacheTTL:    cacheTTL,
	}
}

// NewMultiSourceSecretWithPath 创建一个多源密钥实例，使用自定义文件路径（主要用于测试）。
func NewMultiSourceSecretWithPath(explicitKey string, filePath string, cacheTTL time.Duration) *MultiSourceSecret {
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute
	}

	return &MultiSourceSecret{
		explicitKey: strings.TrimSpace(explicitKey),
		envKey:      "AMP_API_KEY",
		filePath:    filePath,
		cacheTTL:    cacheTTL,
	}
}

// Get 按优先级获取 Amp API 密钥：配置 > 环境变量 > 文件。
// 文件密钥结果会被缓存 cacheTTL 时长，避免频繁读取文件系统。
func (s *MultiSourceSecret) Get(ctx context.Context) (string, error) {
	// Precedence 1: Explicit config key (highest priority, no caching needed)
	if s.explicitKey != "" {
		return s.explicitKey, nil
	}

	// Precedence 2: Environment variable
	if envValue := strings.TrimSpace(os.Getenv(s.envKey)); envValue != "" {
		return envValue, nil
	}

	// Precedence 3: File-based secret (lowest priority, cached)
	// Check cache first
	s.mu.RLock()
	if s.cache != nil && time.Now().Before(s.cache.expiresAt) {
		value := s.cache.value
		s.mu.RUnlock()
		return value, nil
	}
	s.mu.RUnlock()

	// Cache miss or expired - read from file
	key, err := s.readFromFile()
	if err != nil {
		// Cache empty result to avoid repeated file reads on missing files
		s.updateCache("")
		return "", err
	}

	// Cache the result
	s.updateCache(key)
	return key, nil
}

// readFromFile 从密钥文件中读取 Amp API 密钥。
// 文件格式为 JSON，包含 "apiKey@https://ampcode.com/" 键。
// 文件不存在不视为错误，返回空字符串。
func (s *MultiSourceSecret) readFromFile() (string, error) {
	content, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Missing file is not an error, just no key available
		}
		return "", fmt.Errorf("failed to read amp secrets from %s: %w", s.filePath, err)
	}

	var secrets map[string]string
	if err := json.Unmarshal(content, &secrets); err != nil {
		return "", fmt.Errorf("failed to parse amp secrets from %s: %w", s.filePath, err)
	}

	key := strings.TrimSpace(secrets["apiKey@https://ampcode.com/"])
	return key, nil
}

// updateCache 更新缓存中的密钥值和过期时间。
func (s *MultiSourceSecret) updateCache(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = &cachedSecret{
		value:     value,
		expiresAt: time.Now().Add(s.cacheTTL),
	}
}

// InvalidateCache 清除缓存，强制下次 Get 时重新读取密钥。
func (s *MultiSourceSecret) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = nil
}

// UpdateExplicitKey 刷新配置提供的显式密钥并清除缓存。
func (s *MultiSourceSecret) UpdateExplicitKey(key string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.explicitKey = strings.TrimSpace(key)
	s.cache = nil
	s.mu.Unlock()
}

// StaticSecretSource 返回固定 API 密钥的密钥源，主要用于测试场景。
type StaticSecretSource struct {
	key string // 固定的 API 密钥
}

// NewStaticSecretSource 创建一个返回固定密钥的密钥源。
func NewStaticSecretSource(key string) *StaticSecretSource {
	return &StaticSecretSource{key: strings.TrimSpace(key)}
}

// Get 返回静态 API 密钥，始终成功。
func (s *StaticSecretSource) Get(ctx context.Context) (string, error) {
	return s.key, nil
}

// MappedSecretSource 包装默认的 SecretSource，添加客户端到上游 API 密钥的映射功能。
// 当请求上下文中包含匹配映射的客户端 API 密钥时，返回对应的上游密钥；
// 否则降级到默认密钥源。
type MappedSecretSource struct {
	defaultSource SecretSource       // 默认密钥源，用于未匹配映射时的降级
	mu            sync.RWMutex       // 保护映射表的读写锁
	lookup        map[string]string  // 客户端密钥到上游密钥的映射表
}

// NewMappedSecretSource 创建一个 MappedSecretSource 实例，包装给定的默认密钥源。
func NewMappedSecretSource(defaultSource SecretSource) *MappedSecretSource {
	return &MappedSecretSource{
		defaultSource: defaultSource,
		lookup:        make(map[string]string),
	}
}

// Get 获取 Amp API 密钥，优先检查客户端到上游的密钥映射。
// 如果请求上下文中的客户端密钥匹配映射，返回对应的上游密钥；
// 否则降级到默认密钥源。
func (s *MappedSecretSource) Get(ctx context.Context) (string, error) {
	// Try to get client API key from request context
	clientKey := getClientAPIKeyFromContext(ctx)
	if clientKey != "" {
		s.mu.RLock()
		if upstreamKey, ok := s.lookup[clientKey]; ok && upstreamKey != "" {
			s.mu.RUnlock()
			return upstreamKey, nil
		}
		s.mu.RUnlock()
	}

	// Fall back to default source
	return s.defaultSource.Get(ctx)
}

// UpdateMappings 从配置条目重建客户端到上游密钥的映射表。
// 如果同一客户端密钥出现在多个条目中，记录警告并使用第一个映射。
func (s *MappedSecretSource) UpdateMappings(entries []config.AmpUpstreamAPIKeyEntry) {
	newLookup := make(map[string]string)

	for _, entry := range entries {
		upstreamKey := strings.TrimSpace(entry.UpstreamAPIKey)
		if upstreamKey == "" {
			continue
		}
		for _, clientKey := range entry.APIKeys {
			trimmedKey := strings.TrimSpace(clientKey)
			if trimmedKey == "" {
				continue
			}
			if _, exists := newLookup[trimmedKey]; exists {
				// Log warning for duplicate client key, first one wins
				log.Warnf("amp upstream-api-keys: client API key appears in multiple entries; using first mapping.")
				continue
			}
			newLookup[trimmedKey] = upstreamKey
		}
	}

	s.mu.Lock()
	s.lookup = newLookup
	s.mu.Unlock()
}

// UpdateDefaultExplicitKey 更新底层 MultiSourceSecret 的显式密钥（如果适用）。
func (s *MappedSecretSource) UpdateDefaultExplicitKey(key string) {
	if ms, ok := s.defaultSource.(*MultiSourceSecret); ok {
		ms.UpdateExplicitKey(key)
	}
}

// InvalidateCache 使底层 MultiSourceSecret 的缓存失效（如果适用）。
func (s *MappedSecretSource) InvalidateCache() {
	if ms, ok := s.defaultSource.(*MultiSourceSecret); ok {
		ms.InvalidateCache()
	}
}
