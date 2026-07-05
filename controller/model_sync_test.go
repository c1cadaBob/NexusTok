package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/billing_setting"
	"github.com/c1cada/NexusTok/setting/config"
	"github.com/c1cada/NexusTok/setting/ratio_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupModelSyncTestDB 初始化模型同步测试专用 SQLite 内存库。
func setupModelSyncTestDB(t *testing.T) *gorm.DB {
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

	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Vendor{}, &model.Channel{}, &model.Ability{}))
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	resetModelSyncPricingState(t)

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func resetModelSyncPricingState(t *testing.T) {
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
	require.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode("__pricing_reset__"))
	require.NoError(t, model.SetModelPricingSource("__pricing_reset__", model.ModelPricingSource{}))
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

func f64ptr(value float64) *float64 {
	return &value
}

func requireFloatMapValue(t *testing.T, got map[string]float64, key string, want float64) {
	t.Helper()
	value, ok := got[key]
	require.True(t, ok, "missing key %s in %v", key, got)
	require.InDelta(t, want, value, 0.0000001)
}

// withModelsDevTestServer 使用本地 HTTP 服务替代 models.dev，避免单元测试依赖外网。
func withModelsDevTestServer(t *testing.T, payload string) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, modelsDevCatalogPath, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(server.Close)

	originalBase, hadBase := os.LookupEnv("MODELS_DEV_SYNC_BASE")
	require.NoError(t, os.Setenv("MODELS_DEV_SYNC_BASE", server.URL))
	t.Cleanup(func() {
		if hadBase {
			require.NoError(t, os.Setenv("MODELS_DEV_SYNC_BASE", originalBase))
		} else {
			require.NoError(t, os.Unsetenv("MODELS_DEV_SYNC_BASE"))
		}
	})

	cacheMutex.Lock()
	etagCache = make(map[string]string)
	bodyCache = make(map[string][]byte)
	cacheMutex.Unlock()
}

func TestConvertModelsDevCatalogPrefersDirectProvider(t *testing.T) {
	catalog := &modelsDevCatalog{
		Providers: map[string]modelsDevCatalogProvider{
			"302ai": {
				ID:   "302ai",
				Name: "302.AI",
				Models: map[string]modelsDevCatalogModel{
					"gpt-5": {
						ID:     "gpt-5",
						Name:   "GPT-5 via 302",
						Status: "active",
					},
				},
			},
			"openai": {
				ID:   "openai",
				Name: "OpenAI",
				Doc:  "https://platform.openai.com/docs/models",
				Models: map[string]modelsDevCatalogModel{
					"gpt-5": {
						ID:               "gpt-5",
						Name:             "GPT-5",
						Reasoning:        true,
						ToolCall:         true,
						StructuredOutput: true,
						Attachment:       true,
						Status:           "active",
						Modalities: modelsDevCatalogModalities{
							Input:  []string{"text", "image", "pdf"},
							Output: []string{"text"},
						},
						Limit: modelsDevCatalogLimit{Context: 400000, Output: 128000},
					},
				},
			},
		},
	}

	vendors, models := convertModelsDevCatalog(catalog)

	require.Len(t, vendors, 2)
	require.Len(t, models, 1)
	require.Equal(t, "gpt-5", models[0].ModelName)
	require.Equal(t, "OpenAI", models[0].VendorName)
	require.Equal(t, "OpenAI.Color", models[0].Icon)
	require.Contains(t, models[0].Tags, "Reasoning")
	require.Contains(t, models[0].Tags, "Vision")
	require.Contains(t, models[0].Tags, "400K")
}

