// Package setting - sensitive.go
// 该文件管理敏感词过滤的配置
//
// 功能：
// - 控制是否启用敏感词检测
// - 管理敏感词列表
// - 配置检测行为（停止生成或替换敏感词）
// - 流式响应的缓存队列长度配置
package setting

import "strings"

// CheckSensitiveEnabled 是否启用敏感词检测总开关
var CheckSensitiveEnabled = true

// CheckSensitiveOnPromptEnabled 是否在用户提示词阶段检测敏感词
var CheckSensitiveOnPromptEnabled = true

//var CheckSensitiveOnCompletionEnabled = true

// StopOnSensitiveEnabled 检测到敏感词时的行为控制
// true: 立刻停止生成
// false: 替换敏感词后继续生成
var StopOnSensitiveEnabled = true

// StreamCacheQueueLength 流模式缓存队列长度
// 0 表示无缓存，直接输出
// 大于 0 时启用缓存队列，用于在检测到敏感词时回溯处理
var StreamCacheQueueLength = 0

// SensitiveWords 敏感词列表
// 支持通过管理后台动态添加和删除
var SensitiveWords = []string{
	"test_sensitive",
}

// SensitiveWordsToString 将敏感词列表转换为换行分隔的字符串
//
// 返回值：
//   - string: 敏感词字符串（每行一个）
func SensitiveWordsToString() string {
	return strings.Join(SensitiveWords, "\n")
}

// SensitiveWordsFromString 从换行分隔的字符串解析敏感词列表
//
// 参数：
//   - s: 敏感词字符串（每行一个）
func SensitiveWordsFromString(s string) {
	SensitiveWords = []string{}
	sw := strings.Split(s, "\n")
	for _, w := range sw {
		w = strings.TrimSpace(w)
		if w != "" {
			SensitiveWords = append(SensitiveWords, w)
		}
	}
}

// ShouldCheckPromptSensitive 判断是否应该在提示词阶段检测敏感词
//
// 返回值：
//   - bool: 当总开关和提示词检测开关都启用时返回 true
func ShouldCheckPromptSensitive() bool {
	return CheckSensitiveEnabled && CheckSensitiveOnPromptEnabled
}

//func ShouldCheckCompletionSensitive() bool {
//	return CheckSensitiveEnabled && CheckSensitiveOnCompletionEnabled
//}
