package controller

import (
	"strings"
	"testing"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/pkg/billingexpr"
	"github.com/c1cadaBob/NexusTok/relaykit/dto"
	"github.com/c1cadaBob/NexusTok/setting/billing_setting"
	"github.com/c1cadaBob/NexusTok/setting/config"
	"github.com/c1cadaBob/NexusTok/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
)

func TestPricingSyncExpressionPriority(t *testing.T) {
	expression := `tier("base", p * 2 + c * 8 + cr * 0)`
	cases := []struct {
		name       string
		local      map[string]any
		source     map[string]any
		wantFields []string
	}{
		{"equal expressions suppress stale ratios", map[string]any{"billing_mode": map[string]string{"m": "tiered_expr"}, "billing_expr": map[string]string{"m": expression}}, map[string]any{"billing_mode": map[string]string{"m": "tiered_expr"}, "billing_expr": map[string]string{"m": expression}, "model_ratio": map[string]float64{"m": 3}, "model_price": map[string]float64{"m": 2}}, nil},
		{"local expression excludes legacy-only source", map[string]any{"billing_mode": map[string]string{"m": "tiered_expr"}, "billing_expr": map[string]string{"m": expression}}, map[string]any{"model_ratio": map[string]float64{"m": 3}, "completion_ratio": map[string]float64{"m": 2}}, nil},
		{"expression imports without legacy conflicts", map[string]any{"model_ratio": map[string]float64{"m": 1}}, map[string]any{"billing_mode": map[string]string{"m": "tiered_expr"}, "billing_expr": map[string]string{"m": expression}, "model_ratio": map[string]float64{"m": 3}, "model_price": map[string]float64{"m": 2}}, []string{"billing_mode", "billing_expr"}},
		{"inactive expression follows explicit ratio mode", map[string]any{"model_ratio": map[string]float64{"m": 1}}, map[string]any{"billing_mode": map[string]string{"m": "ratio"}, "billing_expr": map[string]string{"m": expression}, "model_ratio": map[string]float64{"m": 3}}, []string{"model_ratio"}},
		{"empty active expression never imports a false free price", map[string]any{}, map[string]any{"billing_mode": map[string]string{"m": "tiered_expr"}, "billing_expr": map[string]string{"m": " "}, "model_ratio": map[string]float64{"m": 0}}, nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			diff := buildDifferences(tt.local, []struct {
				name string
				data map[string]any
			}{{"source", tt.source}})
			fields := make([]string, 0, len(diff["m"]))
			for field := range diff["m"] {
				fields = append(fields, field)
			}
			assert.ElementsMatch(t, tt.wantFields, fields)
		})
	}
}

