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
	}
	logs := []*Log{{Other: common.MapToJsonStr(other)}}

	formatUserLogs(logs, 0)

	formatted, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	require.Equal(t, 1, logs[0].Id)
	require.Equal(t, 0.004, formatted["model_price"])
	_, hasAdminInfo := formatted["admin_info"]
	require.False(t, hasAdminInfo)
}
