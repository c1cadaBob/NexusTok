package relay

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"
)

func TestApplyTaskOtherRatios(t *testing.T) {
	t.Run("normal ratios keep historical truncation semantics", func(t *testing.T) {
		info := &relaycommon.RelayInfo{
			PriceData: types.PriceData{
				Quota: 100,
				OtherRatios: map[string]float64{
					"duration":   1.5,
					"resolution": 2,
				},
			},
		}

		quota := applyTaskOtherRatios(info, "video-model")

		require.Equal(t, 300, quota)
		require.Equal(t, 300, info.PriceData.Quota)
		require.Nil(t, info.QuotaClamp)
	})

	t.Run("huge ratios saturate and record quota clamp", func(t *testing.T) {
		info := &relaycommon.RelayInfo{
			PriceData: types.PriceData{
				Quota: 2000,
				OtherRatios: map[string]float64{
					"duration": 1e100,
				},
			},
		}

		quota := applyTaskOtherRatios(info, "video-model")

		require.Equal(t, common.MaxQuota, quota)
		require.Equal(t, common.MaxQuota, info.PriceData.Quota)
		require.NotNil(t, info.QuotaClamp)
		require.Equal(t, common.QuotaClampOverflow, info.QuotaClamp.Kind)
		require.Equal(t, common.MaxQuota, info.QuotaClamp.Clamped)
	})

	t.Run("task price patch models skip ratio application", func(t *testing.T) {
		previousPatches := constant.TaskPricePatches
		constant.TaskPricePatches = []string{"patched-video-model"}
		t.Cleanup(func() {
			constant.TaskPricePatches = previousPatches
		})

		info := &relaycommon.RelayInfo{
			PriceData: types.PriceData{
				Quota: 100,
				OtherRatios: map[string]float64{
					"duration": 999,
				},
			},
		}

		quota := applyTaskOtherRatios(info, "patched-video-model")

		require.Equal(t, 100, quota)
		require.Equal(t, 100, info.PriceData.Quota)
		require.Nil(t, info.QuotaClamp)
	})
}

func TestRecalcQuotaFromRatiosUsesSaturatingConversion(t *testing.T) {
	t.Run("normal adjusted ratios recover base quota before recalculation", func(t *testing.T) {
		info := &relaycommon.RelayInfo{
			PriceData: types.PriceData{
				Quota: 300,
				OtherRatios: map[string]float64{
					"duration": 3,
				},
			},
		}

		quota := recalcQuotaFromRatios(info, map[string]float64{
			"duration": 2.5,
		})

		require.Equal(t, 250, quota)
		require.Nil(t, info.QuotaClamp)
	})

	t.Run("huge adjusted ratios saturate and record quota clamp", func(t *testing.T) {
		info := &relaycommon.RelayInfo{
			PriceData: types.PriceData{
				Quota: 300,
				OtherRatios: map[string]float64{
					"duration": 3,
				},
			},
		}

		quota := recalcQuotaFromRatios(info, map[string]float64{
			"duration": 1e100,
		})

		require.Equal(t, common.MaxQuota, quota)
		require.NotNil(t, info.QuotaClamp)
		require.Equal(t, common.QuotaClampOverflow, info.QuotaClamp.Kind)
		require.Equal(t, common.MaxQuota, info.QuotaClamp.Clamped)
	})
}
