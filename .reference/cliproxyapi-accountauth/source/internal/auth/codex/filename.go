// codex - filename.go
// 包 codex 提供 OpenAI Codex API 的认证和令牌管理功能。
// 该文件负责生成 Codex OAuth 凭证的持久化文件名，
// 使用电子邮件地址和计划类型作为后缀来区分不同的账户和订阅。
package codex

import (
	"fmt"
	"strings"
	"unicode"
)

// CredentialFileName 返回用于持久化 Codex OAuth 凭证的文件名。
// 当 planType 可用时（如 "plus"、"team"），会附加在电子邮件后面作为后缀，
// 用于区分不同的订阅类型。
//
// 参数：
//   - email: 用户的电子邮件地址
//   - planType: 订阅计划类型（如 "plus"、"team"）
//   - hashAccountID: 哈希后的账户 ID
//   - includeProviderPrefix: 是否包含提供商前缀
//
// 返回：
//   - string: 凭证文件的完整文件名
func CredentialFileName(email, planType, hashAccountID string, includeProviderPrefix bool) string {
	email = strings.TrimSpace(email)
	plan := normalizePlanTypeForFilename(planType)

	prefix := ""
	if includeProviderPrefix {
		prefix = "codex"
	}

	if plan == "" {
		return fmt.Sprintf("%s-%s.json", prefix, email)
	} else if plan == "team" {
		return fmt.Sprintf("%s-%s-%s-%s.json", prefix, hashAccountID, email, plan)
	}
	return fmt.Sprintf("%s-%s-%s.json", prefix, email, plan)
}

// normalizePlanTypeForFilename 规范化计划类型以用于文件名。
// 将计划类型字符串转换为小写，使用连字符分隔，并移除特殊字符。
//
// 参数：
//   - planType: 原始计划类型字符串
//
// 返回：
//   - string: 规范化后的计划类型字符串
func normalizePlanTypeForFilename(planType string) string {
	planType = strings.TrimSpace(planType)
	if planType == "" {
		return ""
	}

	parts := strings.FieldsFunc(planType, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(parts) == 0 {
		return ""
	}

	for i, part := range parts {
		parts[i] = strings.ToLower(strings.TrimSpace(part))
	}
	return strings.Join(parts, "-")
}