func TestConvertModelsDevCatalogPrefersCanonicalOwner(t *testing.T) {
	catalog := &modelsDevCatalog{
		Models: map[string]modelsDevCatalogModel{
			"openai/gpt-5.5": {
				ID:               "openai/gpt-5.5",
				Name:             "GPT-5.5",
				Reasoning:        true,
				ToolCall:         true,
				StructuredOutput: true,
				Attachment:       true,
				Status:           "active",
				Modalities: modelsDevCatalogModalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Limit: modelsDevCatalogLimit{Context: 1050000, Output: 128000},
			},
		},
		Providers: map[string]modelsDevCatalogProvider{
			"openai": {
				ID:   "openai",
				Name: "OpenAI",
				Doc:  "https://platform.openai.com/docs/models",
			},
			"vivgrid": {
				ID:   "vivgrid",
				Name: "Vivgrid",
				Doc:  "https://docs.vivgrid.com/models",
				Models: map[string]modelsDevCatalogModel{
					"gpt-5.5": {
						ID:   "gpt-5.5",
						Name: "GPT-5.5",
					},
				},
			},
		},
	}

	vendors, models := convertModelsDevCatalog(catalog)

	require.Len(t, vendors, 1)
	require.Equal(t, "OpenAI", vendors[0].Name)
	require.Equal(t, "OpenAI.Color", vendors[0].Icon)

	require.Len(t, models, 1)
	require.Equal(t, "gpt-5.5", models[0].ModelName)
	require.Equal(t, "OpenAI", models[0].VendorName)
	require.Equal(t, "OpenAI.Color", models[0].Icon)
	require.Contains(t, models[0].Tags, "Reasoning")
	require.Contains(t, models[0].Tags, "Vision")
	require.Contains(t, models[0].Tags, "1M")
}

func TestConvertModelsDevCatalogMapsDeprecatedToDisabled(t *testing.T) {
	catalog := &modelsDevCatalog{
		Providers: map[string]modelsDevCatalogProvider{
			"anthropic": {
				ID:   "anthropic",
				Name: "Anthropic",
				Models: map[string]modelsDevCatalogModel{
					"claude-3-opus-20240229": {
						ID:     "claude-3-opus-20240229",
						Name:   "Claude Opus 3",
						Status: "deprecated",
					},
				},
			},
		},
	}

	_, models := convertModelsDevCatalog(catalog)

	require.Len(t, models, 1)
	require.Equal(t, 0, models[0].Status)
}

func TestSyncUpstreamModelsCoreCreatesModelsDevCatalogModels(t *testing.T) {
	db := setupModelSyncTestDB(t)
	withModelsDevTestServer(t, `{
		"models": {
			"openai/gpt-5.5": {
				"id": "openai/gpt-5.5",
				"name": "GPT-5.5",
				"reasoning": true,
				"tool_call": true,
				"structured_output": true,
				"attachment": true,
				"modalities": {"input": ["text", "image"], "output": ["text"]},
				"limit": {"context": 1050000, "output": 128000}
			},
			"openai/old-model": {
				"id": "openai/old-model",
				"name": "Old Model",
				"status": "deprecated"
			}
		},
		"providers": {
			"openai": {
				"id": "openai",
				"name": "OpenAI",
				"doc": "https://platform.openai.com/docs/models",
				"models": {
					"gpt-5.5": {
						"id": "gpt-5.5",
						"name": "GPT-5.5"
					},
					"old-model": {
						"id": "old-model",
						"name": "Old Model"
					}
				}
			},
			"vivgrid": {
				"id": "vivgrid",
				"name": "Vivgrid",
				"doc": "https://docs.vivgrid.com/models",
				"models": {
					"gpt-5.5": {
						"id": "gpt-5.5",
						"name": "GPT-5.5"
					}
				}
			}
		}
	}`)

	result, err := syncUpstreamModelsCore(context.Background(), syncRequest{Source: syncSourceModelsDev}, syncUpstreamOptions{CreateAllUpstream: true})
	require.NoError(t, err)
	require.Equal(t, 2, result.CreatedModels)
	require.Equal(t, 1, result.CreatedVendors)
	require.Equal(t, syncSourceModelsDev, result.Source.Source)
	require.Contains(t, result.Source.CatalogURL, modelsDevCatalogPath)

	var vendor model.Vendor
	require.NoError(t, db.Where("name = ?", "OpenAI").First(&vendor).Error)
	require.Equal(t, "OpenAI.Color", vendor.Icon)

	var gpt model.Model
	require.NoError(t, db.Where("model_name = ?", "gpt-5.5").First(&gpt).Error)
	require.Equal(t, vendor.Id, gpt.VendorID)
	require.Equal(t, 1, gpt.Status)
	require.Equal(t, 1, gpt.SyncOfficial)
	require.Contains(t, gpt.Tags, "Reasoning")

	var old model.Model
	require.NoError(t, db.Where("model_name = ?", "old-model").First(&old).Error)
	require.Equal(t, 0, old.Status)

	second, err := syncUpstreamModelsCore(context.Background(), syncRequest{Source: syncSourceModelsDev}, syncUpstreamOptions{CreateAllUpstream: true})
	require.NoError(t, err)
	require.Equal(t, 0, second.CreatedModels)
	require.Empty(t, second.SkippedModels)
}

