// Package controller - linuxdo.go
// 该文件实现了 Linux DO OAuth 登录和绑定的 API 控制器
//
// Linux DO 是一个中文技术社区，使用 OAuth 2.0 授权码流程
// 支持信任等级验证：只有达到最低信任等级的用户才能注册
//
// 主要 API：
// - LinuxdoOAuth：Linux DO OAuth 回调处理（登录/注册）
// - LinuxDoBind：绑定 Linux DO 账户到现有用户
package controller

import (
	"encoding/base64"
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

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// LinuxdoUser Linux DO 用户信息结构体
type LinuxdoUser struct {
	Id         int    `json:"id"`          // 用户 ID
	Username   string `json:"username"`    // 用户名
	Name       string `json:"name"`        // 显示名称
	Active     bool   `json:"active"`      // 是否活跃
	TrustLevel int    `json:"trust_level"` // 信任等级（0-4）
	Silenced   bool   `json:"silenced"`    // 是否被禁言
}

// LinuxDoBind 将 Linux DO 账户绑定到当前登录用户
//
// 如果该 Linux DO 账户已被其他用户绑定，返回错误
func LinuxDoBind(c *gin.Context) {
	if !common.LinuxDOOAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Linux DO 登录以及注册",
		})
		return
	}

	code := c.Query("code")
	linuxdoUser, err := getLinuxdoUserInfoByCode(code, c)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	user := model.User{
		LinuxDOId: strconv.Itoa(linuxdoUser.Id),
	}

	if model.IsLinuxDOIdAlreadyTaken(user.LinuxDOId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该 Linux DO 账户已被绑定",
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

	user.LinuxDOId = strconv.Itoa(linuxdoUser.Id)
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

// getLinuxdoUserInfoByCode 通过授权码获取 Linux DO 用户信息
//
// 使用 Basic Auth 方式获取访问令牌，然后获取用户信息
//
// 参数：
//   - code: Linux DO OAuth 授权码
//   - c: Gin 上下文（用于构建重定向 URI）
//
// 返回值：
//   - *LinuxdoUser: Linux DO 用户信息
//   - err: 错误信息
func getLinuxdoUserInfoByCode(code string, c *gin.Context) (*LinuxdoUser, error) {
	if code == "" {
		return nil, errors.New("invalid code")
	}

	// Get access token using Basic auth
	tokenEndpoint := common.GetEnvOrDefaultString("LINUX_DO_TOKEN_ENDPOINT", "https://connect.linux.do/oauth2/token")
	credentials := common.LinuxDOClientId + ":" + common.LinuxDOClientSecret
	basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))

	// Get redirect URI from request
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	redirectURI := fmt.Sprintf("%s://%s/api/oauth/linuxdo", scheme, c.Request.Host)

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", basicAuth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("failed to connect to Linux DO server")
	}
	defer res.Body.Close()

	var tokenRes struct {
		AccessToken string `json:"access_token"`
		Message     string `json:"message"`
	}
	if err := json.NewDecoder(res.Body).Decode(&tokenRes); err != nil {
		return nil, err
	}

	if tokenRes.AccessToken == "" {
		return nil, fmt.Errorf("failed to get access token: %s", tokenRes.Message)
	}

	// Get user info
	userEndpoint := common.GetEnvOrDefaultString("LINUX_DO_USER_ENDPOINT", "https://connect.linux.do/api/user")
	req, err = http.NewRequest("GET", userEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tokenRes.AccessToken)
	req.Header.Set("Accept", "application/json")

	res2, err := client.Do(req)
	if err != nil {
		return nil, errors.New("failed to get user info from Linux DO")
	}
	defer res2.Body.Close()

	var linuxdoUser LinuxdoUser
	if err := json.NewDecoder(res2.Body).Decode(&linuxdoUser); err != nil {
		return nil, err
	}

	if linuxdoUser.Id == 0 {
		return nil, errors.New("invalid user info returned")
	}

	return &linuxdoUser, nil
}

// LinuxdoOAuth 处理 Linux DO OAuth 回调
//
// 根据会话状态判断是登录还是绑定操作：
// - 如果会话中有用户名信息，执行绑定操作
// - 否则执行登录/注册操作
//
// 注册时会验证用户的信任等级是否达到最低要求
//
// 查询参数：
//   - code: OAuth 授权码
//   - state: 状态参数（用于 CSRF 防护）
//   - error: 错误码（授权失败时返回）
func LinuxdoOAuth(c *gin.Context) {
	session := sessions.Default(c)

	errorCode := c.Query("error")
	if errorCode != "" {
		errorDescription := c.Query("error_description")
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": errorDescription,
		})
		return
	}

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
		LinuxDoBind(c)
		return
	}

	if !common.LinuxDOOAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Linux DO 登录以及注册",
		})
		return
	}

	code := c.Query("code")
	linuxdoUser, err := getLinuxdoUserInfoByCode(code, c)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	user := model.User{
		LinuxDOId: strconv.Itoa(linuxdoUser.Id),
	}

	// Check if user exists
	if model.IsLinuxDOIdAlreadyTaken(user.LinuxDOId) {
		err := user.FillUserByLinuxDOId()
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
			if linuxdoUser.TrustLevel >= common.LinuxDOMinimumTrustLevel {
				user.Username = "linuxdo_" + strconv.Itoa(model.GetMaxUserId()+1)
				user.DisplayName = linuxdoUser.Name
				user.Role = common.RoleCommonUser
				user.Status = common.UserStatusEnabled

				affCode := session.Get("aff")
				inviterId := 0
				if affCode != nil {
					inviterId, _ = model.GetUserIdByAffCode(affCode.(string))
				}

				if err := user.Insert(inviterId); err != nil {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": err.Error(),
					})
					return
				}
			} else {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Linux DO 信任等级未达到管理员设置的最低信任等级",
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
