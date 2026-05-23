package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"gorm.io/gorm"
)

const (
	AccountPoolCLIProxyGroupHeader = "X-NexusTok-Account-Pool-Group"

	defaultAccountPoolCLIProxyURL           = "http://127.0.0.1:8317"
	defaultAccountPoolCLIProxyManagementKey = "nexustok-account-pool-local"
	defaultAccountPoolCLIProxyRelayKey      = "nexustok-account-pool-relay-local"
)

type cliProxyAuthFilesResponse struct {
	Files []cliProxyAuthFileEntry `json:"files"`
}

type cliProxyAuthFileEntry struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Provider      string   `json:"provider"`
	Disabled      bool     `json:"disabled"`
	Unavailable   bool     `json:"unavailable"`
	AccountGroup  string   `json:"account_group"`
	AccountGroups []string `json:"account_groups"`
}

type cliproxyGroupAggregate struct {
	name        string
	platforms   map[string]int
	total       int64
	enabled     int64
	disabled    int64
	unavailable int64
}

// AccountPoolCLIProxyURL 返回 NexusTok 访问内部 CLIProxyAPI sidecar 的地址。
func AccountPoolCLIProxyURL() string {
	value := strings.TrimRight(strings.TrimSpace(os.Getenv("ACCOUNT_POOL_CLI_PROXY_URL")), "/")
	if value == "" {
		value = defaultAccountPoolCLIProxyURL
	}
	return value
}

// AccountPoolCLIProxyRelayKey 返回只在后端和容器网络内使用的 relay key。
func AccountPoolCLIProxyRelayKey() string {
	value := strings.TrimSpace(os.Getenv("ACCOUNT_POOL_CLI_PROXY_RELAY_KEY"))
	if value == "" {
		value = defaultAccountPoolCLIProxyRelayKey
	}
	return value
}

// AccountPoolCLIProxyManagementKey 返回只在后端管理代理中使用的 management key。
func AccountPoolCLIProxyManagementKey() string {
	key := strings.TrimSpace(os.Getenv("ACCOUNT_POOL_CLI_PROXY_MANAGEMENT_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("ACCOUNT_POOL_MANAGEMENT_KEY"))
	}
	if key == "" {
		key = defaultAccountPoolCLIProxyManagementKey
	}
	return key
}

// SyncCLIProxyAccountGroups 将 CPAMC/CLIProxyAPI 管理分组同步为 NexusTok 可引用的账号池组镜像。
func SyncCLIProxyAccountGroups(ctx context.Context) error {
	entries, err := fetchCLIProxyAuthFiles(ctx)
	if err != nil {
		return err
	}
	aggregates := aggregateCLIProxyGroups(entries)
	return upsertCLIProxyAccountGroups(aggregates)
}

func fetchCLIProxyAuthFiles(ctx context.Context) ([]cliProxyAuthFileEntry, error) {
	targetURL, err := url.JoinPath(AccountPoolCLIProxyURL(), "/v0/management/auth-files")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+AccountPoolCLIProxyManagementKey())

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("CLIProxyAPI auth-files 返回状态码 %d", resp.StatusCode)
	}

	var payload cliProxyAuthFilesResponse
	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return nil, err
	}
	return payload.Files, nil
}

func aggregateCLIProxyGroups(entries []cliProxyAuthFileEntry) map[string]*cliproxyGroupAggregate {
	aggregates := map[string]*cliproxyGroupAggregate{}
	for _, entry := range entries {
		groups := normalizeCLIProxyGroupValues(entry.AccountGroup, strings.Join(entry.AccountGroups, "\n"))
		for _, groupName := range groups {
			aggregate := aggregates[groupName]
			if aggregate == nil {
				aggregate = &cliproxyGroupAggregate{
					name:      groupName,
					platforms: map[string]int{},
				}
				aggregates[groupName] = aggregate
			}
			aggregate.total++
			if entry.Disabled {
				aggregate.disabled++
			} else {
				aggregate.enabled++
			}
			if entry.Unavailable {
				aggregate.unavailable++
			}
			if platform := normalizeCLIProxyPlatform(entry.Provider, entry.Type); platform != "" {
				aggregate.platforms[platform]++
			}
		}
	}
	return aggregates
}

func upsertCLIProxyAccountGroups(aggregates map[string]*cliproxyGroupAggregate) error {
	now := common.GetTimestamp()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		names := make([]string, 0, len(aggregates))
		for name := range aggregates {
			names = append(names, name)
		}
		sort.Strings(names)
		seen := make(map[string]struct{}, len(names))
		for _, name := range names {
			aggregate := aggregates[name]
			if aggregate == nil {
				continue
			}
			seen[name] = struct{}{}
			group := &model.AccountPoolGroup{
				Name:        aggregate.name,
				Platform:    aggregate.primaryPlatform(),
				AuthType:    model.AccountPoolAuthTypeOfficialOAuth,
				Source:      model.AccountPoolGroupSourceCLIProxyAPI,
				ExternalKey: aggregate.name,
				Status:      common.ChannelStatusEnabled,
				Strategy:    model.AccountPoolStrategyRoundRobin,
				UpdatedTime: now,
			}
			var existing model.AccountPoolGroup
			err := tx.Where("source = ? AND external_group_key = ?", model.AccountPoolGroupSourceCLIProxyAPI, aggregate.name).First(&existing).Error
			if err == nil {
				if updateErr := tx.Model(&existing).Updates(map[string]interface{}{
					"name":               group.Name,
					"platform":           group.Platform,
					"auth_type":          group.AuthType,
					"status":             group.Status,
					"strategy":           group.Strategy,
					"updated_time":       group.UpdatedTime,
					"external_group_key": group.ExternalKey,
				}).Error; updateErr != nil {
					return updateErr
				}
				continue
			}
			if err != gorm.ErrRecordNotFound {
				return err
			}
			if createErr := tx.Create(group).Error; createErr != nil {
				return createErr
			}
		}
		return disableMissingCLIProxyAccountGroups(tx, seen, now)
	})
}

