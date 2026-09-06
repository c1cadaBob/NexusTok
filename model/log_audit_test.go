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

	apiLogs, apiTotal, err := GetAPICallLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(2), apiTotal)
	require.Len(t, apiLogs, 2)
	require.Equal(t, []int{LogTypeError, LogTypeConsume}, []int{apiLogs[0].Type, apiLogs[1].Type})

	errorLogs, errorTotal, err := GetAPICallLogs(LogTypeError, 0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), errorTotal)
	require.Len(t, errorLogs, 1)
	require.Equal(t, LogTypeError, errorLogs[0].Type)
	require.Equal(t, float64(1), parseErrorGroupForTest(t, errorLogs[0])["count"])

	consumeOnlyLogs, consumeOnlyTotal, err := GetAPICallLogs(LogTypeConsume, 0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), consumeOnlyTotal)
	require.Len(t, consumeOnlyLogs, 1)
	require.Equal(t, LogTypeConsume, consumeOnlyLogs[0].Type)

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

func TestAPICallErrorGroupsMergeConsecutiveSameKeyAndContent(t *testing.T) {
	setupLogAuditTestDB(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		newErrorGroupTestLog(100, "req-100", "upstream 502", "candidate-a", "bad_gateway", 502),
		newErrorGroupTestLog(101, "req-101", "upstream 502", "candidate-a", "bad_gateway", 502),
		newErrorGroupTestLog(102, "req-102", "upstream 502", "candidate-a", "bad_gateway", 502),
	}).Error)

	logs, total, err := GetAPICallLogs(LogTypeError, 0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)

	errorGroup := parseErrorGroupForTest(t, logs[0])
	require.Equal(t, true, errorGroup["grouped"])
	require.Equal(t, float64(3), errorGroup["count"])
	require.Equal(t, float64(100), errorGroup["start_at"])
	require.Equal(t, float64(102), errorGroup["end_at"])
	require.Equal(t, "req-100", errorGroup["first_request_id"])
	require.Equal(t, "req-102", errorGroup["last_request_id"])
	require.Equal(t, "Upstream gateway error or timeout; check the provider, proxy, and channel health.", errorGroup["analysis"])
}

func TestAPICallErrorGroupsSplitWhenConsumeInterrupts(t *testing.T) {
	setupLogAuditTestDB(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		newErrorGroupTestLog(100, "req-100", "upstream 502", "candidate-a", "bad_gateway", 502),
		newErrorGroupTestLog(101, "req-101", "upstream 502", "candidate-a", "bad_gateway", 502),
		newConsumeGroupTestLog(102, "req-102-ok"),
		newErrorGroupTestLog(103, "req-103", "upstream 502", "candidate-a", "bad_gateway", 502),
		newErrorGroupTestLog(104, "req-104", "upstream 502", "candidate-a", "bad_gateway", 502),
	}).Error)

	logs, total, err := GetAPICallLogs(LogTypeError, 0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, logs, 2)
	require.Equal(t, float64(2), parseErrorGroupForTest(t, logs[0])["count"])
	require.Equal(t, float64(103), parseErrorGroupForTest(t, logs[0])["start_at"])
	require.Equal(t, float64(2), parseErrorGroupForTest(t, logs[1])["count"])
	require.Equal(t, float64(100), parseErrorGroupForTest(t, logs[1])["start_at"])
}

func TestAPICallErrorGroupsSplitByContentAndCredential(t *testing.T) {
	setupLogAuditTestDB(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		newErrorGroupTestLog(100, "req-100", "upstream 502", "candidate-a", "bad_gateway", 502),
		newErrorGroupTestLog(101, "req-101", "upstream 502", "candidate-a", "bad_gateway", 502),
		newErrorGroupTestLog(102, "req-102", "upstream auth", "candidate-a", "unauthorized", 401),
		newErrorGroupTestLog(103, "req-103", "upstream 502", "candidate-a", "bad_gateway", 502),
		newErrorGroupTestLog(104, "req-104", "upstream 502", "candidate-b", "bad_gateway", 502),
	}).Error)

	logs, total, err := GetAPICallLogs(LogTypeError, 0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(4), total)
	require.Len(t, logs, 4)
	require.Equal(t, float64(1), parseErrorGroupForTest(t, logs[0])["count"])
	require.Equal(t, float64(1), parseErrorGroupForTest(t, logs[1])["count"])
	require.Equal(t, float64(1), parseErrorGroupForTest(t, logs[2])["count"])
	require.Equal(t, float64(2), parseErrorGroupForTest(t, logs[3])["count"])
}

func TestUserAPICallErrorLogsKeepSummaryAndStripAdminInfo(t *testing.T) {
	setupLogAuditTestDB(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		newErrorGroupTestLog(100, "req-100", "upstream 502", "candidate-a", "bad_gateway", 502),
		newErrorGroupTestLog(101, "req-101", "upstream 502", "candidate-a", "bad_gateway", 502),
	}).Error)

	logs, total, err := GetUserAPICallLogs(1, LogTypeError, 0, 0, "", "", 0, 20, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)

	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := other["admin_info"]
	require.False(t, hasAdminInfo)
	require.Equal(t, float64(502), other["status_code"])
	require.Equal(t, "/v1/responses", other["request_path"])
	require.Equal(t, float64(2), other["error_group"].(map[string]interface{})["count"])
}

func newErrorGroupTestLog(createdAt int64, requestId string, content string, candidateId string, errorCode string, statusCode int) Log {
	return Log{
		UserId:    1,
		Username:  "alice",
		Type:      LogTypeError,
		TokenId:   7,
		TokenName: "prod-key",
		ModelName: "gpt-5.5",
		ChannelId: 27,
		Group:     "default",
		Content:   content,
		CreatedAt: createdAt,
		RequestId: requestId,
		Other: common.MapToJsonStr(map[string]interface{}{
			"request_path": "/v1/responses",
			"error_type":   "upstream_error",
			"error_code":   errorCode,
			"status_code":  statusCode,
			"admin_info": map[string]interface{}{
				"routing_candidate": map[string]interface{}{
					"candidate_id": candidateId,
				},
			},
		}),
	}
}

func newConsumeGroupTestLog(createdAt int64, requestId string) Log {
	return Log{
		UserId:    1,
		Username:  "alice",
		Type:      LogTypeConsume,
		TokenId:   7,
		TokenName: "prod-key",
		ModelName: "gpt-5.5",
		ChannelId: 27,
		Group:     "default",
		CreatedAt: createdAt,
		RequestId: requestId,
		Other: common.MapToJsonStr(map[string]interface{}{
			"request_path": "/v1/responses",
		}),
	}
}

func parseErrorGroupForTest(t *testing.T, log *Log) map[string]interface{} {
	t.Helper()
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	errorGroup, ok := other["error_group"].(map[string]interface{})
	require.True(t, ok)
	return errorGroup
}

func setupLogAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := DB
	oldLogDB := LOG_DB
	oldRedisEnabled := common.RedisEnabled
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Channel{}, &Log{}))
	DB = db
	LOG_DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	return db
}