func TestRatioConfigExportsEffectiveExpressions(t *testing.T) {
	before := config.GlobalConfig.ExportAllConfigs()
	expose := ratio_setting.IsExposeRatioEnabled()
	t.Cleanup(func() {
		config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": before["billing_setting.billing_mode"], "billing_expr": before["billing_setting.billing_expr"]})
		ratio_setting.SetExposeRatioEnabled(expose)
	})
	config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": `{"sync-export":"tiered_expr"}`, "billing_expr": `{"sync-export":"tier(\"base\", p * 2)"}`})
	ratio_setting.SetExposeRatioEnabled(true)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ratio_config", nil)
	GetRatioConfig(c)
	var response struct {
		Success bool
		Data    map[string]any
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, billing_setting.BillingModeTieredExpr, valueMap(response.Data["billing_mode"])["sync-export"])
	assert.Equal(t, `tier("base", p * 2)`, valueMap(response.Data["billing_expr"])["sync-export"])
}

func TestPricingSyncCompleteSourcesAndArrayFormats(t *testing.T) {
	before := config.GlobalConfig.ExportAllConfigs()
	oldRatios, oldCompletion := ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
	t.Cleanup(func() {
		config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": before["billing_setting.billing_mode"], "billing_expr": before["billing_setting.billing_expr"]})
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"sync-token":1}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"sync-token":2}`))
	expression := `len <= 200000 ? tier("short", p * 2 + c * 8 + cr * 0) : tier("long", p * 4 + c * 12)`
	expressions, err := common.Marshal(map[string]string{"sync-already": expression})
	require.NoError(t, err)
	config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": `{"sync-already":"tiered_expr"}`, "billing_expr": string(expressions)})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		if r.URL.Path == "/ratio_config" {
			data = map[string]any{
				"billing_mode":     map[string]string{"sync-already": "tiered_expr", "sync-expression": "tiered_expr"},
				"billing_expr":     map[string]string{"sync-already": expression, "sync-expression": expression},
				"model_ratio":      map[string]float64{"sync-already": 9, "sync-expression": 9, "sync-token": 1},
				"model_price":      map[string]float64{"sync-expression": 4},
				"completion_ratio": map[string]float64{"sync-token": 4},
				"cache_ratio":      map[string]float64{"sync-token": 0},
			}
		} else {
			data = []map[string]any{
				{"model_name": "sync-already", "model_ratio": 5, "completion_ratio": 3},
				{"model_name": "sync-expression", "model_ratio": 2, "model_price": 1},
				{"model_name": "sync-array-expression", "billing_mode": "tiered_expr", "billing_expr": expression, "quota_type": 1, "model_price": 0},
				{"model_name": "sync-unpriced"},
				{"model_name": "sync-invalid-expression", "billing_mode": "tiered_expr", "billing_expr": "", "model_ratio": 0},
				{"model_name": "sync-free", "model_ratio": 0, "completion_ratio": 0},
			}
		}
		encoded, err := common.Marshal(map[string]any{"success": true, "data": data})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encoded)
	}))
	defer server.Close()
	var response struct {
		Success bool
		Data    struct {
			Differences map[string]map[string]dto.DifferenceItem
			Prices      map[string]struct {
				Current   map[string]any
				Upstreams map[string]map[string]any
			}
			TestResults []dto.TestResult `json:"test_results"`
		}
	}
	body := map[string]any{"upstreams": []map[string]any{
		{"id": 1, "name": "Expressions", "base_url": server.URL, "endpoint": "/ratio_config"},
		{"id": 2, "name": "Legacy", "base_url": server.URL, "endpoint": "/pricing"},
	}}
	recorder := modelManagementRequest(t, FetchUpstreamRatios, http.MethodPost, "/api/channel/fetch_upstream_ratios", body, &response)
	require.True(t, response.Success, recorder.Body.String())
	require.Len(t, response.Data.TestResults, 2)
	for _, result := range response.Data.TestResults {
		require.Equal(t, "success", result.Status, result.Error)
	}
	assert.NotContains(t, response.Data.Differences, "sync-already")
	assert.NotContains(t, response.Data.Differences, "sync-unpriced")
	assert.NotContains(t, response.Data.Differences, "sync-invalid-expression")
	assert.Equal(t, map[string]any{"billing_mode": "tiered_expr", "billing_expr": expression}, response.Data.Prices["sync-expression"].Upstreams["Expressions(1)"])
	assert.NotContains(t, response.Data.Prices["sync-expression"].Upstreams, "Legacy(2)")
	assert.Equal(t, map[string]any{"billing_mode": "tiered_expr", "billing_expr": expression}, response.Data.Prices["sync-array-expression"].Upstreams["Legacy(2)"])
	assert.Equal(t, float64(1), response.Data.Prices["sync-token"].Upstreams["Expressions(1)"]["model_ratio"], "unchanged base prices are included for a complete price preview")
	assert.Equal(t, float64(4), response.Data.Prices["sync-token"].Upstreams["Expressions(1)"]["completion_ratio"])
	assert.Equal(t, float64(0), response.Data.Prices["sync-token"].Upstreams["Expressions(1)"]["cache_ratio"])
	assert.Equal(t, float64(0), response.Data.Prices["sync-free"].Upstreams["Legacy(2)"]["model_ratio"])
}

