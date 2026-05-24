// Package taskcommon 提供任务渠道的通用辅助函数和基础计费实现
package taskcommon

import (
	"encoding/base64" // Base64 编解码，用于任务 ID 编码
	"fmt"             // 格式化输出

	"github.com/c1cada/NexusTok/common"              // 公共工具包
	"github.com/c1cada/NexusTok/model"                // 数据模型
	relaycommon "github.com/c1cada/NexusTok/relay/common" // 中继通用类型
	"github.com/c1cada/NexusTok/setting/system_setting" // 系统设置
	"github.com/gin-gonic/gin"                        // Gin Web 框架
)

// UnmarshalMetadata 将 map[string]any 类型的元数据转换为指定结构体
// 通过 JSON 序列化/反序列化实现类型转换
// 防止元数据覆盖 model 字段以避免计费绕过
//
// 参数：
//   - metadata: 原始元数据映射
//   - target: 目标结构体指针
//
// 返回值：
//   - error: 转换过程中的错误
func UnmarshalMetadata(metadata map[string]any, target any) error {
	if metadata == nil {
		return nil
	}
	// 防止元数据覆盖 model 字段，避免计费绕过
	delete(metadata, "model")
	metaBytes, err := common.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata failed: %w", err)
	}
	if err := common.Unmarshal(metaBytes, target); err != nil {
		return fmt.Errorf("unmarshal metadata failed: %w", err)
	}
	return nil
}

// DefaultString 返回非空值，否则返回默认值
func DefaultString(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

// DefaultInt 返回非零值，否则返回默认值
func DefaultInt(val, fallback int) int {
	if val == 0 {
		return fallback
	}
	return val
}

// EncodeLocalTaskID 将上游操作名称编码为 URL 安全的 Base64 字符串
// 用于 Gemini/Vertex 将上游操作名称存储为任务 ID
func EncodeLocalTaskID(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name))
}

// DecodeLocalTaskID 解码 Base64 编码的上游操作名称
func DecodeLocalTaskID(id string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// BuildProxyURL 构建视频代理 URL
// 使用公共任务 ID 构建代理地址，例如: "https://your-server.com/v1/videos/task_xxxx/content"
func BuildProxyURL(taskID string) string {
	return fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, taskID)
}

// 状态到进度的映射常量，用于轮询更新
const (
	ProgressSubmitted  = "10%"  // 已提交
	ProgressQueued     = "20%"  // 排队中
	ProgressInProgress = "30%"  // 处理中
	ProgressComplete   = "100%" // 已完成
)

// ---------------------------------------------------------------------------
// BaseBilling — 可嵌入的空操作计费方法实现
// 不需要自定义计费的适配器可以直接嵌入此结构体
// ---------------------------------------------------------------------------

// BaseBilling 基础计费结构体
// 提供空操作的计费方法实现，适配器可以嵌入此结构体以获得默认行为
type BaseBilling struct{}

// EstimateBilling 返回 nil（无额外比率，使用基础模型价格）
func (BaseBilling) EstimateBilling(_ *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	return nil
}

// AdjustBillingOnSubmit 返回 nil（提交时无调整）
func (BaseBilling) AdjustBillingOnSubmit(_ *relaycommon.RelayInfo, _ []byte) map[string]float64 {
	return nil
}

// AdjustBillingOnComplete 返回 0（保持预扣金额）
func (BaseBilling) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}
