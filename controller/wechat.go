// Package controller - wechat.go
// 该文件实现了微信登录和绑定的 API 控制器
//
// 微信登录通过微信服务器验证用户身份
// 流程：
// 1. 用户在微信客户端获取授权码（code）
// 2. 前端将 code 发送到后端
// 3. 后端调用微信服务器获取微信 ID
// 4. 根据微信 ID 查找或创建用户
// 5. 完成登录
//
// 主要 API：
// - WeChatAuth：微信登录
// - WeChatBind：绑定微信账号到现有用户
package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// wechatLoginResponse 微信登录响应结构体
type wechatLoginResponse struct {
	Success bool   `json:"success"` // 是否成功
	Message string `json:"message"` // 错误信息
	Data    string `json:"data"`    // 微信 ID
}

// getWeChatIdByCode 通过授权码获取微信 ID
//
// 调用微信服务器的 /api/wechat/user 接口验证授权码
//
// 参数：
//   - code: 微信授权码
//
// 返回：
//   - string: 微信 ID
//   - error: 获取失败时返回错误
func getWeChatIdByCode(code string) (string, error) {
	if code == "" {
		return "", errors.New("无效的参数")
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/wechat/user?code=%s", common.WeChatServerAddress, url.QueryEscape(code)), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", common.WeChatServerToken)
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	httpResponse, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer httpResponse.Body.Close()
	var res wechatLoginResponse
	err = json.NewDecoder(httpResponse.Body).Decode(&res)
	if err != nil {
		return "", err
	}
	if !res.Success {
		return "", errors.New(res.Message)
	}
	if res.Data == "" {
		return "", errors.New("验证码错误或已过期")
	}
	return res.Data, nil
}

// WeChatAuth 处理微信登录
//
// 流程：
// 1. 检查微信登录是否启用
// 2. 通过授权码获取微信 ID
// 3. 如果微信 ID 已绑定用户，直接登录
// 4. 如果微信 ID 未绑定且注册已启用，创建新用户
// 5. 检查用户状态，完成登录
func WeChatAuth(c *gin.Context) {
	if !common.WeChatAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "管理员未开启通过微信登录以及注册",
			"success": false,
		})
		return
	}
	code := c.Query("code")
	wechatId, err := getWeChatIdByCode(code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	user := model.User{
		WeChatId: wechatId,
	}
	if model.IsWeChatIdAlreadyTaken(wechatId) {
		err := user.FillUserByWeChatId()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		if user.Id == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "用户已注销",
			})
			return
		}
	} else {
		if common.RegisterEnabled {
			user.Username = "wechat_" + strconv.Itoa(model.GetMaxUserId()+1)
			user.DisplayName = "WeChat User"
			user.Role = common.RoleCommonUser
			user.Status = common.UserStatusEnabled

			if err := user.Insert(0); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "管理员关闭了新用户注册",
			})
			return
		}
	}

	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "用户已被封禁",
			"success": false,
		})
		return
	}
	setupLogin(&user, c)
}

// wechatBindRequest 微信绑定请求结构体
type wechatBindRequest struct {
	Code string `json:"code"` // 微信授权码
}

// WeChatBind 将微信账号绑定到当前登录用户
//
// 流程：
// 1. 检查微信登录是否启用
// 2. 通过授权码获取微信 ID
// 3. 检查微信 ID 是否已被其他用户绑定
// 4. 将微信 ID 绑定到当前用户
func WeChatBind(c *gin.Context) {
	if !common.WeChatAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "管理员未开启通过微信登录以及注册",
			"success": false,
		})
		return
	}
	var req wechatBindRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的请求",
		})
		return
	}
	code := req.Code
	wechatId, err := getWeChatIdByCode(code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	if model.IsWeChatIdAlreadyTaken(wechatId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该微信账号已被绑定",
		})
		return
	}
	session := sessions.Default(c)
	id := session.Get("id")
	user := model.User{
		Id: id.(int),
	}
	err = user.FillUserById()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user.WeChatId = wechatId
	err = user.Update(false)
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
