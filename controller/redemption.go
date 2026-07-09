// Package controller - redemption.go
// 该文件实现了兑换码管理的 API 控制器
//
// 兑换码功能允许用户通过兑换码获取 API 额度
//
// 主要 API：
// - GetAllRedemptions：管理员获取所有兑换码
// - SearchRedemptions：搜索兑换码
// - GetRedemption：获取特定兑换码详情
// - CreateRedemption：创建兑换码
// - DeleteRedemption：删除兑换码
// - RedeemCode：用户兑换兑换码
package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/i18n"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

type redemptionResponse struct {
	Id           int    `json:"id"`
	UserId       int    `json:"user_id"`
	Key          string `json:"key"`
	KeyRedacted  bool   `json:"key_redacted"`
	Status       int    `json:"status"`
	Name         string `json:"name"`
	Quota        int    `json:"quota"`
	CreatedTime  int64  `json:"created_time"`
	RedeemedTime int64  `json:"redeemed_time"`
	UsedUserId   int    `json:"used_user_id"`
	ExpiredTime  int64  `json:"expired_time"`
}

type redemptionKeyResponse struct {
	Key string `json:"key"`
}

// buildRedemptionResponse 构造管理端兑换码响应。
//
// 默认路径只返回脱敏 key；完整兑换码必须通过受 `redemption.secret_view`
// 和安全验证保护的 reveal 接口获取，避免普通 read 权限泄露可入账额度凭据。
func buildRedemptionResponse(redemption *model.Redemption, revealKey bool) *redemptionResponse {
	if redemption == nil {
		return nil
	}
	key := redemption.GetMaskedKey()
	keyRedacted := true
	if revealKey {
		key = redemption.Key
		keyRedacted = false
	}
	return &redemptionResponse{
		Id:           redemption.Id,
		UserId:       redemption.UserId,
		Key:          key,
		KeyRedacted:  keyRedacted,
		Status:       redemption.Status,
		Name:         redemption.Name,
		Quota:        redemption.Quota,
		CreatedTime:  redemption.CreatedTime,
		RedeemedTime: redemption.RedeemedTime,
		UsedUserId:   redemption.UsedUserId,
		ExpiredTime:  redemption.ExpiredTime,
	}
}

func buildRedemptionResponses(redemptions []*model.Redemption) []*redemptionResponse {
	result := make([]*redemptionResponse, 0, len(redemptions))
	for _, redemption := range redemptions {
		result = append(result, buildRedemptionResponse(redemption, false))
	}
	return result
}

// GetAllRedemptions 管理员获取所有兑换码
//
// 支持分页查询
//
// 参数：
//   - c: Gin 上下文
func GetAllRedemptions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.GetAllRedemptions(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildRedemptionResponses(redemptions))
	common.ApiSuccess(c, pageInfo)
	return
}

// SearchRedemptions 搜索兑换码
//
// 支持按关键词和状态搜索；状态筛选在后端完成，避免分页时只过滤当前页。
//
// 查询参数：
//   - keyword: 搜索关键词
//   - status: 状态筛选，可为 1、2、3 或 expired
func SearchRedemptions(c *gin.Context) {
	keyword := c.Query("keyword")
	status := c.Query("status")
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.SearchRedemptions(keyword, status, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildRedemptionResponses(redemptions))
	common.ApiSuccess(c, pageInfo)
	return
}

// GetRedemption 获取特定兑换码详情
//
// 路径参数：
//   - id: 兑换码 ID
func GetRedemption(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	redemption, err := model.GetRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildRedemptionResponse(redemption, false),
	})
	return
}

// GetRedemptionKey reveal 单个兑换码完整值。
//
// 路由层必须先通过 `redemption.secret_view`、关键限流、禁缓存和安全验证；
// handler 只返回完整 key 本身，避免额外元数据在敏感响应中扩大暴露面。
func GetRedemptionKey(c *gin.Context) {
	userId := c.GetInt("id")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	redemption, err := model.GetRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(userId, model.LogTypeSystem, fmt.Sprintf("查看兑换码完整值 (兑换码ID: %d)", id))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "获取成功",
		"data":    redemptionKeyResponse{Key: redemption.Key},
	})
}

func AddRedemption(c *gin.Context) {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
		return
	}

	redemption := model.Redemption{}
	err := c.ShouldBindJSON(&redemption)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if utf8.RuneCountInString(redemption.Name) == 0 || utf8.RuneCountInString(redemption.Name) > 20 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionNameLength)
		return
	}
	if redemption.Count <= 0 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionCountPositive)
		return
	}
	if redemption.Count > 100 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionCountMax)
		return
	}
	if valid, msg := validateExpiredTime(c, redemption.ExpiredTime); !valid {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
		return
	}
	var keys []string
	for i := 0; i < redemption.Count; i++ {
		key := common.GetUUID()
		cleanRedemption := model.Redemption{
			UserId:      c.GetInt("id"),
			Name:        redemption.Name,
			Key:         key,
			CreatedTime: common.GetTimestamp(),
			Quota:       redemption.Quota,
			ExpiredTime: redemption.ExpiredTime,
		}
		err = cleanRedemption.Insert()
		if err != nil {
			common.SysError("failed to insert redemption: " + err.Error())
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": i18n.T(c, i18n.MsgRedemptionCreateFailed),
				"data":    keys,
			})
			return
		}
		keys = append(keys, key)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    keys,
	})
	return
}

func DeleteRedemption(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	err := model.DeleteRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func UpdateRedemption(c *gin.Context) {
	statusOnly := c.Query("status_only")
	redemption := model.Redemption{}
	err := c.ShouldBindJSON(&redemption)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cleanRedemption, err := model.GetRedemptionById(redemption.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if statusOnly == "" {
		if valid, msg := validateExpiredTime(c, redemption.ExpiredTime); !valid {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		// If you add more fields, please also update redemption.Update()
		cleanRedemption.Name = redemption.Name
		cleanRedemption.Quota = redemption.Quota
		cleanRedemption.ExpiredTime = redemption.ExpiredTime
	}
	if statusOnly != "" {
		cleanRedemption.Status = redemption.Status
	}
	err = cleanRedemption.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildRedemptionResponse(cleanRedemption, false),
	})
	return
}

func DeleteInvalidRedemption(c *gin.Context) {
	rows, err := model.DeleteInvalidRedemptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
	return
}

func validateExpiredTime(c *gin.Context, expired int64) (bool, string) {
	if expired != 0 && expired < common.GetTimestamp() {
		return false, i18n.T(c, i18n.MsgRedemptionExpireTimeInvalid)
	}
	return true, ""
}
