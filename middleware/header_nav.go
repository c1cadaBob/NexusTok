// Package middleware - header_nav.go
// 该文件实现顶部导航模块的后端访问控制。
//
// HeaderNavModules 同时驱动前端导航显示和后端接口兜底鉴权：
// - enabled=false：公开入口关闭，直接访问对应 API 也会被拒绝；
// - requireAuth=true：页面会引导登录，后端也必须强制登录；
// - 旧版布尔配置仍被兼容，false 表示模块关闭。
package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"

	"github.com/gin-gonic/gin"
)

// headerNavAccess 描述一个顶部导航模块的后端访问策略。
type headerNavAccess struct {
	Enabled     bool
	RequireAuth bool
}

// getHeaderNavAccess 读取指定模块的访问策略。
// 解析失败或配置缺失时默认公开启用，保持历史站点升级后的兼容性。
func getHeaderNavAccess(module string) headerNavAccess {
	fallback := headerNavAccess{
		Enabled:     true,
		RequireAuth: false,
	}

	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap["HeaderNavModules"]
	common.OptionMapRWMutex.RUnlock()

	if strings.TrimSpace(raw) == "" {
		return fallback
	}

	var parsed map[string]any
	if err := common.Unmarshal([]byte(raw), &parsed); err != nil {
		return fallback
	}

	return parseHeaderNavAccess(parsed[module], fallback)
}

// parseHeaderNavAccess 将历史布尔值和新对象格式统一为 headerNavAccess。
func parseHeaderNavAccess(raw any, fallback headerNavAccess) headerNavAccess {
	switch value := raw.(type) {
	case bool:
		return headerNavAccess{
			Enabled:     value,
			RequireAuth: fallback.RequireAuth,
		}
	case string:
		return headerNavAccess{
			Enabled:     parseHeaderNavBool(value, fallback.Enabled),
			RequireAuth: fallback.RequireAuth,
		}
	case float64:
		return headerNavAccess{
			Enabled:     parseHeaderNavBool(value, fallback.Enabled),
			RequireAuth: fallback.RequireAuth,
		}
	case map[string]any:
		access := fallback
		if enabled, ok := value["enabled"]; ok {
			access.Enabled = parseHeaderNavBool(enabled, fallback.Enabled)
		}
		if requireAuth, ok := value["requireAuth"]; ok {
			access.RequireAuth = parseHeaderNavBool(requireAuth, fallback.RequireAuth)
		}
		return access
	default:
		return fallback
	}
}

// parseHeaderNavBool 兼容 JSON bool、字符串和数字形式的布尔配置。
func parseHeaderNavBool(value any, fallback bool) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1":
			return true
		case "false", "0":
			return false
		default:
			return fallback
		}
	case float64:
		if v == 1 {
			return true
		}
		if v == 0 {
			return false
		}
		return fallback
	case int:
		if v == 1 {
			return true
		}
		if v == 0 {
			return false
		}
		return fallback
	default:
		return fallback
	}
}

// HeaderNavModuleAuth 保护与顶部导航公开模块对应的 API。
// 模块关闭时返回 403；模块要求登录时走 UserAuth；否则走 TryUserAuth，
// 让已登录用户仍能获得分组价格等个性化数据。
func HeaderNavModuleAuth(module string) gin.HandlerFunc {
	return func(c *gin.Context) {
		access := getHeaderNavAccess(module)
		if !access.Enabled {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": fmt.Sprintf("%s is disabled", module),
			})
			c.Abort()
			return
		}

		if access.RequireAuth {
			UserAuth()(c)
			return
		}

		TryUserAuth()(c)
	}
}

// HeaderNavModulePublicOrUserAuth 用于模块关闭后仍允许登录用户访问的辅助 API。
// 例如 perf-metrics 是 pricing 页面辅助数据：公开 pricing 关闭时匿名用户不可访问，
// 但登录用户在后台或其它入口仍可获取自己的可见数据。
func HeaderNavModulePublicOrUserAuth(module string) gin.HandlerFunc {
	return func(c *gin.Context) {
		access := getHeaderNavAccess(module)
		if !access.Enabled || access.RequireAuth {
			UserAuth()(c)
			return
		}

		TryUserAuth()(c)
	}
}