func TestSyncUpstreamModelsCoreAppliesModelsDevPricingProviderOrder(t *testing.T) {
	db := setupModelSyncTestDB(t)
	withModelsDevTestServer(t, `{
		"models": {
			"openai/gpt-sync-priced": {
				"id": "openai/gpt-sync-priced",
				"name": "GPT Sync Priced"
			}
		},
		"providers": {
			"openai": {
				"id": "openai",
				"name": "OpenAI",
				"models": {
					"gpt-sync-priced": {
						"id": "gpt-sync-priced",
						"cost": {
							"input": 2,
							"output": 8,
							"cache_read": 0.5,
							"cache_write": 2.5,
							"input_audio": 3,
							"output_audio": 12
						}
					}
				}
			},
			"azure": {
				"id": "azure",
				"name": "Azure",
				"models": {
					"gpt-sync-priced": {
						"id": "gpt-sync-priced",
						"cost": {
							"input": 1,
							"output": 3,
							"cache_read": 0.1,
							"cache_write": 1.25,
							"input_audio": 2,
							"output_audio": 10
						}
					}
				}
			}
		}
	}`)

	result, err := syncUpstreamModelsCore(context.Background(), syncRequest{
		Source: syncSourceModelsDev,
		Pricing: syncPricingPolicyRequest{
			Enabled:       true,
			ProviderOrder: []string{"azure", "openai"},
		},
	}, syncUpstreamOptions{CreateAllUpstream: true})
	require.NoError(t, err)
	require.Equal(t, 1, result.CreatedModels)
	require.Equal(t, 1, result.PricingUpdated)
	require.Contains(t, result.PricingList, "gpt-sync-priced")

	var saved model.Model
	require.NoError(t, db.Where("model_name = ?", "gpt-sync-priced").First(&saved).Error)

	requireFloatMapValue(t, ratio_setting.GetModelRatioCopy(), "gpt-sync-priced", 0.5)
	requireFloatMapValue(t, ratio_setting.GetCompletionRatioCopy(), "gpt-sync-priced", 3)
	requireFloatMapValue(t, ratio_setting.GetCacheRatioCopy(), "gpt-sync-priced", 0.1)
	requireFloatMapValue(t, ratio_setting.GetCreateCacheRatioCopy(), "gpt-sync-priced", 1.25)
	requireFloatMapValue(t, ratio_setting.GetAudioRatioCopy(), "gpt-sync-priced", 2)
	requireFloatMapValue(t, ratio_setting.GetAudioCompletionRatioCopy(), "gpt-sync-priced", 5)

	pricing := model.BuildModelPricingConfig(saved.Id, "gpt-sync-priced")
	require.NotNil(t, pricing.Source)
	require.Equal(t, model.ModelPricingSourceUpstream, pricing.Source.Kind)
	require.Equal(t, "azure", pricing.Source.Provider)
}

