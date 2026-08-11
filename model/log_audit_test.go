package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecordOperationAuditLogWritesStructuredOther(t *testing.T) {
	setupLogAuditTestDB(t)

	require.NoError(t, DB.Create(&User{
		Id:       1,
		Username: "root",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
	}).Error)

	RecordOperationAuditLog(OperationAuditLogParams{
		UserId:  1,
		Content: "POST /api/option/",
		Ip:      "127.0.0.1",
		Action:  "option.update",
		Params: map[string]interface{}{
			"key": "HeaderNavModules",
		},
		AdminInfo: map[string]interface{}{
			"admin_id":       1,
			"admin_username": "root",
		},
		AuditInfo: map[string]interface{}{
			"route":   "/api/option/",
			"status":  200,
			"success": true,
		},
	})

	var log Log
	require.NoError(t, LOG_DB.Where("type = ?", LogTypeManage).First(&log).Error)
	require.Equal(t, 1, log.UserId)
	require.Equal(t, "root", log.Username)
	require.Equal(t, "127.0.0.1", log.Ip)

	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	op := other["op"].(map[string]interface{})
	require.Equal(t, "option.update", op["action"])
	params := op["params"].(map[string]interface{})
	require.Equal(t, "HeaderNavModules", params["key"])
	adminInfo := other["admin_info"].(map[string]interface{})
	require.Equal(t, "root", adminInfo["admin_username"])
	auditInfo := other["audit_info"].(map[string]interface{})
	require.Equal(t, "/api/option/", auditInfo["route"])
	require.Equal(t, true, auditInfo["success"])
}

func TestRecordOperationAuditLogUsesExplicitUsername(t *testing.T) {
	setupLogAuditTestDB(t)

	RecordOperationAuditLog(OperationAuditLogParams{
		UserId:   0,
		Username: "system",
		Content:  "Ran upstream account sync system task task-1",
		Ip:       "127.0.0.1",
		Action:   "system_task.upstream_account_sync",
		Params: map[string]interface{}{
			"task_id": "task-1",
			"source":  "system_task",
			"success": true,
		},
	})

	var log Log
	require.NoError(t, LOG_DB.Where("type = ?", LogTypeManage).First(&log).Error)
	require.Equal(t, 0, log.UserId)
	require.Equal(t, "system", log.Username)
}

func TestRecordLoginLogWritesVisibleLoginMetadata(t *testing.T) {
	setupLogAuditTestDB(t)

	RecordLoginLog(LoginLogParams{
		UserId:   2,
		Username: "alice",
		Content:  "Logged in successfully via password",
		Ip:       "198.51.100.10",
		Action:   "login",
		Params: map[string]interface{}{
			"method": "password",
		},
		Extra: map[string]interface{}{
			"login_method": "password",
			"user_agent":   "NexusTok-Test/1.0",
		},
	})

	var log Log
	require.NoError(t, LOG_DB.Where("type = ?", LogTypeLogin).First(&log).Error)
	require.Equal(t, 2, log.UserId)
	require.Equal(t, "alice", log.Username)
	require.Equal(t, "198.51.100.10", log.Ip)
	require.Equal(t, "Logged in successfully via password", log.Content)

	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	require.Equal(t, "password", other["login_method"])
	require.Equal(t, "NexusTok-Test/1.0", other["user_agent"])
	op := other["op"].(map[string]interface{})
	require.Equal(t, "login", op["action"])
	params := op["params"].(map[string]interface{})
	require.Equal(t, "password", params["method"])
}

func TestLogQueriesSeparateConsumeAndAuditEntries(t *testing.T) {
	setupLogAuditTestDB(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			UserId:    1,
			Username:  "alice",
			Type:      LogTypeConsume,
			TokenId:   7,
			ModelName: "gpt-5",
			Quota:     100,
			CreatedAt: 100,
		},
		{
			UserId:    1,
			Username:  "alice",
			Type:      LogTypeTopup,
			TokenId:   7,
			CreatedAt: 101,
		},
		{
			UserId:    2,
			Username:  "root",
			Type:      LogTypeManage,
			CreatedAt: 102,
		},
		{
			UserId:    1,
			Username:  "alice",
			Type:      LogTypeLogin,
			CreatedAt: 103,
		},
		{
			UserId:    1,
			Username:  "alice",
			Type:      LogTypeError,
			CreatedAt: 104,
		},
	}).Error)

	consumeLogs, consumeTotal, err := GetConsumeLogs(0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), consumeTotal)
	require.Len(t, consumeLogs, 1)
	require.Equal(t, LogTypeConsume, consumeLogs[0].Type)

	userLogs, userTotal, err := GetUserConsumeLogs(1, 0, 0, "", "", 0, 20, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), userTotal)
	require.Len(t, userLogs, 1)
	require.Equal(t, LogTypeConsume, userLogs[0].Type)

	tokenLogs, err := GetLogByTokenId(7)
	require.NoError(t, err)
	require.Len(t, tokenLogs, 1)
	require.Equal(t, LogTypeConsume, tokenLogs[0].Type)

	auditLogs, auditTotal, err := GetAuditLogs(0, 0, "", 0, 20, "")
	require.NoError(t, err)
	require.Equal(t, int64(2), auditTotal)
	require.Len(t, auditLogs, 2)
	for _, log := range auditLogs {
		require.Contains(t, []int{LogTypeManage, LogTypeLogin}, log.Type)
	}
}

func setupLogAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := DB
	oldLogDB := LOG_DB
	oldRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))
	DB = db
	LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
	})

	return db
}
