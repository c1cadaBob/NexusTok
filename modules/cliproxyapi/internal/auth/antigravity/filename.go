// antigravity - filename.go
// 提供 Antigravity 认证凭据文件名的生成逻辑。
package antigravity

import (
	"fmt"
	"strings"
)

// CredentialFileName 生成 Antigravity 认证凭据的持久化文件名。
// 使用邮箱作为后缀以区分不同账号。
// 如果邮箱为空，则返回默认文件名 "antigravity.json"。
func CredentialFileName(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return "antigravity.json"
	}
	return fmt.Sprintf("antigravity-%s.json", email)
}
