package ali

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/c1cada/NexusTok/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestEstimateBillingCapsMetadataDuration 验证 metadata 覆盖的 Ali 时长作为计费倍率前会被钳制。
func TestEstimateBillingCapsMetadataDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt: "一只猫在城市中奔跑",
		Model:  "wan2.6-i2v",
		Metadata: map[string]any{
			"parameters": map[string]any{
				"duration": relaycommon.MaxTaskDurationSeconds + 1,
			},
		},
	})

	ratios := (&TaskAdaptor{}).EstimateBilling(c, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	})

	require.NotNil(t, ratios)
	require.Equal(t, float64(relaycommon.MaxTaskDurationSeconds), ratios["seconds"])
}
