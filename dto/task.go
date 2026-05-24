// Package dto - task.go
// 该文件定义了异步任务相关的数据传输对象
//
// 主要结构体：
// - TaskError：任务错误信息
// - TaskResponse：任务响应（泛型）
// - TaskDto：任务详情（用于任务列表和查询）
// - FetchReq：批量查询请求
//
// 任务状态：submitted -> queueing -> processing -> success/failed
// 任务平台：Suno 音乐生成、视频生成等
package dto

import (
	"encoding/json"
)

// TaskError 任务错误信息
// Code：错误代码
// Message：错误消息
// Data：附加错误数据
// StatusCode：HTTP 状态码（不序列化）
// LocalError：是否为本地错误（不序列化）
// Error：原始错误对象（不序列化）
type TaskError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Data       any    `json:"data"`
	StatusCode int    `json:"-"`
	LocalError bool   `json:"-"`
	Error      error  `json:"-"`
}

// TaskData 任务数据类型约束（泛型接口）
// 支持 SunoDataResponse、[]SunoDataResponse、string 或任意类型
type TaskData interface {
	SunoDataResponse | []SunoDataResponse | string | any
}

// TaskSuccessCode 任务成功状态码
const TaskSuccessCode = "success"

// TaskResponse 任务响应（泛型）
// Code：响应状态码（"success" 表示成功）
// Message：响应消息
// Data：响应数据（泛型，取决于任务类型）
type TaskResponse[T TaskData] struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// IsSuccess 判断任务是否成功
func (t *TaskResponse[T]) IsSuccess() bool {
	return t.Code == TaskSuccessCode
}

// TaskDto 任务详情
// ID：数据库自增 ID
// CreatedAt/UpdatedAt：创建/更新时间戳
// TaskID：任务唯一标识
// Platform：任务平台（suno/video 等）
// UserId：用户 ID
// Group：用户组
// ChannelId：使用的渠道 ID
// Quota：消耗的配额
// Action：任务动作（song/lyrics 等）
// Status：任务状态（submitted/queueing/processing/success/failed）
// FailReason：失败原因
// ResultURL：任务结果 URL（视频地址等）
// SubmitTime/StartTime/FinishTime：提交/开始/完成时间戳
// Progress：进度百分比
// Properties：扩展属性
// Username：用户名（关联查询时填充）
// Data：原始任务数据（JSON 格式）
type TaskDto struct {
	ID         int64           `json:"id"`
	CreatedAt  int64           `json:"created_at"`
	UpdatedAt  int64           `json:"updated_at"`
	TaskID     string          `json:"task_id"`
	Platform   string          `json:"platform"`
	UserId     int             `json:"user_id"`
	Group      string          `json:"group"`
	ChannelId  int             `json:"channel_id"`
	Quota      int             `json:"quota"`
	Action     string          `json:"action"`
	Status     string          `json:"status"`
	FailReason string          `json:"fail_reason"`
	ResultURL  string          `json:"result_url,omitempty"` // 任务结果 URL（视频地址等）
	SubmitTime int64           `json:"submit_time"`
	StartTime  int64           `json:"start_time"`
	FinishTime int64           `json:"finish_time"`
	Progress   string          `json:"progress"`
	Properties any             `json:"properties"`
	Username   string          `json:"username,omitempty"`
	Data       json.RawMessage `json:"data"`
}

// FetchReq 批量查询请求
// IDs：任务 ID 列表
type FetchReq struct {
	IDs []string `json:"ids"`
}
