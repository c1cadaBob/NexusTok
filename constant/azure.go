// Package constant - azure.go
// 该文件定义了 Azure OpenAI 相关的常量
package constant

import "time"

// AzureNoRemoveDotTime Azure API 版本切换时间点
//
// 在此时间之后，Azure OpenAI 的 API 版本不再需要移除模型名称中的点号
// 用于兼容不同版本的 Azure OpenAI API 行为
//
// 背景：某些旧版本的 Azure API 要求模型名称中不能包含点号（如 gpt-4 → gpt-4）
// 新版本已取消此限制
var AzureNoRemoveDotTime = time.Date(2025, time.May, 10, 0, 0, 0, 0, time.UTC).Unix()
