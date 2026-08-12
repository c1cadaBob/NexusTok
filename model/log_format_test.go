package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/require"
)

func TestFormatUserLogsStripsQuotaSaturationAdminInfo(t *testing.T) {
	other := map[string]interface{}{
		"model_price": 0.004,
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromFloat",
				"kind":    common.QuotaClampOverflow,
				"clamped": common.MaxQuota,
			},
		},
		"audit_info": map[string]interface{}{
			"route":   "/api/option/",
			"success": true,
		},
	}
	logs := []*Log{{Other: common.MapToJsonStr(other)}}

	formatUserLogs(logs, 0)

	formatted, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	require.Equal(t, 1, logs[0].Id)
	require.Equal(t, 0.004, formatted["model_price"])
	_, hasAdminInfo := formatted["admin_info"]
	require.False(t, hasAdminInfo)
	_, hasAuditInfo := formatted["audit_info"]
	require.False(t, hasAuditInfo)
}

func TestFormatUserLogsStripsChannelAccountAdminInfo(t *testing.T) {
	other := map[string]interface{}{
		"model_ratio": 1.25,
		"admin_info": map[string]interface{}{
			"account_pool":           true,
			"channel_account_id":     18,
			"channel_account_name":   "c1cada",
			"ratio_conversion":       0.35,
			"standard_billing_quota": 1200,
			"use_channel":            []int{18},
		},
	}
	logs := []*Log{{Other: common.MapToJsonStr(other)}}

	formatUserLogs(logs, 0)

	formatted, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	require.Equal(t, 1.25, formatted["model_ratio"])
	_, hasAdminInfo := formatted["admin_info"]
	require.False(t, hasAdminInfo)
}

func TestBuildTopupAdminInfoMergesExtraAudit(t *testing.T) {
	extra := map[string]interface{}{
		"quota_saturation": (&common.QuotaClamp{
			Op:      "QuotaFromDecimalTruncated",
			Kind:    common.QuotaClampOverflow,
			Clamped: common.MaxQuota,
		}).AuditMap(),
	}

	adminInfo := buildTopupAdminInfo("127.0.0.1", "epay", "epay", extra)

	require.Equal(t, "127.0.0.1", adminInfo["caller_ip"])
	require.Equal(t, "epay", adminInfo["payment_method"])
	require.Equal(t, "epay", adminInfo["callback_payment_method"])
	require.NotNil(t, adminInfo["quota_saturation"])
}
