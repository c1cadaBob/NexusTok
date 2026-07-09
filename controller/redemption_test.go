package controller

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type redemptionPageResponse struct {
	Items []redemptionResponse `json:"items"`
	Total int                  `json:"total"`
}

func setupRedemptionControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Redemption{}, &model.Log{}, &model.User{}))
	require.NoError(t, db.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&model.Redemption{}).Error)
	require.NoError(t, db.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&model.Log{}).Error)
	require.NoError(t, db.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&model.User{}).Error)
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	return db
}

func seedControllerRedemption(t *testing.T, db *gorm.DB, key string) *model.Redemption {
	t.Helper()
	redemption := &model.Redemption{
		UserId:      1,
		Name:        "controller-redemption",
		Key:         key,
		Status:      common.RedemptionCodeStatusEnabled,
		Quota:       1000,
		CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, db.Create(redemption).Error)
	return redemption
}

func TestRedemptionReadResponsesMaskKey(t *testing.T) {
	db := setupRedemptionControllerTestDB(t)
	rawKey := "1234567890abcdef1234567890abcdef"
	redemption := seedControllerRedemption(t, db, rawKey)
	maskedKey := redemption.GetMaskedKey()

	listCtx, listRecorder := newAuthenticatedContext(t, http.MethodGet, "/api/redemption/?p=1&page_size=10", nil, 1)
	GetAllRedemptions(listCtx)

	listResponse := decodeAPIResponse(t, listRecorder)
	require.True(t, listResponse.Success, listResponse.Message)
	var page redemptionPageResponse
	require.NoError(t, common.Unmarshal(listResponse.Data, &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, maskedKey, page.Items[0].Key)
	assert.True(t, page.Items[0].KeyRedacted)
	assert.NotContains(t, listRecorder.Body.String(), rawKey)

	detailCtx, detailRecorder := newAuthenticatedContext(t, http.MethodGet, "/api/redemption/"+strconv.Itoa(redemption.Id), nil, 1)
	detailCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(redemption.Id)}}
	GetRedemption(detailCtx)

	detailResponse := decodeAPIResponse(t, detailRecorder)
	require.True(t, detailResponse.Success, detailResponse.Message)
	var detail redemptionResponse
	require.NoError(t, common.Unmarshal(detailResponse.Data, &detail))
	assert.Equal(t, maskedKey, detail.Key)
	assert.True(t, detail.KeyRedacted)
	assert.NotContains(t, detailRecorder.Body.String(), rawKey)
}

func TestGetRedemptionKeyReturnsFullKey(t *testing.T) {
	db := setupRedemptionControllerTestDB(t)
	rawKey := "abcdef1234567890abcdef1234567890"
	redemption := seedControllerRedemption(t, db, rawKey)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/redemption/"+strconv.Itoa(redemption.Id)+"/key", nil, 1)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(redemption.Id)}}
	GetRedemptionKey(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var data redemptionKeyResponse
	require.NoError(t, common.Unmarshal(response.Data, &data))
	assert.Equal(t, rawKey, data.Key)
	assert.Contains(t, recorder.Body.String(), rawKey)

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	assert.True(t, len(logs) >= 1)
	assert.True(t, strings.Contains(logs[len(logs)-1].Content, "查看兑换码完整值"))
}
