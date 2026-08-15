package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchesModelListSupportsRuntimeModelSemantics(t *testing.T) {
	require.True(t, MatchesModelList([]string{"gpt-5"}, "gpt-5"))
	require.True(t, MatchesModelList([]string{"*"}, "claude-3-5-haiku"))
	require.True(t, MatchesModelList([]string{"gpt-*"}, "gpt-4o-mini"))
	require.True(t, MatchesModelList([]string{"gpt-4o-gizmo-*"}, "gpt-4o-gizmo-preview"))
	require.False(t, MatchesModelList([]string{"gpt-4"}, "gpt-4.1"))
}

func TestSplitCommaValuesTrimsAndDropsEmptyItems(t *testing.T) {
	require.Equal(t, []string{"gpt-5", "claude-3"}, SplitCommaValues(" gpt-5, ,claude-3,"))
	require.Nil(t, SplitCommaValues("  "))
}
