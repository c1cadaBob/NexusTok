package kling

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
)

func parseKlingTaskResultForTest(t *testing.T, finalUnitDeduction string) int {
	t.Helper()

	body := `{
		"code": 0,
		"data": {
			"task_id": "kling-task-1",
			"task_status": "succeed",
			"task_result": {
				"videos": [
					{"id": "video-1", "url": "https://example.com/video.mp4"}
				]
			},
			"final_unit_deduction": "` + finalUnitDeduction + `"
		}
	}`

	info, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, info.Status)
	return info.TotalTokens
}

func TestParseTaskResultRoundsFinalUnitDeductionUp(t *testing.T) {
	tokens := parseKlingTaskResultForTest(t, "1.2")

	require.Equal(t, 2, tokens)
}

func TestParseTaskResultSaturatesHugeFinalUnitDeduction(t *testing.T) {
	tokens := parseKlingTaskResultForTest(t, "1e100")

	require.Equal(t, common.MaxQuota, tokens)
}

func TestParseTaskResultIgnoresNaNFinalUnitDeduction(t *testing.T) {
	tokens := parseKlingTaskResultForTest(t, "NaN")

	require.Zero(t, tokens)
}
