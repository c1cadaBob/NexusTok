// Package controller - oidc.go
// 该文件实现了 OpenID Connect (OIDC) 登录和绑定的 API 控制器
//
// OIDC 是基于 OAuth 2.0 的身份认证协议
// 支持标准的 OIDC Discovery 和 UserInfo 端点
//
// 主要 API：
// - OidcAuth：OIDC OAuth 回调处理（登录/注册）
// - OidcBind：绑定 OIDC 账户到现有用户
package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting/system_setting"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// OidcResponse OIDC 令牌响应结构体
type OidcResponse struct {
	AccessToken  string `json:"access_token"`  // 访问令牌
	IDToken      string `json:"id_token"`      // ID 令牌
	RefreshToken string `json:"refresh_token"` // 刷新令牌
	TokenType    string `json:"token_type"`    // 令牌类型
	ExpiresIn    int    `json:"expires_in"`    // 过期时间（秒）
	Scope        string `json:"scope"`         // 授权范围
}

// OidcUser OIDC 用户信息结构体
type OidcUser struct {
	OpenID            string `json:"sub"`                // 用户唯一标识
	Email             string `json:"email"`              // 邮箱地址
	Name              string `json:"name"`               // 显示名称
	PreferredUsername string `json:"preferred_username"` // 首选用户名
	Picture           string `json:"picture"`            // 头像 URL
}

// validateLegacyOIDCEndpointURL 使用系统 FetchSetting 校验旧 OIDC 控制器配置端点。
//
// 当前活跃 OAuth 路由已经迁移到 oauth.Provider 体系，但该旧控制器仍在编译范围内。
// 这里保持与活跃 OIDC provider 相同的安全边界，避免未来误接回旧路由时重新引入
// 配置端点 SSRF 风险。
func validateLegacyOIDCEndpointURL(urlStr string) error {
	fetchSetting := system_setting.GetFetchSetting()
	return common.ValidateURLWithFetchSetting(
		urlStr,
		fetchSetting.EnableSSRFProtection,
		fetchSetting.AllowPrivateIp,
		fetchSetting.DomainFilterMode,
		fetchSetting.IpFilterMode,
		fetchSetting.DomainList,
		fetchSetting.IpList,
		fetchSetting.AllowedPorts,
		fetchSetting.ApplyIPFilterForDomain,
	)
}

// newLegacyOIDCHTTPClient 返回旧 OIDC 控制器专用 HTTP client。
//
// 复用 protected fetch client 获得 Dial 阶段 DNS rebinding 防护；复制 client 后覆盖
// Timeout，保持旧控制器原有 5 秒超时行为。
func newLegacyOIDCHTTPClient() *http.Client {
	baseClient := service.GetSSRFProtectedHTTPClient()
	if baseClient == nil {
		return &http.Client{Timeout: 5 * time.Second}
	}
	client := *baseClient
	client.Timeout = 5 * time.Second
	return &client
}

// getOidcUserInfoByCode 通过授权码获取 OIDC 用户信息
//
// 流程：
// 1. 使用授权码交换访问令牌
// 2. 使用访问令牌获取 UserInfo 端点的用户信息
//
// 参数：
//   - code: OIDC OAuth 授权码
//
// 返回值：
//   - *OidcUser: OIDC 用户信息
//   - err: 错误信息
func getOidcUserInfoByCode(code string) (*OidcUser, error) {
	if code == "" {
		return nil, errors.New("无效的参数")
	}

	values := url.Values{}
	values.Set("client_id", system_setting.GetOIDCSettings().ClientId)
	values.Set("client_secret", system_setting.GetOIDCSettings().ClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", fmt.Sprintf("%s/oauth/oidc", system_setting.ServerAddress))
	formData := values.Encode()
	tokenEndpoint := system_setting.GetOIDCSettings().TokenEndpoint
	if err := validateLegacyOIDCEndpointURL(tokenEndpoint); err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 OIDC 服务器，请稍后重试！")
	}
	req, err := http.NewRequest("POST", tokenEndpoint, strings.NewReader(formData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := newLegacyOIDCHTTPClient()
	res, err := client.Do(req)
	if err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 OIDC 服务器，请稍后重试！")
	}
	defer res.Body.Close()
	var oidcResponse OidcResponse
	err = common.DecodeJson(res.Body, &oidcResponse)
	if err != nil {
		return nil, err
	}

	if oidcResponse.AccessToken == "" {
		common.SysLog("OIDC 获取 Token 失败，请检查设置！")
		return nil, errors.New("OIDC 获取 Token 失败，请检查设置！")
	}

	userInfoEndpoint := system_setting.GetOIDCSettings().UserInfoEndpoint
	if err := validateLegacyOIDCEndpointURL(userInfoEndpoint); err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 OIDC 服务器，请稍后重试！")
	}
	req, err = http.NewRequest("GET", userInfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+oidcResponse.AccessToken)
	res2, err := client.Do(req)
	if err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 OIDC 服务器，请稍后重试！")
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		common.SysLog("OIDC 获取用户信息失败！请检查设置！")
		return nil, errors.New("OIDC 获取用户信息失败！请检查设置！")
	}

	var oidcUser OidcUser
	err = common.DecodeJson(res2.Body, &oidcUser)
	if err != nil {
		return nil, err
	}
	if oidcUser.OpenID == "" || oidcUser.Email == "" {
		common.SysLog("OIDC 获取用户信息为空！请检查设置！")
		return nil, errors.New("OIDC 获取用户信息为空！请检查设置！")
	}
	return &oidcUser, nil
}

// OidcAuth 处理 OIDC OAuth 回调
//
// 根据会话状态判断是登录还是绑定操作：
// - 如果会话中有用户名信息，执行绑定操作
// - 否则执行登录/注册操作
//
// 查询参数：
//   - code: OAuth 授权码
//   - state: 状态参数（用于 CSRF 防护）
func OidcAuth(c *gin.Context) {
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
		OidcBind(c)
		return
	}
	if !system_setting.GetOIDCSettings().Enabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 OIDC 登录以及注册",
		})
		return
	}
	code := c.Query("code")
	oidcUser, err := getOidcUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user := model.User{
		OidcId: oidcUser.OpenID,
	}
	if model.IsOidcIdAlreadyTaken(user.OidcId) {
		err := user.FillUserByOidcId()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	} else {
		if common.RegisterEnabled {
			user.Email = oidcUser.Email
			if oidcUser.PreferredUsername != "" {
				user.Username = oidcUser.PreferredUsername
			} else {
				user.Username = "oidc_" + strconv.Itoa(model.GetMaxUserId()+1)
			}
			if oidcUser.Name != "" {
				user.DisplayName = oidcUser.Name
			} else {
				user.DisplayName = "OIDC User"
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

// OidcBind 将 OIDC 账户绑定到当前登录用户
//
// 如果该 OIDC 账户已被其他用户绑定，返回错误
func OidcBind(c *gin.Context) {
	if !system_setting.GetOIDCSettings().Enabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 OIDC 登录以及注册",
		})
		return
	}
	code := c.Query("code")
	oidcUser, err := getOidcUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user := model.User{
		OidcId: oidcUser.OpenID,
	}
	if model.IsOidcIdAlreadyTaken(user.OidcId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该 OIDC 账户已被绑定",
		})
		return
	}
	session := sessions.Default(c)
	id := session.Get("id")
	// id := c.GetInt("id")  // critical bug!
	user.Id = id.(int)
	err = user.FillUserById()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user.OidcId = oidcUser.OpenID
	err = user.Update(false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "bind",
	})
	return
}
