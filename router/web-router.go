package router

import (
	"embed"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// ThemeAssets holds the embedded frontend assets for both themes.
type ThemeAssets struct {
	DefaultBuildFS      embed.FS
	DefaultIndexPage    []byte
	ClassicBuildFS      embed.FS
	ClassicIndexPage    []byte
	CPAManagerBuildFS   embed.FS
	CPAManagerIndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets ThemeAssets) {
	defaultFS := common.EmbedFolder(assets.DefaultBuildFS, "web/default/dist")
	classicFS := common.EmbedFolder(assets.ClassicBuildFS, "web/classic/dist")
	cpaManagerFS := common.EmbedFolder(assets.CPAManagerBuildFS, "modules/cpa-manager/dist")
	themeFS := common.NewThemeAwareFS(defaultFS, classicFS)

	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit())
	router.Use(middleware.Cache())
	setAccountPoolManagerRouter(router, cpaManagerFS, assets.CPAManagerIndexPage)
	router.Use(static.Serve("/", themeFS))
	router.NoRoute(func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		if common.GetTheme() == "classic" {
			c.Data(http.StatusOK, "text/html; charset=utf-8", assets.ClassicIndexPage)
		} else {
			c.Data(http.StatusOK, "text/html; charset=utf-8", assets.DefaultIndexPage)
		}
	})
}

func setAccountPoolManagerRouter(router *gin.Engine, cpaManagerFS static.ServeFileSystem, indexPage []byte) {
	router.GET("/account-pool/manager", accountPoolManagerSessionAuth(), func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/account-pool/manager/")
	})
	router.GET("/account-pool/manager/*path", accountPoolManagerSessionAuth(), func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		requestPath := strings.TrimPrefix(c.Param("path"), "/")
		if requestPath != "" && cpaManagerFS.Exists("/", requestPath) {
			c.FileFromFS(requestPath, cpaManagerFS)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	})
}

func accountPoolManagerSessionAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		role, roleOK := session.Get("role").(int)
		status, statusOK := session.Get("status").(int)
		if session.Get("id") == nil || !roleOK || !statusOK {
			loginPath := "/sign-in"
			if common.GetTheme() == "classic" {
				loginPath = "/login"
			}
			redirect := loginPath + "?redirect=/account-pool/manager/"
			c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.Redirect(http.StatusFound, redirect)
			c.Abort()
			return
		}
		if status == common.UserStatusDisabled {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		if role < common.RoleAdminUser {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}
