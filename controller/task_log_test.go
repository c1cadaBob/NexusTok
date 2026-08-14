package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTaskLogControllerTestDB(t *testing.T) {
	t.Helper()
	originDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Task{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = originDB
	})
}

func TestGetAllTaskIncludesSystemChannelNames(t *testing.T) {
	setupTaskLogControllerTestDB(t)

	channel := model.Channel{
		Name: "同步渠道-A",
		Key:  "channel-key",
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	task := &model.Task{
		CreatedAt:  100,
		UpdatedAt:  100,
		TaskID:     "systask_task_log_test",
		Platform:   constant.TaskPlatformSystem,
		UserId:     0,
		ChannelId:  channel.Id,
		Action:     constant.TaskActionUpstreamAccountSync,
		Status:     model.TaskStatusSkipped,
		FailReason: "渠道已禁用，跳过上游账号自动同步",
		SubmitTime: 100,
		StartTime:  100,
		FinishTime: 100,
		Progress:   "100%",
	}
	task.SetData(map[string]any{
		"type":           "upstream_account_sync",
		"system_task_id": "systask_task_log_test",
		"channel_id":     channel.Id,
		"channel_name":   channel.Name,
		"status":         "SKIPPED",
		"skip_reason":    "渠道已禁用，跳过上游账号自动同步",
	})
	require.NoError(t, model.DB.Create(task).Error)

	router := gin.New()
	router.GET("/api/task/", GetAllTask)

	req := httptest.NewRequest(http.MethodGet, "/api/task/?p=1&page_size=20&task_id=systask_task_log_test", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Items []dto.TaskDto `json:"items"`
			Total int           `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &result))
	require.True(t, result.Success)
	require.Equal(t, 1, result.Data.Total)
	require.Len(t, result.Data.Items, 1)
	require.Equal(t, channel.Name, result.Data.Items[0].ChannelName)
	require.Equal(t, "System", result.Data.Items[0].Username)
	require.Equal(t, constant.TaskPlatformSystem, constant.TaskPlatform(result.Data.Items[0].Platform))
}
