// 本文件测试账号池 CLI Proxy 相关功能，包括：
// - CLI Proxy 账号分组的聚合统计（总数、不可用数、禁用数）
// - CLI Proxy 分组值的归一化处理（支持换行、逗号、JSON 数组等多种格式）
// - 请求头覆盖合并逻辑（内部分组头优先级高于渠道头）
// - CLI Proxy 账号分组的数据库同步（新增活跃分组、禁用已移除的镜像分组）
package service

import (
	"reflect"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestAggregateCLIProxyGroupsSupportsMultipleGroups 测试将 CLI Proxy 账号条目按分组聚合统计，
// 验证多分组支持（一个条目可属于多个分组）、禁用/不可用状态计数，以及主平台推断逻辑。
func TestAggregateCLIProxyGroupsSupportsMultipleGroups(t *testing.T) {
	entries := []cliProxyAuthFileEntry{
		{
			Name:          "codex-a.json",
			Provider:      "codex",
			AccountGroups: []string{"main", "fallback"},
		},
		{
			Name:         "codex-b.json",
			Type:         "codex",
			Disabled:     true,
			AccountGroup: "fallback",
		},
		{
			Name:          "gemini-a.json",
			Provider:      "gemini",
			Unavailable:   true,
			AccountGroups: []string{"main"},
		},
	}

	stats := aggregateCLIProxyGroups(entries)

	// 验证 "main" 分组包含 2 个条目（codex-a 和 gemini-a）
	if got := stats["main"].total; got != 2 {
		t.Fatalf("main.total = %d, want 2", got)
	}
	// 验证 "main" 分组中有 1 个不可用条目（gemini-a 标记为 Unavailable）
	if got := stats["main"].unavailable; got != 1 {
		t.Fatalf("main.unavailable = %d, want 1", got)
	}
	// 验证 "fallback" 分组中有 1 个禁用条目（codex-b 标记为 Disabled）
	if got := stats["fallback"].disabled; got != 1 {
		t.Fatalf("fallback.disabled = %d, want 1", got)
	}
	// 验证 "main" 分组的主平台推断为 "cliproxyapi"（默认平台）
	if platform := stats["main"].primaryPlatform(); platform != "cliproxyapi" {
		t.Fatalf("main.primaryPlatform() = %q, want cliproxyapi", platform)
	}
}

// TestNormalizeCLIProxyGroupValues 测试分组值归一化函数，
// 验证支持换行分隔、逗号分隔、JSON 数组三种格式，并且能自动去重。
func TestNormalizeCLIProxyGroupValues(t *testing.T) {
	got := normalizeCLIProxyGroupValues("main\nfallback", "fallback, extra", `["json","main"]`)
	want := []string{"main", "fallback", "extra", "json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeCLIProxyGroupValues() = %#v, want %#v", got, want)
	}
}

// TestMergeHeaderOverridesKeepsInternalGroupHeader 测试请求头覆盖合并逻辑，
// 验证内部分组头（AccountPoolCLIProxyGroupHeader）的值优先于渠道头，
// 同时保留不在覆盖列表中的自定义头。
func TestMergeHeaderOverridesKeepsInternalGroupHeader(t *testing.T) {
	base := map[string]interface{}{
		AccountPoolCLIProxyGroupHeader: "channel-value",
		"X-Custom":                     "kept",
	}
	overrides := map[string]interface{}{
		AccountPoolCLIProxyGroupHeader: "internal-value",
	}

	got := MergeHeaderOverrides(base, overrides)

	if got[AccountPoolCLIProxyGroupHeader] != "internal-value" {
		t.Fatalf("group header = %#v, want internal-value", got[AccountPoolCLIProxyGroupHeader])
	}
	if got["X-Custom"] != "kept" {
		t.Fatalf("custom header = %#v, want kept", got["X-Custom"])
	}
}

// TestUpsertCLIProxyAccountGroupsDisablesMissingMirrorGroups 测试 CLI Proxy 账号分组的数据库同步逻辑，
// 验证：新的活跃分组会被正确创建并启用；旧的镜像分组（不在当前同步列表中）会被自动禁用。
func TestUpsertCLIProxyAccountGroupsDisablesMissingMirrorGroups(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AccountPoolGroup{}))
	originDB := model.DB
	model.DB = db
	defer func() {
		model.DB = originDB
	}()

	stale := &model.AccountPoolGroup{
		Name:        "stale",
		Platform:    "codex",
		AuthType:    model.AccountPoolAuthTypeOfficialOAuth,
		Source:      model.AccountPoolGroupSourceCLIProxyAPI,
		ExternalKey: "stale",
		Status:      common.ChannelStatusEnabled,
		Strategy:    model.AccountPoolStrategyRoundRobin,
	}
	require.NoError(t, model.DB.Create(stale).Error)

	require.NoError(t, upsertCLIProxyAccountGroups(map[string]*cliproxyGroupAggregate{
		"active": {
			name:      "active",
			platforms: map[string]int{"codex": 1},
			total:     1,
			enabled:   1,
		},
	}))

	var activeGroup model.AccountPoolGroup
	require.NoError(t, model.DB.Where("source = ? AND external_group_key = ?", model.AccountPoolGroupSourceCLIProxyAPI, "active").First(&activeGroup).Error)
	// 验证新同步的 "active" 分组状态为启用
	require.Equal(t, common.ChannelStatusEnabled, activeGroup.Status)

	var staleGroup model.AccountPoolGroup
	require.NoError(t, model.DB.First(&staleGroup, stale.Id).Error)
	// 验证不在同步列表中的旧 "stale" 分组已被禁用为手动禁用状态
	require.Equal(t, common.ChannelStatusManuallyDisabled, staleGroup.Status)
}
