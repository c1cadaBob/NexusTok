// Package model - model_pricing_config_test.go
// 该文件验证模型级定价聚合层对现有 options 的读写与互斥清理行为。
package model

import (
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/billing_setting"
	"github.com/c1cada/NexusTok/setting/config"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func withModelPricingConfigTestState(t *testing.T) {
	t.Helper()

	savedOptions := map[string]string{}
	common.OptionMapRWMutex.Lock()
	for k, v := range common.OptionMap {
		savedOptions[k] = common.Interface2String(v)
	}
	common.OptionMapRWMutex.Unlock()
	originalSelfUseMode := operation_setting.SelfUseModeEnabled
	require.NoError(t, DB.Exec("DELETE FROM options").Error)

	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = make(map[string]string, len(savedOptions))
		for k, v := range savedOptions {
			common.OptionMap[k] = v
		}
		common.OptionMapRWMutex.Unlock()
		operation_setting.SelfUseModeEnabled = originalSelfUseMode
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString("{}"))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString("{}"))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString("{}"))
		require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString("{}"))
		require.NoError(t, ratio_setting.UpdateCreateCacheRatioByJSONString("{}"))
		require.NoError(t, ratio_setting.UpdateImageRatioByJSONString("{}"))
		require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString("{}"))
		require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString("{}"))
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"billing_setting.billing_mode": "{}",
			"billing_setting.billing_expr": "{}",
		}))
		InvalidatePricingCache()
	})

	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	operation_setting.SelfUseModeEnabled = false
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString("{}"))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString("{}"))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString("{}"))
	require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString("{}"))
	require.NoError(t, ratio_setting.UpdateCreateCacheRatioByJSONString("{}"))
	require.NoError(t, ratio_setting.UpdateImageRatioByJSONString("{}"))
	require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString("{}"))
	require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString("{}"))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": "{}",
		"billing_setting.billing_expr": "{}",
	}))
	InvalidatePricingCache()
}

func f64ptr(value float64) *float64 {
	return &value
}

func strptr(value string) *string {
	return &value
}

func requireFloatMapValue(t *testing.T, got map[string]float64, key string, want float64) {
	t.Helper()
	value, ok := got[key]
	require.True(t, ok, "missing key %s in %v", key, got)
	require.InDelta(t, want, value, 0.0000001)
}

func TestBuildModelPricingConfigShowsOverridesAndEffectiveValues(t *testing.T) {
	withModelPricingConfigTestState(t)
	require.NoError(t, SaveModelPricingConfig("zz-pricing-read", ModelPricingUpdateRequest{
		BillingMode:           ModelPricingModeRatio,
		InputPricePerMillion:  f64ptr(2),
		OutputPricePerMillion: f64ptr(8),
		CacheRatio:            f64ptr(0.25),
		CreateCacheRatio:      f64ptr(1.25),
		ImageRatio:            f64ptr(1.5),
		AudioRatio:            f64ptr(2.5),
		AudioCompletionRatio:  f64ptr(3.5),
	}))

	config := BuildModelPricingConfig(7, "zz-pricing-read")
	require.Equal(t, 7, config.ModelID)
	require.Equal(t, "zz-pricing-read", config.ModelName)
	require.Equal(t, ModelPricingModeRatio, config.BillingMode)
	require.NotNil(t, config.Effective.InputPricePerMillion)
	require.NotNil(t, config.Effective.OutputPricePerMillion)
	require.NotNil(t, config.Override.InputPricePerMillion)
	require.NotNil(t, config.Override.OutputPricePerMillion)
	require.InEpsilon(t, 2, *config.Effective.InputPricePerMillion, 0.0000001)
	require.InEpsilon(t, 8, *config.Effective.OutputPricePerMillion, 0.0000001)
	require.InEpsilon(t, 4, *config.Effective.CompletionRatio, 0.0000001)
	require.InEpsilon(t, 0.25, *config.Override.CacheRatio, 0.0000001)
	require.InEpsilon(t, 1.25, *config.Override.CreateCacheRatio, 0.0000001)
	require.InEpsilon(t, 1.5, *config.Override.ImageRatio, 0.0000001)
	require.InEpsilon(t, 2.5, *config.Override.AudioRatio, 0.0000001)
	require.InEpsilon(t, 3.5, *config.Override.AudioCompletionRatio, 0.0000001)
}

