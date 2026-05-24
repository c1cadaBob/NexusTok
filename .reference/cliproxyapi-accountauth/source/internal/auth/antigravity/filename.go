// antigravity - filename.go
// 包 antigravity 提供 Antigravity 提供商的 OAuth2 认证功能。
// 该文件负责生成 Antigravity 凭证的持久化文件名，
// 使用电子邮件地址作为后缀来区分不同的账户。
package antigravity

import (
	"fmt"
	"strings"
)

// CredentialFileName 返回用于持久化 Antigravity 凭证的文件名。
// 使用用户的电子邮件地址作为文件名后缀，以便区分不同账户的凭证文件。
// 如果电子邮件为空，则使用默认的文件名 "antigravity.json"。
//
// 参数：
//   - email: 用户的电子邮件地址
//
// 返回：
//   - string: 凭证文件的完整文件名
func CredentialFileName(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return "antigravity.json"
	}
	return fmt.Sprintf("antigravity-%s.json", email)
}
