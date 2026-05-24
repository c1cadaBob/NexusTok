// user.go 实现了 WebAuthn 用户接口适配器。
// 将系统内部的 User 和 PasskeyCredential 模型适配为 go-webauthn 库所需的
// WebAuthnUser 接口，用于 Passkey (WebAuthn) 认证流程。
package passkey

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/model" // 数据模型：User、PasskeyCredential

	webauthn "github.com/go-webauthn/webauthn/webauthn" // WebAuthn 库
)

// WebAuthnUser 将内部用户模型适配为 WebAuthn 用户接口
type WebAuthnUser struct {
	user       *model.User              // 内部用户模型
	credential *model.PasskeyCredential // 用户的 Passkey 凭证
}

// NewWebAuthnUser 创建一个新的 WebAuthnUser 适配器实例。
//
// 参数：
//   - user: 内部用户模型
//   - credential: 用户的 Passkey 凭证
//
// 返回：
//   - *WebAuthnUser: 适配器实例
func NewWebAuthnUser(user *model.User, credential *model.PasskeyCredential) *WebAuthnUser {
	return &WebAuthnUser{user: user, credential: credential}
}

// WebAuthnID 返回用户的唯一标识（字节数组格式）。
// 使用用户 ID 的字符串表示作为 WebAuthn 用户句柄。
//
// 返回：
//   - []byte: 用户 ID 的字节表示
func (u *WebAuthnUser) WebAuthnID() []byte {
	if u == nil || u.user == nil {
		return nil
	}
	return []byte(strconv.Itoa(u.user.Id))
}

// WebAuthnName 返回用户的唯一名称（用于 WebAuthn 凭证标识）。
// 优先使用用户名，若为空则生成 "user-{id}" 格式。
//
// 返回：
//   - string: 用户唯一名称
func (u *WebAuthnUser) WebAuthnName() string {
	if u == nil || u.user == nil {
		return ""
	}
	name := strings.TrimSpace(u.user.Username)
	if name == "" {
		return fmt.Sprintf("user-%d", u.user.Id)
	}
	return name
}

// WebAuthnDisplayName 返回用户的显示名称（用于认证器界面展示）。
// 优先使用 DisplayName，若为空则回退到 WebAuthnName。
//
// 返回：
//   - string: 用户显示名称
func (u *WebAuthnUser) WebAuthnDisplayName() string {
	if u == nil || u.user == nil {
		return ""
	}
	display := strings.TrimSpace(u.user.DisplayName)
	if display != "" {
		return display
	}
	return u.WebAuthnName()
}

// WebAuthnCredentials 返回用户的 WebAuthn 凭证列表。
// 将内部的 PasskeyCredential 转换为 go-webauthn 库的 Credential 格式。
//
// 返回：
//   - []webauthn.Credential: WebAuthn 凭证列表
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	if u == nil || u.credential == nil {
		return nil
	}
	cred := u.credential.ToWebAuthnCredential()
	return []webauthn.Credential{cred}
}

// ModelUser 返回底层的内部用户模型。
//
// 返回：
//   - *model.User: 用户模型，nil 表示 WebAuthnUser 本身为 nil
func (u *WebAuthnUser) ModelUser() *model.User {
	if u == nil {
		return nil
	}
	return u.user
}

// PasskeyCredential 返回底层的 Passkey 凭证模型。
//
// 返回：
//   - *model.PasskeyCredential: Passkey 凭证模型
func (u *WebAuthnUser) PasskeyCredential() *model.PasskeyCredential {
	if u == nil {
		return nil
	}
	return u.credential
}
