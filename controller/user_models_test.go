package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildUserModelsForGroupsFiltersRequestedGroup(t *testing.T) {
	modelsByGroup := map[string][]string{
		"default": []string{"gpt-4o", "gpt-4o-mini"},
		"vip":     []string{"claude-sonnet-4"},
	}
	getGroupModels := func(group string) []string {
		return modelsByGroup[group]
	}

	assert.Equal(
		t,
		[]string{"claude-sonnet-4"},
		buildUserModelsForGroups(
			"vip",
			map[string]string{"default": "Default", "vip": "VIP"},
			getGroupModels,
		),
	)
}

func TestBuildUserModelsForGroupsReturnsEmptyForUnavailableGroup(t *testing.T) {
	called := false
	getGroupModels := func(group string) []string {
		called = true
		return []string{group}
	}

	assert.Empty(
		t,
		buildUserModelsForGroups(
			"private",
			map[string]string{"default": "Default"},
			getGroupModels,
		),
	)
	assert.False(t, called)
}

func TestBuildUserModelsForGroupsKeepsLegacyAggregateBehavior(t *testing.T) {
	modelsByGroup := map[string][]string{
		"default": []string{"gpt-4o", "shared-model"},
		"vip":     []string{"shared-model", "claude-sonnet-4"},
	}
	getGroupModels := func(group string) []string {
		return modelsByGroup[group]
	}

	assert.ElementsMatch(
		t,
		[]string{"gpt-4o", "shared-model", "claude-sonnet-4"},
		buildUserModelsForGroups(
			"",
			map[string]string{"default": "Default", "vip": "VIP"},
			getGroupModels,
		),
	)
}
