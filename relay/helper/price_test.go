// 本文件是 relay/helper 包中 ModelPriceHelper 函数的单元测试集。
// 测试了分层计费（Tiered Billing）表达式场景下的预加载请求输入使用、
// tier 估算和配额预消费计算等功能。
package helper

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/pkg/billingexpr"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/setting/billing_setting"
	"github.com/c1cada/NexusTok/setting/config"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestModelPriceHelperTieredUsesPreloadedRequestInput 测试分层计费模式下使用预加载的请求输入，
// 验证 tier 估算（stream 模式）和配额预消费计算（p*3=1500）的正确性。
func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}

// TestModelPriceHelperSaturatesRatioPreConsume 验证标准 Token 倍率预扣在异常大倍率下饱和到 int32 上限。
func TestModelPriceHelperSaturatesRatioPreConsume(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreRatioSettings(t)

	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"quota-saturation-ratio":1.8446744073686647e19}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))

	ctx, info := newPriceTestContext("quota-saturation-ratio")
	priceData, err := ModelPriceHelper(ctx, info, 2000, &types.TokenCountMeta{})

	require.NoError(t, err)
	require.Equal(t, math.MaxInt32, priceData.QuotaToPreConsume)
	require.Equal(t, math.MaxInt32, info.PriceData.QuotaToPreConsume)
}

// TestModelPriceHelperPerCallSaturatesFixedPrice 验证按次固定价格预扣在异常大价格下饱和到 int32 上限。
func TestModelPriceHelperPerCallSaturatesFixedPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreRatioSettings(t)

	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"quota-saturation-per-call":1.8446744073686647e19}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))

	ctx, info := newPriceTestContext("quota-saturation-per-call")
	priceData, err := ModelPriceHelperPerCall(ctx, info)

	require.NoError(t, err)
	require.Equal(t, math.MaxInt32, priceData.Quota)
}

func TestModelPriceNotConfiguredErrorPointsAdminsToModelsPage(t *testing.T) {
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:price_error_admin?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
	})

	admin := model.User{
		Username:    "price-admin",
		Password:    "password",
		DisplayName: "price-admin",
		Role:        common.RoleAdminUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "price-admin",
	}
	require.NoError(t, model.DB.Create(&admin).Error)
	t.Cleanup(func() {
		_ = model.DB.Delete(&model.User{}, admin.Id).Error
	})

	err = modelPriceNotConfiguredError("gpt-5.5", admin.Id)

	require.Error(t, err)
	require.Contains(t, err.Error(), "模型 → 同步源模型")
	require.Contains(t, err.Error(), "Models → Sync Source Models")
	require.NotContains(t, err.Error(), "分组与模型"+"定价设置")
	require.NotContains(t, err.Error(), "Group & Model"+" Pricing")
}

// restoreRatioSettings 保存并恢复全局 ratio 配置，避免本文件的极端倍率污染其它测试。
func restoreRatioSettings(t *testing.T) {
	t.Helper()

	modelRatio := ratio_setting.ModelRatio2JSONString()
	modelPrice := ratio_setting.ModelPrice2JSONString()
	groupRatio := ratio_setting.GroupRatio2JSONString()
	groupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()

	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(modelRatio))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(modelPrice))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groupRatio))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(groupGroupRatio))
	})
}

func newPriceTestContext(modelName string) (*gin.Context, *relaycommon.RelayInfo) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("group", "default")

	return ctx, &relaycommon.RelayInfo{
		OriginModelName: modelName,
		UserGroup:       "default",
		UsingGroup:      "default",
	}
}
