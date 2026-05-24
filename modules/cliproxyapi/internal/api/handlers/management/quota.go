// management - quota.go
// 配额耗尽相关的配置端点。
// 该模块提供配额耗尽时的行为配置，包括是否切换到其他项目和是否切换到预览模型。
package management

import "github.com/gin-gonic/gin"

// GetSwitchProject 获取配额耗尽时是否切换项目的配置值。
func (h *Handler) GetSwitchProject(c *gin.Context) {
	c.JSON(200, gin.H{"switch-project": h.cfg.QuotaExceeded.SwitchProject})
}

// PutSwitchProject 设置配额耗尽时是否切换项目的配置值。
func (h *Handler) PutSwitchProject(c *gin.Context) {
	h.updateBoolField(c, func(v bool) { h.cfg.QuotaExceeded.SwitchProject = v })
}

// GetSwitchPreviewModel 获取配额耗尽时是否切换到预览模型的配置值。
func (h *Handler) GetSwitchPreviewModel(c *gin.Context) {
	c.JSON(200, gin.H{"switch-preview-model": h.cfg.QuotaExceeded.SwitchPreviewModel})
}

// PutSwitchPreviewModel 设置配额耗尽时是否切换到预览模型的配置值。
func (h *Handler) PutSwitchPreviewModel(c *gin.Context) {
	h.updateBoolField(c, func(v bool) { h.cfg.QuotaExceeded.SwitchPreviewModel = v })
}
