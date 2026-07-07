package service

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	relaycommon "github.com/c1cada/NexusTok/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAttachQuotaSaturationNestsUnderAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{
		UserId:          7,
		OriginModelName: "gpt-image-1",
		QuotaClamp: &common.QuotaClamp{
			Op:       "QuotaFromDecimal",
			Kind:     common.QuotaClampOverflow,
			Original: 1.8e19,
			Clamped:  common.MaxQuota,
		},
	}

	other := map[string]interface{}{"model_price": 0.004}
	AttachQuotaSaturation(ctx, relayInfo, other)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	saturation, ok := adminInfo["quota_saturation"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "QuotaFromDecimal", saturation["op"])
	require.Equal(t, common.QuotaClampOverflow, saturation["kind"])
	require.Equal(t, common.MaxQuota, saturation["clamped"])
}

func TestAttachQuotaSaturationPreservesExistingAdminInfo(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		QuotaClamp: &common.QuotaClamp{
			Op:      "QuotaFromFloat",
			Kind:    common.QuotaClampUnderflow,
			Clamped: common.MinQuota,
		},
	}
	other := map[string]interface{}{
		"admin_info": map[string]interface{}{"admin_username": "root"},
	}

	AttachQuotaSaturation(nil, relayInfo, other)

	adminInfo := other["admin_info"].(map[string]interface{})
	require.Equal(t, "root", adminInfo["admin_username"])
	require.NotNil(t, adminInfo["quota_saturation"])
}

func TestAttachQuotaSaturationNoClampNoMarker(t *testing.T) {
	other := map[string]interface{}{"model_price": 0.004}
	AttachQuotaSaturation(nil, &relaycommon.RelayInfo{}, other)

	_, exists := other["admin_info"]
	require.False(t, exists)
}
