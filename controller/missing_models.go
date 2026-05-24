// Package controller - missing_models.go
// 该文件实现了缺失模型查询的 API 控制器
//
// 缺失模型是指在渠道中引用但在模型元数据表中没有对应记录的模型
// 这有助于管理员发现需要配置的模型
//
// 主要 API：
// - GetMissingModels：获取缺失模型列表
package controller

import (
	"net/http"

	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
)

// GetMissingModels 获取缺失模型列表
//
// 返回在渠道中引用但没有在模型元数据表中配置的模型名称列表
// 管理员可以根据此列表补充模型配置
//
// 参数：
//   - c: Gin 上下文
func GetMissingModels(c *gin.Context) {
	missing, err := model.GetMissingModels()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    missing,
	})
}