func convertModelsDevJSONForTest(t *testing.T, body string) map[string]any {
	t.Helper()
	converted, err := convertModelsDevToRatioData(strings.NewReader(body))
	require.NoError(t, err)
	return converted
}

func TestModelsDevPricingPrefersOfficialProviderAndBuildsContextExpression(t *testing.T) {
	converted := convertModelsDevJSONForTest(t, `{
		"unorouter": {
			"models": {
				"gpt-5.5": {
					"id": "gpt-5.5",
					"cost": {"input": 0.1875, "output": 1.125}
				}
			}
		},
		"openai": {
			"models": {
				"gpt-5.5": {
					"id": "gpt-5.5",
					"cost": {
						"input": 5,
						"output": 30,
						"cache_read": 0.5,
						"tiers": [
							{
								"input": 10,
								"output": 45,
								"cache_read": 1,
								"tier": {"type": "context", "size": 272000}
							}
						],
						"context_over_200k": {"input": 10, "output": 45, "cache_read": 1}
					}
				}
			}
		}
	}`)

	assert.NotContains(t, valueMap(converted["model_ratio"]), "gpt-5.5")
	assert.Equal(t, billing_setting.BillingModeTieredExpr, valueMap(converted[billing_setting.BillingModeField])["gpt-5.5"])
	expression, ok := valueMap(converted[billing_setting.BillingExprField])["gpt-5.5"].(string)
	require.True(t, ok)
	assert.Equal(t, `len <= 200000 ? tier("standard", p * 5 + c * 30 + cr * 0.5) : tier("long_context", p * 10 + c * 45 + cr * 1)`, expression)

	standardCost, standardTrace, err := billingexpr.RunExpr(expression, billingexpr.TokenParams{P: 100, C: 10, CR: 20, Len: 200000})
	require.NoError(t, err)
	assert.Equal(t, "standard", standardTrace.MatchedTier)
	assert.InEpsilon(t, 810, standardCost, 1e-9)

	longCost, longTrace, err := billingexpr.RunExpr(expression, billingexpr.TokenParams{P: 100, C: 10, CR: 20, Len: 200001})
	require.NoError(t, err)
	assert.Equal(t, "long_context", longTrace.MatchedTier)
	assert.InEpsilon(t, 1470, longCost, 1e-9)
}

func TestModelsDevPricingPrefersProviderQualifiedIDBeforeBareID(t *testing.T) {
	converted := convertModelsDevJSONForTest(t, `{
		"Open AI": {
			"models": {
				"gpt-5.5": {
					"id": "gpt-5.5",
					"cost": {"input": 1, "output": 2, "cache_read": 0.1}
				},
				"openai/gpt-5.5": {
					"id": "openai/gpt-5.5",
					"cost": {"input": 5, "output": 30, "cache_read": 0.5}
				}
			}
		}
	}`)

	assert.Equal(t, float64(2.5), valueMap(converted["model_ratio"])["gpt-5.5"])
	assert.Equal(t, float64(6), valueMap(converted["completion_ratio"])["gpt-5.5"])
	assert.Equal(t, float64(0.1), valueMap(converted["cache_ratio"])["gpt-5.5"])
	assert.NotContains(t, valueMap(converted[billing_setting.BillingModeField]), "gpt-5.5")
	assert.NotContains(t, valueMap(converted[billing_setting.BillingExprField]), "gpt-5.5")
}

