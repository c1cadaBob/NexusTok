// Package controller - setup.go
// 该文件实现了系统初始化设置的 API 控制器
//
// 系统初始化是首次运行时的必要步骤
// 需要创建管理员账户和配置基础参数
//
// 初始化流程：
// 1. 前端检查系统是否已初始化（GetSetup）
// 2. 如果未初始化，显示初始化表单
// 3. 用户提交管理员账户信息
// 4. 后端创建管理员账户并完成初始化
//
// 主要 API：
// - GetSetup：获取初始化状态
// - SetupSystem：执行系统初始化
package controller

import (
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// Setup 系统初始化状态结构体
type Setup struct {
	Status       bool   `json:"status"`       // 系统是否已完成初始化
	RootInit     bool   `json:"root_init"`     // 管理员账户是否已创建
	DatabaseType string `json:"database_type"` // 数据库类型
}

// SetupRequest 系统初始化请求结构体
type SetupRequest struct {
	Username           string `json:"username"`            // 管理员用户名
	Password           string `json:"password"`            // 管理员密码
	ConfirmPassword    string `json:"confirmPassword"`     // 确认密码
	SelfUseModeEnabled bool   `json:"SelfUseModeEnabled"`  // 是否启用自用模式
	DemoSiteEnabled    bool   `json:"DemoSiteEnabled"`     // 是否启用演示站点
}

// GetSetup 获取系统初始化状态
//
// 返回系统的初始化状态，包括：
// - 是否已完成初始化
// - 管理员账户是否已创建
// - 数据库类型
//
// 参数：
//   - c: Gin 上下文
func GetSetup(c *gin.Context) {
	setup := Setup{
		Status: constant.Setup,
	}
	// 如果已完成初始化，直接返回
	if constant.Setup {
		c.JSON(200, gin.H{
			"success": true,
			"data":    setup,
		})
		return
	}
	// 检查管理员账户是否存在
	setup.RootInit = model.RootUserExists()
	// 设置数据库类型
	if common.UsingMySQL {
		setup.DatabaseType = "mysql"
	}
	if common.UsingPostgreSQL {
		setup.DatabaseType = "postgres"
	}
	if common.UsingSQLite {
		setup.DatabaseType = "sqlite"
	}
	c.JSON(200, gin.H{
		"success": true,
		"data":    setup,
	})
}

func PostSetup(c *gin.Context) {
	// Check if setup is already completed
	if constant.Setup {
		c.JSON(200, gin.H{
			"success": false,
			"message": "系统已经初始化完成",
		})
		return
	}

	// Check if root user already exists
	rootExists := model.RootUserExists()

	var req SetupRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": "请求参数有误",
		})
		return
	}

	// If root doesn't exist, validate and create admin account
	if !rootExists {
		// Validate username length: max 12 characters to align with model.User validation
		if len(req.Username) > 12 {
			c.JSON(200, gin.H{
				"success": false,
				"message": "用户名长度不能超过12个字符",
			})
			return
		}
		// Validate password
		if req.Password != req.ConfirmPassword {
			c.JSON(200, gin.H{
				"success": false,
				"message": "两次输入的密码不一致",
			})
			return
		}

		if len(req.Password) < 8 {
			c.JSON(200, gin.H{
				"success": false,
				"message": "密码长度至少为8个字符",
			})
			return
		}

		// Create root user
		hashedPassword, err := common.Password2Hash(req.Password)
		if err != nil {
			c.JSON(200, gin.H{
				"success": false,
				"message": "系统错误: " + err.Error(),
			})
			return
		}
		rootUser := model.User{
			Username:    req.Username,
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		err = model.DB.Create(&rootUser).Error
		if err != nil {
			c.JSON(200, gin.H{
				"success": false,
				"message": "创建管理员账号失败: " + err.Error(),
			})
			return
		}
	}

	// Set operation modes
	operation_setting.SelfUseModeEnabled = req.SelfUseModeEnabled
	operation_setting.DemoSiteEnabled = req.DemoSiteEnabled

	// Save operation modes to database for persistence
	err = model.UpdateOption("SelfUseModeEnabled", boolToString(req.SelfUseModeEnabled))
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": "保存自用模式设置失败: " + err.Error(),
		})
		return
	}

	err = model.UpdateOption("DemoSiteEnabled", boolToString(req.DemoSiteEnabled))
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": "保存演示站点模式设置失败: " + err.Error(),
		})
		return
	}

	// Update setup status
	constant.Setup = true

	setup := model.Setup{
		Version:       common.Version,
		InitializedAt: time.Now().Unix(),
	}
	err = model.DB.Create(&setup).Error
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": "系统初始化失败: " + err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "系统初始化成功",
	})
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
