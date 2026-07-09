// Package common - email_ntlm_auth.go
// 该文件实现 SMTP 认证方式的自适应选择。
//
// 背景：
// - 常规邮件服务商通常支持 AUTH PLAIN；
// - Outlook/Exchange 一类服务器可能要求 AUTH LOGIN；
// - 部分企业内网或 Microsoft 365 SMTP 只开放 AUTH NTLM。
//
// AutoSMTPAuth 会在 smtp.Client.Auth 调用 Start 时读取服务器 EHLO 返回的
// ServerInfo.Auth，并在不改变现有配置模型的前提下选择可用机制。
package common

import (
	"errors"
	"net/smtp"
	"strings"

	ntlmssp "github.com/Azure/go-ntlmssp"
)

// smtpAutoAuth 根据服务器声明的 SMTP AUTH 机制自动选择 PLAIN、LOGIN 或 NTLM。
//
// mech 记录 Start 阶段选中的认证机制，供后续 Next 阶段响应服务器挑战。
type smtpAutoAuth struct {
	username string
	password string
	mech     string
}

// AutoSMTPAuth 创建自适应 SMTP 认证对象。
//
// 参数：
//   - username: SMTP 用户名
//   - password: SMTP 密码或应用令牌
//
// 返回值：
//   - smtp.Auth: 可传给 smtp.Client.Auth 的认证对象
func AutoSMTPAuth(username, password string) smtp.Auth {
	return &smtpAutoAuth{username: username, password: password}
}

// Start 根据 SMTP 服务器能力选择认证机制。
//
// 选择规则：
// 1. 管理员显式开启 SMTPForceAuthLogin 时强制 LOGIN；
// 2. Outlook/历史白名单服务器优先 LOGIN，但服务器只声明 NTLM 时改用 NTLM；
// 3. 常规服务器优先 PLAIN，其次 LOGIN，再其次 NTLM；
// 4. 服务器未声明机制时回退 PLAIN，保持旧行为。
func (a *smtpAutoAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	useLoginAuth := SMTPForceAuthLogin
	if !useLoginAuth && shouldUseSMTPLoginAuth() {
		useLoginAuth = !(server != nil && len(server.Auth) == 1 && smtpServerSupportsAuth(server, "NTLM"))
	}
	if useLoginAuth {
		a.mech = "LOGIN"
		return "LOGIN", []byte{}, nil
	}

	switch {
	case smtpServerSupportsAuth(server, "PLAIN"):
		a.mech = "PLAIN"
		return smtp.PlainAuth("", a.username, a.password, SMTPServer).Start(server)
	case smtpServerSupportsAuth(server, "LOGIN"):
		a.mech = "LOGIN"
		return "LOGIN", []byte{}, nil
	case smtpServerSupportsAuth(server, "NTLM"):
		a.mech = "NTLM"
		negotiateMessage, err := ntlmssp.NewNegotiateMessage("", "")
		if err != nil {
			return "", nil, err
		}
		return "NTLM", negotiateMessage, nil
	default:
		a.mech = "PLAIN"
		return smtp.PlainAuth("", a.username, a.password, SMTPServer).Start(server)
	}
}

// Next 响应服务器的后续认证挑战。
//
// LOGIN 使用明文用户名/密码响应服务器挑战；NTLM 使用 go-ntlmssp 根据服务端
// challenge 生成 authenticate message。PLAIN 不应进入 Next 阶段。
func (a *smtpAutoAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}

	switch a.mech {
	case "LOGIN":
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("unknown SMTP AUTH LOGIN challenge")
		}
	case "NTLM":
		return ntlmssp.NewAuthenticateMessage(fromServer, a.username, a.password, nil)
	default:
		return nil, errors.New("unexpected SMTP auth challenge")
	}
}

// smtpServerSupportsAuth 判断服务器 EHLO 返回的机制列表中是否包含指定机制。
func smtpServerSupportsAuth(server *smtp.ServerInfo, mechanism string) bool {
	if server == nil {
		return false
	}
	for _, auth := range server.Auth {
		if strings.EqualFold(auth, mechanism) {
			return true
		}
	}
	return false
}
