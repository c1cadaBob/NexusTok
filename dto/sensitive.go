// Package dto - sensitive.go
// 该文件定义了敏感词过滤相关的数据传输对象
//
// 主要结构体：
// - SensitiveResponse：敏感词过滤响应
//
// 用途：用于内容审核，返回检测到的敏感词和过滤后的内容
package dto

// SensitiveResponse 敏感词过滤响应
// SensitiveWords：检测到的敏感词列表
// Content：过滤后的内容（敏感词被替换或移除）
type SensitiveResponse struct {
	SensitiveWords []string `json:"sensitive_words"`
	Content        string   `json:"content"`
}
