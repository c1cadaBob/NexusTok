package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateModelRatioByJSONStringMergesDefaultsAndOverrides(t *testing.T) {
	originalModelRatio := GetModelRatioCopy()
	originalModelPrice := GetModelPriceCopy()
	t.Cleanup(func() {
		modelRatioMap.ReplaceAll(originalModelRatio)
		modelPriceMap.ReplaceAll(originalModelPrice)
		InvalidateExposedDataCache()
	})

	require.NoError(t, UpdateModelRatioByJSONString("{}"))
	ratio, ok, matchedName := GetModelRatio("gpt-5")
	require.True(t, ok)
	require.Equal(t, "gpt-5", matchedName)
	require.InDelta(t, 0.625, ratio, 0.0000001)
	ratio, ok, matchedName = GetModelRatio("gpt-5.5")
	require.True(t, ok)
	require.Equal(t, "gpt-5.5", matchedName)
	require.InDelta(t, 2.5, ratio, 0.0000001)

	require.NoError(t, UpdateModelRatioByJSONString(`{"gpt-5":1.5,"custom-loadtest-model":2}`))
	ratio, ok, _ = GetModelRatio("gpt-5")
	require.True(t, ok)
	require.InDelta(t, 1.5, ratio, 0.0000001)
	ratio, ok, _ = GetModelRatio("custom-loadtest-model")
	require.True(t, ok)
	require.InDelta(t, 2, ratio, 0.0000001)
}

func TestUpdateModelPriceByJSONStringMergesDefaultsAndOverrides(t *testing.T) {
	originalModelPrice := GetModelPriceCopy()
	t.Cleanup(func() {
		modelPriceMap.ReplaceAll(originalModelPrice)
		InvalidateExposedDataCache()
	})

	require.NoError(t, UpdateModelPriceByJSONString("{}"))
	price, ok := GetModelPrice("dall-e-3", false)
	require.True(t, ok)
	require.InDelta(t, 0.04, price, 0.0000001)

	require.NoError(t, UpdateModelPriceByJSONString(`{"dall-e-3":0.08,"custom-fixed-model":0.2}`))
	price, ok = GetModelPrice("dall-e-3", false)
	require.True(t, ok)
	require.InDelta(t, 0.08, price, 0.0000001)
	price, ok = GetModelPrice("custom-fixed-model", false)
	require.True(t, ok)
	require.InDelta(t, 0.2, price, 0.0000001)
}
