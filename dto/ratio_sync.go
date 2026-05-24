// Package dto - ratio_sync.go
// 该文件定义了渠道配置同步相关的数据传输对象
//
// 主要结构体：
// - UpstreamDTO：上游渠道配置
// - UpstreamRequest：上游同步请求（批量渠道 ID + 上游配置）
// - TestResult：上游连通性测试结果
// - DifferenceItem：配置差异项（本地值 vs 各上游值）
// - SyncableChannel：可同步的渠道信息
//
// 用途：支持从多个上游实例同步渠道配置，比较差异并选择性应用
package dto

// UpstreamDTO 上游渠道配置
// ID：渠道 ID（可选，更新时使用）
// Name：渠道名称（必填）
// BaseURL：上游基础 URL（必填）
// Endpoint：端点路径
type UpstreamDTO struct {
	ID       int    `json:"id,omitempty"`
	Name     string `json:"name" binding:"required"`
	BaseURL  string `json:"base_url" binding:"required"`
	Endpoint string `json:"endpoint"`
}

// UpstreamRequest 上游同步请求
// ChannelIDs：要同步的渠道 ID 列表
// Upstreams：上游实例列表（支持多源同步）
// Timeout：请求超时时间（秒）
type UpstreamRequest struct {
	ChannelIDs []int64       `json:"channel_ids"`
	Upstreams  []UpstreamDTO `json:"upstreams"`
	Timeout    int           `json:"timeout"`
}

// TestResult 上游测试连通性结果
type TestResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// DifferenceItem 差异项
// Current 为本地值，可能为 nil
// Upstreams 为各渠道的上游值，具体数值 / "same" / nil

type DifferenceItem struct {
	Current    interface{}            `json:"current"`
	Upstreams  map[string]interface{} `json:"upstreams"`
	Confidence map[string]bool        `json:"confidence"`
}

// SyncableChannel 可同步的渠道信息
// ID：渠道 ID
// Name：渠道名称
// BaseURL：上游基础 URL
// Status：渠道状态
// Type：渠道类型
type SyncableChannel struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Status  int    `json:"status"`
	Type    int    `json:"type"`
}
