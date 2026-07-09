// Package controller - payment_webhook_availability_test.go
// 该文件包含支付 Webhook 可用性检查的单元测试
//
// 测试内容包括：
// - Stripe Webhook 启用条件检查
// - Creem Webhook 启用条件检查
// - Waffo Webhook 启用条件检查
// - Waffo Pancake Webhook 启用条件检查
// - EPay Webhook 启用条件检查
package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func confirmPaymentComplianceForTest(t *testing.T) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
}

func TestStripeWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	originalPriceID := setting.StripePriceId
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
		setting.StripePriceId = originalPriceID
	})

	setting.StripeWebhookSecret = ""
	setting.StripeApiSecret = "sk_test_123"
	setting.StripePriceId = "price_123"
	require.False(t, isStripeWebhookEnabled())

	setting.StripeWebhookSecret = "whsec_test"
	require.True(t, isStripeWebhookEnabled())

	setting.StripePriceId = ""
	require.False(t, isStripeWebhookEnabled())
}

func TestCreemWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPIKey := setting.CreemApiKey
	originalProducts := setting.CreemProducts
	originalWebhookSecret := setting.CreemWebhookSecret
	t.Cleanup(func() {
		setting.CreemApiKey = originalAPIKey
		setting.CreemProducts = originalProducts
		setting.CreemWebhookSecret = originalWebhookSecret
	})

	setting.CreemWebhookSecret = ""
	setting.CreemApiKey = "creem_api_key"
	setting.CreemProducts = `[{"productId":"prod_123"}]`
	require.False(t, isCreemWebhookEnabled())

	setting.CreemWebhookSecret = "creem_secret"
	require.True(t, isCreemWebhookEnabled())

	setting.CreemProducts = "[]"
	require.False(t, isCreemWebhookEnabled())
}

func TestWaffoWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalEnabled := setting.WaffoEnabled
	originalSandbox := setting.WaffoSandbox
	originalAPIKey := setting.WaffoApiKey
	originalPrivateKey := setting.WaffoPrivateKey
	originalPublicCert := setting.WaffoPublicCert
	originalSandboxAPIKey := setting.WaffoSandboxApiKey
	originalSandboxPrivateKey := setting.WaffoSandboxPrivateKey
	originalSandboxPublicCert := setting.WaffoSandboxPublicCert
	t.Cleanup(func() {
		setting.WaffoEnabled = originalEnabled
		setting.WaffoSandbox = originalSandbox
		setting.WaffoApiKey = originalAPIKey
		setting.WaffoPrivateKey = originalPrivateKey
		setting.WaffoPublicCert = originalPublicCert
		setting.WaffoSandboxApiKey = originalSandboxAPIKey
		setting.WaffoSandboxPrivateKey = originalSandboxPrivateKey
		setting.WaffoSandboxPublicCert = originalSandboxPublicCert
	})

	setting.WaffoEnabled = true
	setting.WaffoSandbox = false
	setting.WaffoApiKey = ""
	setting.WaffoPrivateKey = "private"
	setting.WaffoPublicCert = "public"
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoApiKey = "api"
	require.True(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = false
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = true
	setting.WaffoSandbox = true
	setting.WaffoSandboxApiKey = ""
	setting.WaffoSandboxPrivateKey = "sandbox_private"
	setting.WaffoSandboxPublicCert = "sandbox_public"
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoSandboxApiKey = "sandbox_api"
	require.True(t, isWaffoWebhookEnabled())
}

func TestWaffoPancakeWebhookEnabledRequiresRuntimeAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalEnabled := setting.WaffoPancakeEnabled
	originalSandbox := setting.WaffoPancakeSandbox
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalWebhookPublicKey := setting.WaffoPancakeWebhookPublicKey
	originalWebhookTestKey := setting.WaffoPancakeWebhookTestKey
	originalStoreID := setting.WaffoPancakeStoreID
	originalProductID := setting.WaffoPancakeProductID
	t.Cleanup(func() {
		setting.WaffoPancakeEnabled = originalEnabled
		setting.WaffoPancakeSandbox = originalSandbox
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		setting.WaffoPancakeWebhookPublicKey = originalWebhookPublicKey
		setting.WaffoPancakeWebhookTestKey = originalWebhookTestKey
		setting.WaffoPancakeStoreID = originalStoreID
		setting.WaffoPancakeProductID = originalProductID
	})

	setting.WaffoPancakeEnabled = true
	setting.WaffoPancakeSandbox = false
	setting.WaffoPancakeMerchantID = "merchant"
	setting.WaffoPancakePrivateKey = "private"
	setting.WaffoPancakeStoreID = ""
	setting.WaffoPancakeProductID = ""
	setting.WaffoPancakeWebhookPublicKey = ""
	require.False(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeWebhookPublicKey = "public"
	require.True(t, isWaffoPancakeWebhookEnabled())
	require.False(t, isWaffoPancakeTopUpEnabled())

	setting.WaffoPancakeStoreID = "store"
	setting.WaffoPancakeProductID = "product"
	require.True(t, isWaffoPancakeTopUpEnabled())

	setting.WaffoPancakeEnabled = false
	require.False(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeEnabled = true
	setting.WaffoPancakeSandbox = true
	setting.WaffoPancakeWebhookTestKey = ""
	require.False(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeWebhookTestKey = "test_public"
	require.True(t, isWaffoPancakeWebhookEnabled())
}

func TestGetTopUpInfoExposesWaffoPancakeSubscriptionSwitchSeparately(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalEnabled := setting.WaffoPancakeEnabled
	originalSandbox := setting.WaffoPancakeSandbox
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalWebhookPublicKey := setting.WaffoPancakeWebhookPublicKey
	originalStoreID := setting.WaffoPancakeStoreID
	originalProductID := setting.WaffoPancakeProductID
	t.Cleanup(func() {
		setting.WaffoPancakeEnabled = originalEnabled
		setting.WaffoPancakeSandbox = originalSandbox
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		setting.WaffoPancakeWebhookPublicKey = originalWebhookPublicKey
		setting.WaffoPancakeStoreID = originalStoreID
		setting.WaffoPancakeProductID = originalProductID
	})

	setting.WaffoPancakeEnabled = true
	setting.WaffoPancakeSandbox = false
	setting.WaffoPancakeMerchantID = "merchant"
	setting.WaffoPancakePrivateKey = "private"
	setting.WaffoPancakeWebhookPublicKey = "public"
	setting.WaffoPancakeStoreID = ""
	setting.WaffoPancakeProductID = ""

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup", nil)

	GetTopUpInfo(ctx)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			EnableWaffoPancakeTopUp        bool `json:"enable_waffo_pancake_topup"`
			EnableWaffoPancakeSubscription bool `json:"enable_waffo_pancake_subscription"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.False(t, response.Data.EnableWaffoPancakeTopUp)
	require.True(t, response.Data.EnableWaffoPancakeSubscription)
}

func TestEpayWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
	})

	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = ""
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	require.False(t, isEpayWebhookEnabled())

	operation_setting.EpayKey = "epay_key"
	require.True(t, isEpayWebhookEnabled())

	operation_setting.PayMethods = nil
	require.False(t, isEpayWebhookEnabled())
}
