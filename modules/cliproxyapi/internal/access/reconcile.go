// access - reconcile.go
// 访问认证提供者的协调（Reconciliation）逻辑。
// 在配置热更新时，该模块负责比较新旧配置中的提供者差异，确定哪些提供者需要新增、
// 更新或移除，从而实现平滑的配置变更而无需重启服务。
package access

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	configaccess "github.com/router-for-me/CLIProxyAPI/v7/internal/access/config_access"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	log "github.com/sirupsen/logrus"
)

// ReconcileProviders 构建期望的提供者列表，在可能的情况下复用现有提供者，
// 仅在配置发生变化时才创建或移除提供者。返回最终排序的提供者切片，
// 以及与之前配置相比被新增、更新或移除的提供者标识列表。
//
// 参数：
//   - oldCfg: 旧配置（当前未使用，保留用于未来扩展）
//   - newCfg: 新配置，为 nil 时返回空结果
//   - existing: 当前已注册的提供者列表
//
// 返回值：
//   - result: 最终的提供者列表
//   - added: 新增的提供者标识列表
//   - updated: 更新的提供者标识列表
//   - removed: 移除的提供者标识列表
//   - err: 错误信息
func ReconcileProviders(oldCfg, newCfg *config.Config, existing []sdkaccess.Provider) (result []sdkaccess.Provider, added, updated, removed []string, err error) {
	_ = oldCfg
	if newCfg == nil {
		return nil, nil, nil, nil, nil
	}

	result = sdkaccess.RegisteredProviders()

	existingMap := make(map[string]sdkaccess.Provider, len(existing))
	for _, provider := range existing {
		providerID := identifierFromProvider(provider)
		if providerID == "" {
			continue
		}
		existingMap[providerID] = provider
	}

	finalIDs := make(map[string]struct{}, len(result))

	// isInlineProvider 判断给定的提供者标识是否为内联提供者（默认提供者）。
	// 内联提供者不参与变更追踪，因为它们始终存在。
	isInlineProvider := func(id string) bool {
		return strings.EqualFold(id, sdkaccess.DefaultAccessProviderName)
	}
	// appendChange 将提供者标识追加到变更列表中，但会跳过内联提供者。
	appendChange := func(list *[]string, id string) {
		if isInlineProvider(id) {
			return
		}
		*list = append(*list, id)
	}

	for _, provider := range result {
		providerID := identifierFromProvider(provider)
		if providerID == "" {
			continue
		}
		finalIDs[providerID] = struct{}{}

		existingProvider, exists := existingMap[providerID]
		if !exists {
			appendChange(&added, providerID)
			continue
		}
		if !providerInstanceEqual(existingProvider, provider) {
			appendChange(&updated, providerID)
		}
	}

	for providerID := range existingMap {
		if _, exists := finalIDs[providerID]; exists {
			continue
		}
		appendChange(&removed, providerID)
	}

	sort.Strings(added)
	sort.Strings(updated)
	sort.Strings(removed)

	return result, added, updated, removed, nil
}

// ApplyAccessProviders 协调配置的访问认证提供者与当前注册的提供者，并更新管理器。
// 该函数会：
//  1. 获取当前已注册的提供者列表
//  2. 注册基于配置的 API Key 提供者
//  3. 执行提供者协调以检测变更
//  4. 更新管理器中的提供者列表
//
// 返回是否有任何提供者发生变化以及可能的错误。
func ApplyAccessProviders(manager *sdkaccess.Manager, oldCfg, newCfg *config.Config) (bool, error) {
	if manager == nil || newCfg == nil {
		return false, nil
	}

	existing := manager.Providers()
	configaccess.Register(&newCfg.SDKConfig)
	providers, added, updated, removed, err := ReconcileProviders(oldCfg, newCfg, existing)
	if err != nil {
		log.Errorf("failed to reconcile request auth providers: %v", err)
		return false, fmt.Errorf("reconciling access providers: %w", err)
	}

	manager.SetProviders(providers)

	if len(added)+len(updated)+len(removed) > 0 {
		log.Debugf("auth providers reconciled (added=%d updated=%d removed=%d)", len(added), len(updated), len(removed))
		log.Debugf("auth providers changes details - added=%v updated=%v removed=%v", added, updated, removed)
		return true, nil
	}

	log.Debug("auth providers unchanged after config update")
	return false, nil
}

// identifierFromProvider 从提供者接口中提取标识名称。
// 如果提供者为 nil 则返回空字符串，否则返回去除首尾空白的标识名。
func identifierFromProvider(provider sdkaccess.Provider) string {
	if provider == nil {
		return ""
	}
	return strings.TrimSpace(provider.Identifier())
}

// providerInstanceEqual 比较两个提供者实例是否相等。
// 首先比较类型是否相同，然后对指针类型比较内存地址，否则使用 reflect.DeepEqual 进行深度比较。
// 这种策略在大多数情况下能高效地检测提供者是否发生了变化。
func providerInstanceEqual(a, b sdkaccess.Provider) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		return false
	}
	valueA := reflect.ValueOf(a)
	valueB := reflect.ValueOf(b)
	if valueA.Kind() == reflect.Pointer && valueB.Kind() == reflect.Pointer {
		return valueA.Pointer() == valueB.Pointer()
	}
	return reflect.DeepEqual(a, b)
}