func TestSyncUpstreamModelsCoreKeepsManualPricingByDefault(t *testing.T) {
	_ = setupModelSyncTestDB(t)
	withModelsDevTestServer(t, `{
		"models": {
			"openai/gpt-manual-price": {
				"id": "openai/gpt-manual-price",
				"name": "GPT Manual Price"
			}
		},
		"providers": {
			"openai": {
				"id": "openai",
				"name": "OpenAI",
				"models": {
					"gpt-manual-price": {
						"id": "gpt-manual-price",
						"cost": {
							"input": 2,
							"output": 8
						}
					}
				}
			}
		}
	}`)

	local := &model.Model{
		ModelName:    "gpt-manual-price",
		Status:       1,
		SyncOfficial: 1,
		NameRule:     model.NameRuleExact,
	}
	require.NoError(t, local.Insert())
	require.NoError(t, model.SaveModelPricingConfig("gpt-manual-price", model.ModelPricingUpdateRequest{
		BillingMode:           model.ModelPricingModeRatio,
		InputPricePerMillion:  f64ptr(9),
		OutputPricePerMillion: f64ptr(18),
	}))

	result, err := syncUpstreamModelsCore(context.Background(), syncRequest{
		Source: syncSourceModelsDev,
		Pricing: syncPricingPolicyRequest{
			Enabled: true,
		},
	}, syncUpstreamOptions{CreateAllUpstream: true})
	require.NoError(t, err)
	require.Equal(t, 0, result.PricingUpdated)
	require.GreaterOrEqual(t, result.PricingSkipped, 1)
	requireFloatMapValue(t, ratio_setting.GetModelRatioCopy(), "gpt-manual-price", 4.5)
	requireFloatMapValue(t, ratio_setting.GetCompletionRatioCopy(), "gpt-manual-price", 2)
	require.Equal(t, model.ModelPricingSourceManual, model.GetModelPricingSourceCopy()["gpt-manual-price"].Kind)

	result, err = syncUpstreamModelsCore(context.Background(), syncRequest{
		Source: syncSourceModelsDev,
		Pricing: syncPricingPolicyRequest{
			Enabled:         true,
			OverwriteManual: true,
		},
	}, syncUpstreamOptions{CreateAllUpstream: true})
	require.NoError(t, err)
	require.Equal(t, 1, result.PricingUpdated)
	requireFloatMapValue(t, ratio_setting.GetModelRatioCopy(), "gpt-manual-price", 1)
	requireFloatMapValue(t, ratio_setting.GetCompletionRatioCopy(), "gpt-manual-price", 4)
	require.Equal(t, model.ModelPricingSourceUpstream, model.GetModelPricingSourceCopy()["gpt-manual-price"].Kind)
}

func TestSyncUpstreamModelsCoreKeepsLegacyOverridePricingByDefault(t *testing.T) {
	_ = setupModelSyncTestDB(t)
	withModelsDevTestServer(t, `{
		"models": {
			"openai/gpt-legacy-price": {
				"id": "openai/gpt-legacy-price",
				"name": "GPT Legacy Price"
			}
		},
		"providers": {
			"openai": {
				"id": "openai",
				"name": "OpenAI",
				"models": {
					"gpt-legacy-price": {
						"id": "gpt-legacy-price",
						"cost": {
							"input": 2,
							"output": 8
						}
					}
				}
			}
		}
	}`)

	local := &model.Model{
		ModelName:    "gpt-legacy-price",
		Status:       1,
		SyncOfficial: 1,
		NameRule:     model.NameRuleExact,
	}
	require.NoError(t, local.Insert())
	require.NoError(t, model.UpdateOption("ModelRatio", `{"gpt-legacy-price":4.5}`))
	require.NoError(t, model.UpdateOption("CompletionRatio", `{"gpt-legacy-price":2}`))
	require.Empty(t, model.GetModelPricingSourceCopy()["gpt-legacy-price"].Kind)

	result, err := syncUpstreamModelsCore(context.Background(), syncRequest{
		Source: syncSourceModelsDev,
		Pricing: syncPricingPolicyRequest{
			Enabled: true,
		},
	}, syncUpstreamOptions{CreateAllUpstream: true})
	require.NoError(t, err)
	require.Equal(t, 0, result.PricingUpdated)
	require.GreaterOrEqual(t, result.PricingSkipped, 1)
	requireFloatMapValue(t, ratio_setting.GetModelRatioCopy(), "gpt-legacy-price", 4.5)
	requireFloatMapValue(t, ratio_setting.GetCompletionRatioCopy(), "gpt-legacy-price", 2)

	result, err = syncUpstreamModelsCore(context.Background(), syncRequest{
		Source: syncSourceModelsDev,
		Pricing: syncPricingPolicyRequest{
			Enabled:         true,
			OverwriteManual: true,
		},
	}, syncUpstreamOptions{CreateAllUpstream: true})
	require.NoError(t, err)
	require.Equal(t, 1, result.PricingUpdated)
	requireFloatMapValue(t, ratio_setting.GetModelRatioCopy(), "gpt-legacy-price", 1)
	requireFloatMapValue(t, ratio_setting.GetCompletionRatioCopy(), "gpt-legacy-price", 4)
	require.Equal(t, model.ModelPricingSourceUpstream, model.GetModelPricingSourceCopy()["gpt-legacy-price"].Kind)
}