func TestSaveModelPricingConfigFixedClearsOtherModes(t *testing.T) {
	withModelPricingConfigTestState(t)
	expr := `tier("base", p * 1 + c * 2)`
	require.NoError(t, SaveModelPricingConfig("zz-pricing-fixed", ModelPricingUpdateRequest{
		BillingMode:           ModelPricingModeTieredExpr,
		BillingExpr:           &expr,
		InputPricePerMillion:  f64ptr(2),
		OutputPricePerMillion: f64ptr(4),
		CacheRatio:            f64ptr(0.5),
	}))

	require.NoError(t, SaveModelPricingConfig("zz-pricing-fixed", ModelPricingUpdateRequest{
		BillingMode: ModelPricingModeFixed,
		ModelPrice:  f64ptr(0.0123),
	}))

	price, ok := ratio_setting.GetModelPrice("zz-pricing-fixed", false)
	require.True(t, ok)
	require.InEpsilon(t, 0.0123, price, 0.0000001)
	require.NotContains(t, ratio_setting.GetModelRatioCopy(), "zz-pricing-fixed")
	require.NotContains(t, ratio_setting.GetCompletionRatioCopy(), "zz-pricing-fixed")
	require.NotContains(t, ratio_setting.GetCacheRatioCopy(), "zz-pricing-fixed")
	require.NotContains(t, billing_setting.GetBillingModeCopy(), "zz-pricing-fixed")
	require.NotContains(t, billing_setting.GetBillingExprCopy(), "zz-pricing-fixed")
	require.Equal(t, ModelPricingModeFixed, BuildModelPricingConfig(1, "zz-pricing-fixed").BillingMode)
}

func TestSaveModelPricingConfigRatioClearsFixedAndTiered(t *testing.T) {
	withModelPricingConfigTestState(t)
	require.NoError(t, SaveModelPricingConfig("zz-pricing-ratio", ModelPricingUpdateRequest{
		BillingMode: ModelPricingModeFixed,
		ModelPrice:  f64ptr(0.02),
	}))
	expr := `tier("base", p * 1 + c * 2)`
	require.NoError(t, SaveModelPricingConfig("zz-pricing-ratio", ModelPricingUpdateRequest{
		BillingMode: ModelPricingModeTieredExpr,
		BillingExpr: &expr,
	}))

	require.NoError(t, SaveModelPricingConfig("zz-pricing-ratio", ModelPricingUpdateRequest{
		BillingMode:           ModelPricingModeRatio,
		InputPricePerMillion:  f64ptr(6),
		OutputPricePerMillion: f64ptr(18),
		CacheRatio:            f64ptr(0),
	}))

	require.NotContains(t, ratio_setting.GetModelPriceCopy(), "zz-pricing-ratio")
	require.NotContains(t, billing_setting.GetBillingModeCopy(), "zz-pricing-ratio")
	require.NotContains(t, billing_setting.GetBillingExprCopy(), "zz-pricing-ratio")
	requireFloatMapValue(t, ratio_setting.GetModelRatioCopy(), "zz-pricing-ratio", 3)
	requireFloatMapValue(t, ratio_setting.GetCompletionRatioCopy(), "zz-pricing-ratio", 3)
	requireFloatMapValue(t, ratio_setting.GetCacheRatioCopy(), "zz-pricing-ratio", 0)
}

