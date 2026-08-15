package upstreamaccount

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSuggestedWeightForRatioBoundaries(t *testing.T) {
	cases := []struct {
		name  string
		ratio float64
		want  int
	}{
		{name: "missing", ratio: 0, want: 100},
		{name: "tiny discount rounds up", ratio: 0.999, want: 101},
		{name: "half cent discount rounds up", ratio: 0.995, want: 101},
		{name: "one cent discount", ratio: 0.99, want: 101},
		{name: "neutral", ratio: 1, want: 100},
		{name: "tiny premium rounds down", ratio: 1.001, want: 99},
		{name: "one cent premium", ratio: 1.01, want: 99},
		{name: "large premium clamps to zero", ratio: 2.5, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, SuggestedWeightForRatio(tc.ratio))
		})
	}
}

func TestApplySuggestionsUsesUnifiedPriority(t *testing.T) {
	cheapRatio := 0.8
	expensiveRatio := 1.2
	snapshot := &Snapshot{
		Keys: []SyncedKey{
			{Name: "cheap", GroupRatio: &cheapRatio},
			{Name: "expensive", GroupRatio: &expensiveRatio},
		},
	}

	ApplySuggestions(snapshot)

	require.EqualValues(t, 1, snapshot.Keys[0].SuggestedPriority)
	require.EqualValues(t, 1, snapshot.Keys[1].SuggestedPriority)
	require.Equal(t, 120, snapshot.Keys[0].SuggestedWeight)
	require.Equal(t, 80, snapshot.Keys[1].SuggestedWeight)
}