func TestSyncUpstreamModelsCoreDoesNotTreatPersistedDefaultsAsManualPricing(t *testing.T) {
	_ = setupModelSyncTestDB(t)
	withModelsDevTestServer(t, `{
		"models": {
			"openai/gpt-5-mini": {
				"id": "openai/gpt-5-mini",
				"name": "GPT-5 Mini"
			}
		},
		"providers": {
			"openai": {
				"id": "openai",
				"name": "OpenAI",
				"models": {
					"gpt-5-mini": {
						"id": "gpt-5-mini",
						"cost": {
							"input": 1,
							"output": 4
						}
					}
				}
			}
		}
	}`)

	local := &model.Model{
		ModelName:    "gpt-5-mini",
		Status:       1,
		SyncOfficial: 1,
		NameRule:     model.NameRuleExact,
	}
	require.NoError(t, local.Insert())
	require.NoError(t, model.UpdateOption("ModelRatio", ratio_setting.DefaultModelRatio2JSONString()))
	require.Empty(t, model.GetModelPricingSourceCopy()["gpt-5-mini"].Kind)
	require.NotContains(t, model.GetModelPricingOverrideModelSet(), "gpt-5-mini")

	result, err := syncUpstreamModelsCore(context.Background(), syncRequest{
		Source: syncSourceModelsDev,
		Pricing: syncPricingPolicyRequest{
			Enabled: true,
		},
	}, syncUpstreamOptions{CreateAllUpstream: true})
	require.NoError(t, err)
	require.Equal(t, 1, result.PricingUpdated)
	requireFloatMapValue(t, ratio_setting.GetModelRatioCopy(), "gpt-5-mini", 0.5)
	requireFloatMapValue(t, ratio_setting.GetCompletionRatioCopy(), "gpt-5-mini", 4)
	require.Equal(t, model.ModelPricingSourceUpstream, model.GetModelPricingSourceCopy()["gpt-5-mini"].Kind)
}

func TestSyncUpstreamModelsCoreCorrectsExistingModelsDevVendor(t *testing.T) {
	db := setupModelSyncTestDB(t)

	openaiVendor := &model.Vendor{
		Name:   "OpenAI",
		Icon:   "OpenAI.Color",
		Status: 1,
	}
	require.NoError(t, openaiVendor.Insert())

	vivgridVendor := &model.Vendor{
		Name:   "Vivgrid",
		Icon:   "Vivgrid",
		Status: 1,
	}
	require.NoError(t, vivgridVendor.Insert())

	wrongVendorModel := &model.Model{
		ModelName:    "gpt-5.5",
		VendorID:     vivgridVendor.Id,
		Status:       1,
		SyncOfficial: 1,
		NameRule:     model.NameRuleExact,
	}
	require.NoError(t, wrongVendorModel.Insert())

	existingModel := &model.Model{
		ModelName:    "old-model",
		VendorID:     openaiVendor.Id,
		Status:       0,
		SyncOfficial: 1,
		NameRule:     model.NameRuleExact,
	}
	require.NoError(t, existingModel.Insert())

	withModelsDevTestServer(t, `{
		"models": {
			"openai/gpt-5.5": {
				"id": "openai/gpt-5.5",
				"name": "GPT-5.5",
				"reasoning": true,
				"tool_call": true,
				"structured_output": true,
				"attachment": true,
				"modalities": {"input": ["text", "image"], "output": ["text"]},
				"limit": {"context": 1050000, "output": 128000}
			},
			"openai/old-model": {
				"id": "openai/old-model",
				"name": "Old Model",
				"status": "deprecated"
			}
		},
		"providers": {
			"openai": {
				"id": "openai",
				"name": "OpenAI",
				"doc": "https://platform.openai.com/docs/models"
			},
			"vivgrid": {
				"id": "vivgrid",
				"name": "Vivgrid",
				"doc": "https://docs.vivgrid.com/models"
			}
		}
	}`)

	result, err := syncUpstreamModelsCore(context.Background(), syncRequest{Source: syncSourceModelsDev}, syncUpstreamOptions{CreateAllUpstream: true})
	require.NoError(t, err)
	require.Equal(t, 0, result.CreatedModels)
	require.Equal(t, 1, result.UpdatedModels)
	require.Contains(t, result.UpdatedList, "gpt-5.5")

	var gpt model.Model
	require.NoError(t, db.Where("model_name = ?", "gpt-5.5").First(&gpt).Error)
	require.Equal(t, openaiVendor.Id, gpt.VendorID)
}

