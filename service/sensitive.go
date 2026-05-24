// sensitive.go - 敏感词检测与替换服务
// 本文件提供消息内容的敏感词检测和替换功能。
// 基于 Aho-Corasick 多模式匹配算法，支持对聊天消息和纯文本进行敏感词过滤。
// 敏感词列表由 setting.SensitiveWords 配置，匹配时忽略大小写。
package service

import (
	"errors"
	"strings"

	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/setting"
)

// CheckSensitiveMessages 检查消息列表中是否包含敏感词。
// 遍历所有消息的文本内容（跳过图片类型），逐条进行敏感词检测。
// 参数:
//   - messages: 待检查的消息列表（dto.Message 数组）
// 返回值:
//   - []string: 检测到的敏感词列表（未检测到时为 nil）
//   - error: 检测到敏感词时返回 "sensitive words detected" 错误
func CheckSensitiveMessages(messages []dto.Message) ([]string, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	for _, message := range messages {
		arrayContent := message.ParseContent()
		for _, m := range arrayContent {
			if m.Type == "image_url" {
				// TODO: check image url
				continue
			}
			// 检查 text 是否为空
			if m.Text == "" {
				continue
			}
			if ok, words := SensitiveWordContains(m.Text); ok {
				return words, errors.New("sensitive words detected")
			}
		}
	}
	return nil, nil
}

// CheckSensitiveText 检查纯文本是否包含敏感词。
// 这是 SensitiveWordContains 的便捷封装。
// 参数:
//   - text: 待检查的文本内容
// 返回值:
//   - bool: 是否包含敏感词
//   - []string: 检测到的敏感词列表
func CheckSensitiveText(text string) (bool, []string) {
	return SensitiveWordContains(text)
}

// SensitiveWordContains 是否包含敏感词，返回是否包含敏感词和敏感词列表
func SensitiveWordContains(text string) (bool, []string) {
	if len(setting.SensitiveWords) == 0 {
		return false, nil
	}
	if len(text) == 0 {
		return false, nil
	}
	checkText := strings.ToLower(text)
	return AcSearch(checkText, setting.SensitiveWords, true)
}

// SensitiveWordReplace 敏感词替换，返回是否包含敏感词和替换后的文本
func SensitiveWordReplace(text string, returnImmediately bool) (bool, []string, string) {
	if len(setting.SensitiveWords) == 0 {
		return false, nil, text
	}
	checkText := strings.ToLower(text)
	m := getOrBuildAC(setting.SensitiveWords)
	hits := m.MultiPatternSearch([]rune(checkText), returnImmediately)
	if len(hits) > 0 {
		words := make([]string, 0, len(hits))
		var builder strings.Builder
		builder.Grow(len(text))
		lastPos := 0

		for _, hit := range hits {
			pos := hit.Pos
			word := string(hit.Word)
			builder.WriteString(text[lastPos:pos])
			builder.WriteString("**###**")
			lastPos = pos + len(word)
			words = append(words, word)
		}
		builder.WriteString(text[lastPos:])
		return true, words, builder.String()
	}
	return false, nil, text
}
