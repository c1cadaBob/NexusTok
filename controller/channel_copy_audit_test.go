package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelCopyAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Log{}))
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.Create(&model.User{
		Id:       1,
		Username: "root",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}).Error)

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.RedisEnabled = oldRedisEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestCopyChannelWritesStructuredAuditWithNewChannelID(t *testing.T) {
	db := setupChannelCopyAuditTestDB(t)

	origin := &model.Channel{
		Type:        constant.ChannelTypeOpenAI,
		Key:         "sk-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "primary-openai",
		Models:      "gpt-4o",
		Group:       "default",
		Balance:     12.5,
		UsedQuota:   99,
		AutoBan:     common.GetPointer(1),
		CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, origin.Insert())
	require.NotZero(t, origin.Id)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/copy/"+strconv.Itoa(origin.Id)+"?suffix=_copy", nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(origin.Id)}}
	ctx.Set("id", 1)
	ctx.Set("username", "root")
	ctx.Set("role", common.RoleRootUser)

	CopyChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyAuditLogged))

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.NotZero(t, response.Data.ID)
	require.NotEqual(t, origin.Id, response.Data.ID)

	var copied model.Channel
	require.NoError(t, db.First(&copied, "id = ?", response.Data.ID).Error)
	require.Equal(t, "primary-openai_copy", copied.Name)
	require.Equal(t, "sk-secret", copied.Key)
	require.Zero(t, copied.Balance)
	require.Zero(t, copied.UsedQuota)

	var copiedAbility model.Ability
	require.NoError(t, db.Where("channel_id = ? AND model = ? AND `group` = ?", response.Data.ID, "gpt-4o", "default").First(&copiedAbility).Error)

	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeManage).Count(&logCount).Error)
	require.EqualValues(t, 1, logCount)

	var auditLog model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeManage).First(&auditLog).Error)
	require.Equal(t, "root", auditLog.Username)
	require.Equal(t, "Copied channel "+strconv.Itoa(origin.Id)+" to primary-openai_copy (ID: "+strconv.Itoa(response.Data.ID)+")", auditLog.Content)

	other, err := common.StrToMap(auditLog.Other)
	require.NoError(t, err)
	op := other["op"].(map[string]interface{})
	require.Equal(t, "channel.copy", op["action"])
	params := op["params"].(map[string]interface{})
	require.EqualValues(t, origin.Id, params["sourceId"])
	require.EqualValues(t, response.Data.ID, params["id"])
	require.Equal(t, "primary-openai_copy", params["name"])
}
