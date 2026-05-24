// Package dto - request_common.go
// 该文件定义了请求接口的通用抽象
//
// 主要接口和结构体：
// - Request：请求接口（所有 API 请求必须实现）
// - BaseRequest：基础请求实现（提供默认方法）
//
// Request 接口方法：
// - GetTokenCountMeta：获取 Token 计数元数据（用于计费）
// - IsStream：判断是否为流式请求
// - SetModelName：设置模型名称（用于模型映射）
package dto

import (
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
)

// Request 请求接口
// 所有 API 请求结构体必须实现此接口
// GetTokenCountMeta：获取 Token 计数元数据
// IsStream：判断是否为流式请求
// SetModelName：设置模型名称
type Request interface {
	GetTokenCountMeta() *types.TokenCountMeta
	IsStream(c *gin.Context) bool
	SetModelName(modelName string)
}

// BaseRequest 基础请求结构体
// 提供 Request 接口的默认实现
// 可嵌入到其他请求结构体中以减少重复代码
type BaseRequest struct {
}

// GetTokenCountMeta 获取 Token 计数元数据（默认使用 Tokenizer 方式）
func (b *BaseRequest) GetTokenCountMeta() *types.TokenCountMeta {
	return &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
}
// IsStream 默认不支持流式输出
func (b *BaseRequest) IsStream(c *gin.Context) bool {
	return false
}
// SetModelName 空实现（子结构体应覆盖此方法）
func (b *BaseRequest) SetModelName(modelName string) {}
