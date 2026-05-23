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

	if got := stats["main"].total; got != 2 {
		t.Fatalf("main.total = %d, want 2", got)
	}
	if got := stats["main"].unavailable; got != 1 {
		t.Fatalf("main.unavailable = %d, want 1", got)
	}
	if got := stats["fallback"].disabled; got != 1 {
		t.Fatalf("fallback.disabled = %d, want 1", got)
	}
	if platform := stats["main"].primaryPlatform(); platform != "cliproxyapi" {
		t.Fatalf("main.primaryPlatform() = %q, want cliproxyapi", platform)
	}
}

func TestNormalizeCLIProxyGroupValues(t *testing.T) {
	got := normalizeCLIProxyGroupValues("main\nfallback", "fallback, extra", `["json","main"]`)
	want := []string{"main", "fallback", "extra", "json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeCLIProxyGroupValues() = %#v, want %#v", got, want)
	}
}

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
	require.Equal(t, common.ChannelStatusEnabled, activeGroup.Status)

	var staleGroup model.AccountPoolGroup
	require.NoError(t, model.DB.First(&staleGroup, stale.Id).Error)
	require.Equal(t, common.ChannelStatusManuallyDisabled, staleGroup.Status)
}
