package modelcatalog

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/billing_setting"
	"github.com/c1cada/NexusTok/setting/config"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestParseRepositoryFiles(t *testing.T) {
	files := map[string][]byte{
		"models/openai/gpt-test.toml": []byte(`
id = "gpt-test"
name = "GPT Test"
reasoning = true
tool_call = true
tags = ["Reasoning", "Tools"]

[limit]
context = 128000

[modalities]
input = ["text", "image"]
output = ["text"]
`),
		"providers/openai/provider.toml": []byte(`
id = "openai"
name = "OpenAI"
icon = "OpenAI.Color"
doc = "https://platform.openai.com/docs/models"
`),
		"providers/openai/models/gpt-test.toml": []byte(`
id = "gpt-test"

[cost]
input = 1.25
output = 10
cache_read = 0.125
`),
	}

	catalog, err := ParseRepositoryFiles(files)
	require.NoError(t, err)
	require.Equal(t, "GPT Test", catalog.Models["openai/gpt-test"].Name)
	require.True(t, catalog.Models["openai/gpt-test"].Reasoning)
	require.Equal(t, int64(128000), catalog.Models["openai/gpt-test"].Limit.Context)
	require.Equal(t, "OpenAI", catalog.Providers["openai"].Name)
	require.NotNil(t, catalog.Providers["openai"].Models["gpt-test"].Cost.Input)
	require.Equal(t, "nexustok-model-repository", catalog.Manifest.Name)
}

func TestWriteCatalogToRepositoryRoundTrip(t *testing.T) {
	input := &Catalog{
		Models: map[string]CatalogModel{
			"openai/gpt-roundtrip": {
				ID:         "gpt-roundtrip",
				Name:       "GPT Roundtrip",
				Status:     "active",
				Reasoning:  true,
				Tags:       []string{"Reasoning"},
				Modalities: CatalogModalities{Input: []string{"text"}, Output: []string{"text"}},
			},
		},
		Providers: map[string]CatalogProvider{
			"openai": {
				ID:     "openai",
				Name:   "OpenAI",
				Status: "active",
				Models: map[string]CatalogModel{
					"gpt-roundtrip": {
						ID:     "gpt-roundtrip",
						Status: "active",
						Cost:   CatalogCost{Input: f64(2), Output: f64(8)},
					},
				},
			},
		},
	}

	dir := t.TempDir()
	require.NoError(t, WriteCatalogToRepository(dir, input))
	loaded, err := LoadRepository(dir)
	require.NoError(t, err)
	require.Equal(t, input.Models["openai/gpt-roundtrip"].Name, loaded.Models["openai/gpt-roundtrip"].Name)
	require.NotEmpty(t, loaded.Manifest.Hash)
	require.FileExists(t, filepath.Join(dir, "catalog.generated.json"))
	require.FileExists(t, filepath.Join(dir, "manifest.json"))
}

func TestSeedCatalogCreatesMissingAndPreservesManualPricing(t *testing.T) {
	db := setupModelCatalogTestDB(t)
	openai := CatalogProvider{
		ID:     "openai",
		Name:   "OpenAI",
		Icon:   "OpenAI.Color",
		Status: "active",
		Models: map[string]CatalogModel{
			"gpt-seed-new": {
				ID:     "gpt-seed-new",
				Status: "active",
				Cost:   CatalogCost{Input: f64(2), Output: f64(8)},
			},
			"gpt-seed-manual": {
				ID:     "gpt-seed-manual",
				Status: "active",
				Cost:   CatalogCost{Input: f64(1), Output: f64(4)},
			},
		},
	}
	catalog := &Catalog{
		Models: map[string]CatalogModel{
			"openai/gpt-seed-new": {
				ID:     "gpt-seed-new",
				Name:   "GPT Seed New",
				Status: "active",
			},
			"openai/gpt-seed-manual": {
				ID:     "gpt-seed-manual",
				Name:   "GPT Seed Manual",
				Status: "active",
			},
		},
		Providers: map[string]CatalogProvider{"openai": openai},
	}

	manual := &model.Model{ModelName: "gpt-seed-manual", Status: 1, SyncOfficial: 1, NameRule: model.NameRuleExact}
	require.NoError(t, manual.Insert())
	require.NoError(t, model.SaveModelPricingConfig("gpt-seed-manual", model.ModelPricingUpdateRequest{
		BillingMode:           model.ModelPricingModeRatio,
		InputPricePerMillion:  f64(9),
		OutputPricePerMillion: f64(18),
	}))

	result, err := SeedCatalog(catalog)
	require.NoError(t, err)
	require.Equal(t, 1, result.CreatedModels)
	require.Equal(t, 1, result.PricingUpdated)

	var saved model.Model
	require.NoError(t, db.Where("model_name = ?", "gpt-seed-new").First(&saved).Error)
	require.Equal(t, 1, saved.SyncOfficial)
	require.Equal(t, model.ModelPricingSourceBuiltin, model.GetModelPricingSourceCopy()["gpt-seed-new"].Kind)
	requireFloatMapValue(t, ratio_setting.GetModelRatioCopy(), "gpt-seed-new", 1)
	requireFloatMapValue(t, ratio_setting.GetCompletionRatioCopy(), "gpt-seed-new", 4)
	requireFloatMapValue(t, ratio_setting.GetModelRatioCopy(), "gpt-seed-manual", 4.5)
	require.Equal(t, model.ModelPricingSourceManual, model.GetModelPricingSourceCopy()["gpt-seed-manual"].Kind)
}

func setupModelCatalogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Vendor{}, &model.Option{}))
	resetModelCatalogPricingState(t)
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func resetModelCatalogPricingState(t *testing.T) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
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
	require.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode("__modelcatalog_reset__"))
	require.NoError(t, model.SetModelPricingSource("__modelcatalog_reset__", model.ModelPricingSource{}))
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = map[string]string{}
		common.OptionMapRWMutex.Unlock()
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
	})
}

func f64(value float64) *float64 {
	return &value
}

func requireFloatMapValue(t *testing.T, got map[string]float64, key string, want float64) {
	t.Helper()
	value, ok := got[key]
	require.True(t, ok, "missing key %s in %v", key, got)
	require.InDelta(t, want, value, 0.0000001)
}
