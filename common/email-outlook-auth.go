// Package common - email-outlook-auth.go
// 该文件实现了 SMTP LOGIN 认证方式
//
// 标准的 SMTP 认证方式是 PLAIN（smtp.PlainAuth）
// 但某些邮件服务器（如 Outlook、Exchange）要求使用 LOGIN 认证
//
// LOGIN 认证流程：
// 1. 客户端发送 AUTH LOGIN 命令
// 2. 服务器返回 "Username:" 提示
// 3. 客户端发送 Base64 编码的用户名
// 4. 服务器返回 "Password:" 提示
// 5. 客户端发送 Base64 编码的密码
package common

import (
	"errors"
	"net/smtp"
	"strings"
)

// outlookAuth Outlook/LOGIN 认证结构体
//
// 实现 smtp.Auth 接口
// 使用 LOGIN 认证方式（而非 PLAIN）
type outlookAuth struct {
	username, password string // 用户名和密码
}

// LoginAuth 创建 LOGIN 认证对象
//
// 用于 Outlook、Exchange 等要求 LOGIN 认证的邮件服务器
//
// 参数：
//   - username: SMTP 用户名
//   - password: SMTP 密码
//
// 返回值：
//   - smtp.Auth: SMTP 认证对象
func LoginAuth(username, password string) smtp.Auth {
	return &outlookAuth{username, password}
}

// Start 启动 LOGIN 认证
//
// 返回认证机制名称 "LOGIN" 和空的初始响应
//
// 参数：
//   - _: 服务器信息（未使用）
//
// 返回值：
//   - string: 认证机制名称 "LOGIN"
//   - []byte: 初始响应（空）
//   - error: 错误（始终为 nil）
func (a *outlookAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

// Next 处理认证交互的下一步
//
// 根据服务器的提示返回对应的凭据：
// - "Username:" → 返回用户名
// - "Password:" → 返回密码
//
// 参数：
//   - fromServer: 服务器返回的提示信息
//   - more: 是否需要更多数据
//
// 返回值：
//   - []byte: 响应数据（用户名或密码）
//   - error: 错误
func (a *outlookAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("unknown fromServer")
		}
	}
	return nil, nil
}

// isOutlookServer 判断是否为 Outlook 邮箱服务器
//
// 通过检查服务器地址中是否包含 "outlook" 或 "onmicrosoft" 来判断
// 兼容多地区的 Outlook 邮箱和 OFB 邮箱
//
// 参数：
//   - server: SMTP 服务器地址或账号
//
// 返回值：
//   - bool: 是否为 Outlook 服务器
func isOutlookServer(server string) bool {
	return strings.Contains(server, "outlook") || strings.Contains(server, "onmicrosoft")
}