func TestSaveModelPricingConfigTieredValidatesBeforeWriting(t *testing.T) {
	withModelPricingConfigTestState(t)
	require.NoError(t, SaveModelPricingConfig("zz-pricing-tiered", ModelPricingUpdateRequest{
		BillingMode:           ModelPricingModeRatio,
		InputPricePerMillion:  f64ptr(2),
		OutputPricePerMillion: f64ptr(4),
	}))

	err := SaveModelPricingConfig("zz-pricing-tiered", ModelPricingUpdateRequest{
		BillingMode: ModelPricingModeTieredExpr,
		BillingExpr: strptr(`tier("bad", p *)`),
		ModelPrice:  f64ptr(0.1),
	})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "计费表达式校验失败"))
	require.NotContains(t, billing_setting.GetBillingModeCopy(), "zz-pricing-tiered")
	require.NotContains(t, ratio_setting.GetModelPriceCopy(), "zz-pricing-tiered")
	requireFloatMapValue(t, ratio_setting.GetModelRatioCopy(), "zz-pricing-tiered", 1)
	requireFloatMapValue(t, ratio_setting.GetCompletionRatioCopy(), "zz-pricing-tiered", 2)

	valid := `tier("base", p * 1 + c * 2)`
	require.NoError(t, SaveModelPricingConfig("zz-pricing-tiered", ModelPricingUpdateRequest{
		BillingMode:           ModelPricingModeTieredExpr,
		BillingExpr:           &valid,
		InputPricePerMillion:  f64ptr(2),
		OutputPricePerMillion: f64ptr(6),
	}))
	require.Equal(t, ModelPricingModeTieredExpr, billing_setting.GetBillingMode("zz-pricing-tiered"))
	gotExpr, ok := billing_setting.GetBillingExpr("zz-pricing-tiered")
	require.True(t, ok)
	require.Equal(t, valid, gotExpr)
	requireFloatMapValue(t, ratio_setting.GetModelRatioCopy(), "zz-pricing-tiered", 1)
	requireFloatMapValue(t, ratio_setting.GetCompletionRatioCopy(), "zz-pricing-tiered", 3)
}

func TestSaveModelPricingConfigRejectsImpossibleRatioPriceShape(t *testing.T) {
	withModelPricingConfigTestState(t)
	err := SaveModelPricingConfig("zz-pricing-impossible", ModelPricingUpdateRequest{
		BillingMode:           ModelPricingModeRatio,
		InputPricePerMillion:  f64ptr(0),
		OutputPricePerMillion: f64ptr(1),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "倍率模式无法表达")
	require.NotContains(t, ratio_setting.GetModelRatioCopy(), "zz-pricing-impossible")
}

func TestRenameModelPricingConfigMovesAllDirectKeys(t *testing.T) {
	withModelPricingConfigTestState(t)
	valid := `tier("base", p * 1 + c * 2)`
	require.NoError(t, SaveModelPricingConfig("zz-pricing-old", ModelPricingUpdateRequest{
		BillingMode:           ModelPricingModeTieredExpr,
		BillingExpr:           &valid,
		ModelPrice:            f64ptr(0.04),
		InputPricePerMillion:  f64ptr(2),
		OutputPricePerMillion: f64ptr(4),
		CacheRatio:            f64ptr(0.1),
		CreateCacheRatio:      f64ptr(1.25),
		ImageRatio:            f64ptr(1.5),
		AudioRatio:            f64ptr(2.5),
		AudioCompletionRatio:  f64ptr(3.5),
	}))

	require.NoError(t, RenameModelPricingConfig("zz-pricing-old", "zz-pricing-new"))

	for _, item := range []map[string]float64{
		ratio_setting.GetModelPriceCopy(),
		ratio_setting.GetModelRatioCopy(),
		ratio_setting.GetCompletionRatioCopy(),
		ratio_setting.GetCacheRatioCopy(),
		ratio_setting.GetCreateCacheRatioCopy(),
		ratio_setting.GetImageRatioCopy(),
		ratio_setting.GetAudioRatioCopy(),
		ratio_setting.GetAudioCompletionRatioCopy(),
	} {
		require.NotContains(t, item, "zz-pricing-old")
		require.Contains(t, item, "zz-pricing-new")
	}
	require.NotContains(t, billing_setting.GetBillingModeCopy(), "zz-pricing-old")
	require.NotContains(t, billing_setting.GetBillingExprCopy(), "zz-pricing-old")
	require.Equal(t, ModelPricingModeTieredExpr, billing_setting.GetBillingMode("zz-pricing-new"))
	gotExpr, ok := billing_setting.GetBillingExpr("zz-pricing-new")
	require.True(t, ok)
	require.Equal(t, valid, gotExpr)
}
