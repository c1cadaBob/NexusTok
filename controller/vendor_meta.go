// Package controller - vendor_meta.go
// 该文件实现了供应商（Vendor）元数据管理的 API 控制器
//
// 供应商是 AI 模型的提供方（如 OpenAI、Anthropic、Google 等）
// 功能包括：
// - 供应商列表查询（分页）
// - 供应商搜索
// - 供应商 CRUD 操作（创建、读取、更新、删除）
// - 供应商名称唯一性检查
//
// 主要 API：
// - GetAllVendors：获取供应商列表
// - SearchVendors：搜索供应商
// - GetVendorMeta：获取单个供应商
// - CreateVendorMeta：创建供应商
// - UpdateVendorMeta：更新供应商
// - DeleteVendorMeta：删除供应商
package controller

import (
	"strconv"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
)

// GetAllVendors 获取供应商列表（分页）
func GetAllVendors(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	vendors, err := model.GetAllVendors(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var total int64
	model.DB.Model(&model.Vendor{}).Count(&total)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(vendors)
	common.ApiSuccess(c, pageInfo)
}

// SearchVendors 搜索供应商
func SearchVendors(c *gin.Context) {
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	vendors, total, err := model.SearchVendors(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(vendors)
	common.ApiSuccess(c, pageInfo)
}

// GetVendorMeta 根据 ID 获取供应商
func GetVendorMeta(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	v, err := model.GetVendorByID(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, v)
}

// CreateVendorMeta 新建供应商
func CreateVendorMeta(c *gin.Context) {
	var v model.Vendor
	if err := c.ShouldBindJSON(&v); err != nil {
		common.ApiError(c, err)
		return
	}
	if v.Name == "" {
		common.ApiErrorMsg(c, "供应商名称不能为空")
		return
	}
	// 创建前先检查名称
	if dup, err := model.IsVendorNameDuplicated(0, v.Name); err != nil {
		common.ApiError(c, err)
		return
	} else if dup {
		common.ApiErrorMsg(c, "供应商名称已存在")
		return
	}

	if err := v.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, &v)
}

// UpdateVendorMeta 更新供应商
func UpdateVendorMeta(c *gin.Context) {
	var v model.Vendor
	if err := c.ShouldBindJSON(&v); err != nil {
		common.ApiError(c, err)
		return
	}
	if v.Id == 0 {
		common.ApiErrorMsg(c, "缺少供应商 ID")
		return
	}
	// 名称冲突检查
	if dup, err := model.IsVendorNameDuplicated(v.Id, v.Name); err != nil {
		common.ApiError(c, err)
		return
	} else if dup {
		common.ApiErrorMsg(c, "供应商名称已存在")
		return
	}

	if err := v.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, &v)
}

// DeleteVendorMeta 删除供应商
func DeleteVendorMeta(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Delete(&model.Vendor{}, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
