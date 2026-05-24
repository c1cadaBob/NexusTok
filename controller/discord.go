// Package controller - discord.go
// 该文件实现了 Discord OAuth 登录和绑定的 API 控制器
//
// Discord OAuth 流程：
// 1. 用户访问 Discord 授权页面
// 2. 授权后回调到 DiscordOAuth
// 3. 使用授权码获取访问令牌
// 4. 获取 Discord 用户信息
// 5. 登录或注册用户
//
// 主要 API：
// - DiscordOAuth：Discord OAuth 回调处理（登录/注册）
// - DiscordBind：绑定 Discord 账户到现有用户
package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/system_setting"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// DiscordResponse Discord OAuth 令牌响应结构体
type DiscordResponse struct {
	AccessToken  string `json:"access_token"`  // 访问令牌
	IDToken      string `json:"id_token"`      // ID 令牌
	RefreshToken string `json:"refresh_token"` // 刷新令牌
	TokenType    string `json:"token_type"`    // 令牌类型
	ExpiresIn    int    `json:"expires_in"`    // 过期时间（秒）
	Scope        string `json:"scope"`         // 授权范围
}

// DiscordUser Discord 用户信息结构体
type DiscordUser struct {
	UID  string `json:"id"`         // Discord 用户唯一 ID
	ID   string `json:"username"`   // Discord 用户名
	Name string `json:"global_name"` // Discord 全局显示名称
}

// getDiscordUserInfoByCode 通过授权码获取 Discord 用户信息
//
// 流程：
// 1. 使用授权码交换访问令牌
// 2. 使用访问令牌获取用户信息
//
// 参数：
//   - code: Discord OAuth 授权码
//
// 返回值：
//   - *DiscordUser: Discord 用户信息
//   - err: 错误信息
func getDiscordUserInfoByCode(code string) (*DiscordUser, error) {
	if code == "" {
		return nil, errors.New("无效的参数")
	}

	values := url.Values{}
	values.Set("client_id", system_setting.GetDiscordSettings().ClientId)
	values.Set("client_secret", system_setting.GetDiscordSettings().ClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", fmt.Sprintf("%s/oauth/discord", system_setting.ServerAddress))
	formData := values.Encode()
	req, err := http.NewRequest("POST", "https://discord.com/api/v10/oauth2/token", strings.NewReader(formData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 Discord 服务器，请稍后重试！")
	}
	defer res.Body.Close()
	var discordResponse DiscordResponse
	err = json.NewDecoder(res.Body).Decode(&discordResponse)
	if err != nil {
		return nil, err
	}

	if discordResponse.AccessToken == "" {
		common.SysError("Discord 获取 Token 失败，请检查设置！")
		return nil, errors.New("Discord 获取 Token 失败，请检查设置！")
	}

	req, err = http.NewRequest("GET", "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+discordResponse.AccessToken)
	res2, err := client.Do(req)
	if err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 Discord 服务器，请稍后重试！")
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		common.SysError("Discord 获取用户信息失败！请检查设置！")
		return nil, errors.New("Discord 获取用户信息失败！请检查设置！")
	}

	var discordUser DiscordUser
	err = json.NewDecoder(res2.Body).Decode(&discordUser)
	if err != nil {
		return nil, err
	}
	if discordUser.UID == "" || discordUser.ID == "" {
		common.SysError("Discord 获取用户信息为空！请检查设置！")
		return nil, errors.New("Discord 获取用户信息为空！请检查设置！")
	}
	return &discordUser, nil
}

// DiscordOAuth 处理 Discord OAuth 回调
//
// 根据会话状态判断是登录还是绑定操作：
// - 如果会话中有用户名信息，执行绑定操作
// - 否则执行登录/注册操作
//
// 查询参数：
//   - code: OAuth 授权码
//   - state: 状态参数（用于 CSRF 防护）
func DiscordOAuth(c *gin.Context) {
	session := sessions.Default(c)
	state := c.Query("state")
	if state == "" || session.Get("oauth_state") == nil || state != session.Get("oauth_state").(string) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "state is empty or not same",
		})
		return
	}
	username := session.Get("username")
	if username != nil {
		DiscordBind(c)
		return
	}
	if !system_setting.GetDiscordSettings().Enabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Discord 登录以及注册",
		})
		return
	}
	code := c.Query("code")
	discordUser, err := getDiscordUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user := model.User{
		DiscordId: discordUser.UID,
	}
	if model.IsDiscordIdAlreadyTaken(user.DiscordId) {
		err := user.FillUserByDiscordId()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	} else {
		if common.RegisterEnabled {
			if discordUser.ID != "" {
				user.Username = discordUser.ID
			} else {
				user.Username = "discord_" + strconv.Itoa(model.GetMaxUserId()+1)
			}
			if discordUser.Name != "" {
				user.DisplayName = discordUser.Name
			} else {
				user.DisplayName = "Discord User"
			}
			err := user.Insert(0)
			if err != nil {
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

// DiscordBind 将 Discord 账户绑定到当前登录用户
//
// 如果该 Discord 账户已被其他用户绑定，返回错误
func DiscordBind(c *gin.Context) {
	if !system_setting.GetDiscordSettings().Enabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Discord 登录以及注册",
		})
		return
	}
	code := c.Query("code")
	discordUser, err := getDiscordUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user := model.User{
		DiscordId: discordUser.UID,
	}
	if model.IsDiscordIdAlreadyTaken(user.DiscordId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该 Discord 账户已被绑定",
		})
		return
	}
	session := sessions.Default(c)
	id := session.Get("id")
	user.Id = id.(int)
	err = user.FillUserById()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user.DiscordId = discordUser.UID
	err = user.Update(false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "bind",
	})
}
