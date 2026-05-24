// Package dto - playground.go
// 该文件定义了 Playground（在线调试）相关的数据传输对象
//
// 主要结构体：
// - PlayGroundRequest：Playground 请求（指定模型和用户组）
package dto

// PlayGroundRequest Playground 调试请求
// Model：目标模型名称（可选，不指定则使用默认模型）
// Group：用户组（可选，用于确定可用模型和计费规则）
type PlayGroundRequest struct {
	Model string `json:"model,omitempty"`
	Group string `json:"group,omitempty"`
}