func disableMissingCLIProxyAccountGroups(tx *gorm.DB, activeGroups map[string]struct{}, now int64) error {
	var groups []model.AccountPoolGroup
	if err := tx.Where("source = ?", model.AccountPoolGroupSourceCLIProxyAPI).Find(&groups).Error; err != nil {
		return err
	}
	missingIds := make([]int, 0, len(groups))
	for _, group := range groups {
		groupKey := strings.TrimSpace(group.ExternalKey)
		if groupKey == "" {
			groupKey = strings.TrimSpace(group.Name)
		}
		if groupKey == "" {
			continue
		}
		if _, ok := activeGroups[groupKey]; ok {
			continue
		}
		missingIds = append(missingIds, group.Id)
	}
	if len(missingIds) == 0 {
		return nil
	}
	return tx.Model(&model.AccountPoolGroup{}).Where("id IN ?", missingIds).Updates(map[string]interface{}{
		"status":       common.ChannelStatusManuallyDisabled,
		"updated_time": now,
	}).Error
}

func (aggregate *cliproxyGroupAggregate) primaryPlatform() string {
	if aggregate == nil || len(aggregate.platforms) == 0 {
		return "cliproxyapi"
	}
	type platformCount struct {
		platform string
		count    int
	}
	items := make([]platformCount, 0, len(aggregate.platforms))
	for platform, count := range aggregate.platforms {
		items = append(items, platformCount{platform: platform, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].platform < items[j].platform
	})
	if len(items) > 1 {
		return "cliproxyapi"
	}
	return items[0].platform
}

func normalizeCLIProxyPlatform(values ...string) string {
	for _, value := range values {
		platform := strings.ToLower(strings.TrimSpace(value))
		if platform != "" {
			return platform
		}
	}
	return ""
}

func normalizeCLIProxyGroupValues(values ...string) []string {
	seen := map[string]struct{}{}
	groups := make([]string, 0, len(values))
	add := func(value string) {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ';'
		}) {
			groupName := strings.Join(strings.Fields(strings.TrimSpace(part)), " ")
			if groupName == "" {
				continue
			}
			if _, ok := seen[groupName]; ok {
				continue
			}
			seen[groupName] = struct{}{}
			groups = append(groups, groupName)
		}
	}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			var decoded []string
			if err := common.UnmarshalJsonStr(trimmed, &decoded); err == nil {
				for _, item := range decoded {
					add(item)
				}
				continue
			}
		}
		add(trimmed)
	}
	return groups
}

// CLIProxyGroupStats 读取当前 sidecar 分组统计，列表接口用于展示镜像组实时状态。
func CLIProxyGroupStats(ctx context.Context) (map[string]map[string]int64, error) {
	entries, err := fetchCLIProxyAuthFiles(ctx)
	if err != nil {
		return nil, err
	}
	aggregates := aggregateCLIProxyGroups(entries)
	stats := make(map[string]map[string]int64, len(aggregates))
	for name, aggregate := range aggregates {
		if aggregate == nil {
			continue
		}
		stats[name] = map[string]int64{
			"total":       aggregate.total,
			"enabled":     aggregate.enabled,
			"disabled":    aggregate.disabled,
			"cooldown":    0,
			"unavailable": aggregate.unavailable,
		}
	}
	return stats, nil
}

// BuildCLIProxyGroupHeaderOverride 构造发往 sidecar 的组过滤请求头。
func BuildCLIProxyGroupHeaderOverride(group *model.AccountPoolGroup) map[string]interface{} {
	groupKey := ""
	if group != nil {
		groupKey = strings.TrimSpace(group.ExternalKey)
		if groupKey == "" {
			groupKey = strings.TrimSpace(group.Name)
		}
	}
	if groupKey == "" {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		AccountPoolCLIProxyGroupHeader: groupKey,
	}
}

// MergeHeaderOverrides 以账号池内部调度头覆盖渠道侧同名配置，避免用户误覆盖组过滤。
func MergeHeaderOverrides(base map[string]interface{}, overrides map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(base)+len(overrides))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

func IsCLIProxyAccountPoolGroup(group *model.AccountPoolGroup) bool {
	return group != nil && strings.EqualFold(strings.TrimSpace(group.Source), model.AccountPoolGroupSourceCLIProxyAPI)
}

func AccountPoolSidecarUnavailableError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("同步 CLIProxyAPI 账号组失败: %w", err)
}
