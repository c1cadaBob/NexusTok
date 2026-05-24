// 本文件 (task.go) 提供任务相关的工具函数，
// 负责将任务平台和动作名称转换为统一的模型名称格式。
package service

import (
	"strings" // 字符串操作

	"github.com/c1cada/NexusTok/constant" // 项目常量定义
)

// CoverTaskActionToModelName 将任务平台和动作名称组合转换为模型名称。
// 转换规则：平台名称（小写）+ "_" + 动作名称（小写）
// 例如：platform="MIDJOURNEY", action="IMAGINE" => "midjourney_imagine"
// 参数:
//   - platform: 任务平台标识（如 MIDJOURNEY 等）
//   - action: 任务动作名称（如 IMAGINE、DESCRIBE 等）
// 返回值:
//   - string: 组合后的小写模型名称，格式为 "platform_action"
func CoverTaskActionToModelName(platform constant.TaskPlatform, action string) string {
	return strings.ToLower(string(platform)) + "_" + strings.ToLower(action)
}