func TestSyncUpstreamModelsCoreSkipsNonOfficialVendorCorrection(t *testing.T) {
	db := setupModelSyncTestDB(t)

	openaiVendor := &model.Vendor{
		Name:   "OpenAI",
		Icon:   "OpenAI.Color",
		Status: 1,
	}
	require.NoError(t, openaiVendor.Insert())

	vivgridVendor := &model.Vendor{
		Name:   "Vivgrid",
		Icon:   "Vivgrid",
		Status: 1,
	}
	require.NoError(t, vivgridVendor.Insert())

	localModel := &model.Model{
		ModelName:    "gpt-5.5",
		VendorID:     vivgridVendor.Id,
		Status:       1,
		SyncOfficial: 0,
		NameRule:     model.NameRuleExact,
	}
	require.NoError(t, localModel.Insert())

	withModelsDevTestServer(t, `{
		"models": {
			"openai/gpt-5.5": {
				"id": "openai/gpt-5.5",
				"name": "GPT-5.5",
				"reasoning": true,
				"tool_call": true,
				"structured_output": true,
				"attachment": true,
				"modalities": {"input": ["text", "image"], "output": ["text"]},
				"limit": {"context": 1050000, "output": 128000}
			},
			"openai/old-model": {
				"id": "openai/old-model",
				"name": "Old Model",
				"status": "deprecated"
			}
		},
		"providers": {
			"openai": {
				"id": "openai",
				"name": "OpenAI",
				"doc": "https://platform.openai.com/docs/models"
			}
		}
	}`)

	result, err := syncUpstreamModelsCore(context.Background(), syncRequest{Source: syncSourceModelsDev}, syncUpstreamOptions{CreateAllUpstream: true})
	require.NoError(t, err)
	require.Equal(t, 1, result.CreatedModels)
	require.Equal(t, 0, result.UpdatedModels)

	var gpt model.Model
	require.NoError(t, db.Where("model_name = ?", "gpt-5.5").First(&gpt).Error)
	require.Equal(t, vivgridVendor.Id, gpt.VendorID)

	var old model.Model
	require.NoError(t, db.Where("model_name = ?", "old-model").First(&old).Error)
	require.Equal(t, openaiVendor.Id, old.VendorID)
}

func TestBuildSyncSourceInfoModelsDevAliases(t *testing.T) {
	info := buildSyncSourceInfo(syncRequest{Source: "models-dev"})

	require.Equal(t, syncSourceModelsDev, info.Source)
	require.Equal(t, getModelsDevCatalogURL(), info.CatalogURL)
	require.Empty(t, info.ModelsURL)
	require.Empty(t, info.VendorsURL)
}

func TestParseDailyScheduleTime(t *testing.T) {
	hour, minute, ok := parseDailyScheduleTime("02:30")
	require.True(t, ok)
	require.Equal(t, 2, hour)
	require.Equal(t, 30, minute)

	_, _, ok = parseDailyScheduleTime("24:00")
	require.False(t, ok)

	_, _, ok = parseDailyScheduleTime("2x:00")
	require.False(t, ok)
}

func TestNextDailyScheduleTime(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*3600)

	now := time.Date(2026, 6, 18, 1, 0, 0, 0, loc)
	next := nextDailyScheduleTime(now, 2, 0)
	require.Equal(t, time.Date(2026, 6, 18, 2, 0, 0, 0, loc), next)

	now = time.Date(2026, 6, 18, 3, 0, 0, 0, loc)
	next = nextDailyScheduleTime(now, 2, 0)
	require.Equal(t, time.Date(2026, 6, 19, 2, 0, 0, 0, loc), next)
}
