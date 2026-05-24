// Package controller - telegram.go
// 该文件实现了 Telegram 登录和绑定的 API 控制器
//
// Telegram 登录使用 Telegram Login Widget 进行身份验证
// 通过 HMAC-SHA256 验证请求的合法性
//
// 主要 API：
// - TelegramLogin：Telegram 登录处理
// - TelegramBind：绑定 Telegram 账户到现有用户
package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"sort"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// TelegramBind 将 Telegram 账户绑定到当前登录用户
//
// 验证 Telegram 授权数据后，将 Telegram ID 绑定到当前用户
// 如果该 Telegram 账户已被其他用户绑定，返回错误
func TelegramBind(c *gin.Context) {
	if !common.TelegramOAuthEnabled {
		c.JSON(200, gin.H{
			"message": "管理员未开启通过 Telegram 登录以及注册",
			"success": false,
		})
		return
	}
	params := c.Request.URL.Query()
	if !checkTelegramAuthorization(params, common.TelegramBotToken) {
		c.JSON(200, gin.H{
			"message": "无效的请求",
			"success": false,
		})
		return
	}
	telegramId := params["id"][0]
	if model.IsTelegramIdAlreadyTaken(telegramId) {
		c.JSON(200, gin.H{
			"message": "该 Telegram 账户已被绑定",
			"success": false,
		})
		return
	}

	session := sessions.Default(c)
	id := session.Get("id")
	user := model.User{Id: id.(int)}
	if err := user.FillUserById(); err != nil {
		c.JSON(200, gin.H{
			"message": err.Error(),
			"success": false,
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
	user.TelegramId = telegramId
	if err := user.Update(false); err != nil {
		c.JSON(200, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}

	c.Redirect(302, common.ThemeAwarePath("/console/personal"))
}

// TelegramLogin 处理 Telegram 登录
//
// 验证 Telegram 授权数据后，查找并登录对应用户
func TelegramLogin(c *gin.Context) {
	if !common.TelegramOAuthEnabled {
		c.JSON(200, gin.H{
			"message": "管理员未开启通过 Telegram 登录以及注册",
			"success": false,
		})
		return
	}
	params := c.Request.URL.Query()
	if !checkTelegramAuthorization(params, common.TelegramBotToken) {
		c.JSON(200, gin.H{
			"message": "无效的请求",
			"success": false,
		})
		return
	}

	telegramId := params["id"][0]
	user := model.User{TelegramId: telegramId}
	if err := user.FillUserByTelegramId(); err != nil {
		c.JSON(200, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	setupLogin(&user, c)
}

// checkTelegramAuthorization 验证 Telegram 授权数据的合法性
//
// 使用 HMAC-SHA256 算法验证请求签名
// 验证逻辑：
// 1. 将所有参数（除 hash 外）按字母顺序排列
// 2. 拼接为 key=value 格式
// 3. 使用 Bot Token 的 SHA256 哈希作为 HMAC 密钥
// 4. 计算 HMAC-SHA256 并与请求中的 hash 比较
//
// 参数：
//   - params: 请求参数
//   - token: Telegram Bot Token
//
// 返回值：
//   - bool: 验证是否通过
func checkTelegramAuthorization(params map[string][]string, token string) bool {
	strs := []string{}
	var hash = ""
	for k, v := range params {
		if k == "hash" {
			hash = v[0]
			continue
		}
		strs = append(strs, k+"="+v[0])
	}
	sort.Strings(strs)
	var imploded = ""
	for _, s := range strs {
		if imploded != "" {
			imploded += "\n"
		}
		imploded += s
	}
	sha256hash := sha256.New()
	io.WriteString(sha256hash, token)
	hmachash := hmac.New(sha256.New, sha256hash.Sum(nil))
	io.WriteString(hmachash, imploded)
	ss := hex.EncodeToString(hmachash.Sum(nil))
	return hash == ss
}