func TestModelsDevPricingFallsBackToBareOfficialModelID(t *testing.T) {
	converted := convertModelsDevJSONForTest(t, `{
		"unorouter": {
			"models": {
				"gpt-5.5": {
					"id": "gpt-5.5",
					"cost": {"input": 0.1875, "output": 1.125}
				}
			}
		},
		"openai": {
			"models": {
				"gpt-5.5": {
					"id": "gpt-5.5",
					"cost": {"input": 5, "output": 30, "cache_read": 0.5}
				}
			}
		}
	}`)

	assert.Equal(t, float64(2.5), valueMap(converted["model_ratio"])["gpt-5.5"])
	assert.Equal(t, float64(6), valueMap(converted["completion_ratio"])["gpt-5.5"])
	assert.Equal(t, float64(0.1), valueMap(converted["cache_ratio"])["gpt-5.5"])
}

func TestModelsDevPricingUsesTokenRatiosWithoutTiers(t *testing.T) {
	converted := convertModelsDevJSONForTest(t, `{
		"custom": {
			"models": {
				"plain-model": {
					"id": "plain-model",
					"cost": {
						"input": 5,
						"output": 30,
						"cache_read": 0.5,
						"cache_write": 10,
						"input_audio": 20,
						"output_audio": 80
					}
				}
			}
		}
	}`)

	assert.Equal(t, float64(2.5), valueMap(converted["model_ratio"])["plain-model"])
	assert.Equal(t, float64(6), valueMap(converted["completion_ratio"])["plain-model"])
	assert.Equal(t, float64(0.1), valueMap(converted["cache_ratio"])["plain-model"])
	assert.Equal(t, float64(2), valueMap(converted["create_cache_ratio"])["plain-model"])
	assert.Equal(t, float64(4), valueMap(converted["audio_ratio"])["plain-model"])
	assert.Equal(t, float64(4), valueMap(converted["audio_completion_ratio"])["plain-model"])
	assert.Empty(t, valueMap(converted[billing_setting.BillingModeField]))
	assert.Empty(t, valueMap(converted[billing_setting.BillingExprField]))
}

func TestModelsDevPricingBuildsGenericContextTiers(t *testing.T) {
	converted := convertModelsDevJSONForTest(t, `{
		"custom": {
			"models": {
				"context-model": {
					"id": "context-model",
					"cost": {
						"input": 1,
						"output": 2,
						"tiers": [
							{"input": 5, "output": 6, "tier": {"type": "context", "size": 128000}},
							{"input": 3, "output": 4, "tier": {"type": "context", "size": 32000}}
						]
					}
				}
			}
		}
	}`)

	expression, ok := valueMap(converted[billing_setting.BillingExprField])["context-model"].(string)
	require.True(t, ok)
	assert.Equal(t, `len <= 32000 ? tier("standard", p * 1 + c * 2) : len <= 128000 ? tier("context_over_32000", p * 3 + c * 4) : tier("context_over_128000", p * 5 + c * 6)`, expression)
	_, _, err := billingexpr.RunExpr(expression, billingexpr.TokenParams{P: 1, C: 1, Len: 128001})
	require.NoError(t, err)
}

func TestModelsDevPricingSkipsInvalidCostsAndKeepsFreeModels(t *testing.T) {
	converted := convertModelsDevJSONForTest(t, `{
		"custom": {
			"models": {
				"bad-negative": {"id": "bad-negative", "cost": {"input": -1, "output": 2}},
				"bad-cache": {"id": "bad-cache", "cost": {"input": 1, "cache_read": -0.1}},
				"bad-missing": {"id": "bad-missing", "cost": {}},
				"bad-zero": {"id": "bad-zero", "cost": {"input": 0, "output": 1}},
				"good-free": {"id": "good-free", "cost": {"input": 0, "output": 0, "cache_read": 0}}
			}
		}
	}`)

	modelRatios := valueMap(converted["model_ratio"])
	assert.Equal(t, float64(0), modelRatios["good-free"])
	assert.NotContains(t, modelRatios, "bad-negative")
	assert.NotContains(t, modelRatios, "bad-cache")
	assert.NotContains(t, modelRatios, "bad-missing")
	assert.NotContains(t, modelRatios, "bad-zero")
}
