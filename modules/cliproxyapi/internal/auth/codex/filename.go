// codex - filename.go
// 提供 Codex 认证凭据文件名的生成逻辑。
// 根据邮箱、订阅计划类型和账号 ID 生成唯一的文件名。
package codex

import (
	"fmt"
	"strings"
	"unicode"
)

// CredentialFileName 生成 Codex OAuth 认证凭据的持久化文件名。
// 当 planType 可用时（如 "plus"、"team"），会附加在邮箱后作为后缀以区分不同订阅。
// team 计划会额外包含 hashAccountID 以确保唯一性。
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

// normalizePlanTypeForFilename 规范化订阅计划类型以用于文件名。
// 移除所有非字母数字字符，将结果转换为小写并用连字符连接。
// 例如 "Pro Plus" -> "pro-plus"。
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
