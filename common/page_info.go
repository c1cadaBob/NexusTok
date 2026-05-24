// Package common - page_info.go
// 该文件定义了分页信息结构体和分页查询参数解析函数
//
// 分页参数：
// - page/p: 页码（从 1 开始）
// - page_size/ps/size: 每页条目数（默认 10，最大 100）
//
// 支持多种查询参数名称以兼容不同的前端实现
package common

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// PageInfo 分页信息结构体
//
// 包含分页参数和查询结果
// 用于列表查询的请求和响应
type PageInfo struct {
	Page     int `json:"page"`      // 页码（从 1 开始）
	PageSize int `json:"page_size"` // 每页条目数

	Total int `json:"total"` // 总条数（查询后设置）
	Items any `json:"items"` // 数据（查询后设置）
}

// GetStartIdx 获取分页起始索引
//
// 用于数据库查询的 OFFSET
//
// 返回值：
//   - int: 起始索引
func (p *PageInfo) GetStartIdx() int {
	return (p.Page - 1) * p.PageSize
}

// GetEndIdx 获取分页结束索引
//
// 用于内存分页的结束位置
//
// 返回值：
//   - int: 结束索引
func (p *PageInfo) GetEndIdx() int {
	return p.Page * p.PageSize
}

// GetPageSize 获取每页条目数
func (p *PageInfo) GetPageSize() int {
	return p.PageSize
}

// GetPage 获取页码
func (p *PageInfo) GetPage() int {
	return p.Page
}

// SetTotal 设置总条数
func (p *PageInfo) SetTotal(total int) {
	p.Total = total
}

// SetItems 设置数据
func (p *PageInfo) SetItems(items any) {
	p.Items = items
}

// GetPageQuery 从 Gin 上下文获取分页查询参数
//
// 支持的查询参数：
// - page/p: 页码（默认 1）
// - page_size/ps/size: 每页条目数（默认 10，最大 100）
//
// 参数：
//   - c: Gin 上下文
//
// 返回值：
//   - *PageInfo: 分页信息
func GetPageQuery(c *gin.Context) *PageInfo {
	pageInfo := &PageInfo{}
	// 手动获取并处理每个参数
	if page, err := strconv.Atoi(c.Query("p")); err == nil {
		pageInfo.Page = page
	}
	if pageSize, err := strconv.Atoi(c.Query("page_size")); err == nil {
		pageInfo.PageSize = pageSize
	}
	if pageInfo.Page < 1 {
		// 兼容旧的查询参数
		page, _ := strconv.Atoi(c.Query("p"))
		if page != 0 {
			pageInfo.Page = page
		} else {
			pageInfo.Page = 1
		}
	}

	if pageInfo.PageSize == 0 {
		// 兼容旧的查询参数
		pageSize, _ := strconv.Atoi(c.Query("ps"))
		if pageSize != 0 {
			pageInfo.PageSize = pageSize
		}
		if pageInfo.PageSize == 0 {
			pageSize, _ = strconv.Atoi(c.Query("size")) // token page
			if pageSize != 0 {
				pageInfo.PageSize = pageSize
			}
		}
		if pageInfo.PageSize == 0 {
			pageInfo.PageSize = ItemsPerPage // 使用默认值
		}
	}

	// 限制最大每页条目数为 100
	if pageInfo.PageSize > 100 {
		pageInfo.PageSize = 100
	}

	return pageInfo
}
