// Package common - email.go
// 该文件实现了邮件发送功能
//
// 支持的邮件发送方式：
// - 普通 SMTP（端口 587，STARTTLS）
// - SMTP SSL（端口 465，直接 TLS 连接）
//
// 支持的认证方式：
// - PLAIN 认证（smtp.PlainAuth）
// - LOGIN 认证（自定义 LoginAuth，用于 Outlook 等特殊服务器）
//
// 使用场景：
// - 用户注册邮箱验证
// - 密码重置通知
// - 配额预警通知
// - 系统告警通知
package common

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"slices"
	"strings"
	"time"
)

// generateMessageID 生成邮件 Message-ID 头
//
// Message-ID 是邮件的唯一标识符，格式为：<时间戳.随机字符串@域名>
// 用于邮件客户端识别和去重
//
// 返回值：
//   - string: Message-ID 字符串
//   - error: 生成错误（如 SMTPFrom 格式无效）
func generateMessageID() (string, error) {
	split := strings.Split(SMTPFrom, "@")
	if len(split) < 2 {
		return "", fmt.Errorf("invalid SMTP account")
	}
	domain := strings.Split(SMTPFrom, "@")[1]
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), GetRandomString(12), domain), nil
}

// shouldUseSMTPLoginAuth 判断是否应该使用 SMTP LOGIN 认证
//
// 以下情况使用 LOGIN 认证：
// 1. 强制使用 LOGIN 认证（SMTPForceAuthLogin = true）
// 2. Outlook 邮箱服务器
// 3. 在 EmailLoginAuthServerList 中的服务器
//
// 返回值：
//   - bool: 是否使用 LOGIN 认证
func shouldUseSMTPLoginAuth() bool {
	if SMTPForceAuthLogin {
		return true
	}
	return isOutlookServer(SMTPAccount) || slices.Contains(EmailLoginAuthServerList, SMTPServer)
}

// getSMTPAuth 获取 SMTP 认证对象
//
// 根据服务器类型选择认证方式：
// - LOGIN 认证：用于 Outlook 等特殊服务器
// - PLAIN 认证：用于大多数标准 SMTP 服务器
//
// 返回值：
//   - smtp.Auth: SMTP 认证对象
func getSMTPAuth() smtp.Auth {
	if shouldUseSMTPLoginAuth() {
		return LoginAuth(SMTPAccount, SMTPToken)
	}
	return smtp.PlainAuth("", SMTPAccount, SMTPToken, SMTPServer)
}

// SendEmail 发送邮件
//
// 支持两种连接方式：
// - SSL/TLS（端口 465）：直接建立 TLS 连接
// - STARTTLS（端口 587）：先建立普通连接，再升级为 TLS
//
// 邮件格式：
// - Content-Type: text/html; charset=UTF-8
// - Subject: Base64 编码（支持中文主题）
// - 支持多个收件人（用分号分隔）
//
// 参数：
//   - subject: 邮件主题
//   - receiver: 收件人邮箱（多个用分号分隔）
//   - content: 邮件内容（HTML 格式）
//
// 返回值：
//   - error: 发送错误
func SendEmail(subject string, receiver string, content string) error {
	if SMTPFrom == "" { // 兼容旧配置，如果未设置 SMTPFrom 则使用 SMTPAccount
		SMTPFrom = SMTPAccount
	}
	id, err2 := generateMessageID()
	if err2 != nil {
		return err2
	}
	if SMTPServer == "" && SMTPAccount == "" {
		return fmt.Errorf("SMTP 服务器未配置")
	}
	// 主题使用 Base64 编码，支持中文
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	// 构建邮件内容（RFC 5322 格式）
	mail := []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s <%s>\r\n"+
		"Subject: %s\r\n"+
		"Date: %s\r\n"+
		"Message-ID: %s\r\n"+ // 添加 Message-ID 头
		"Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n",
		receiver, SystemName, SMTPFrom, encodedSubject, time.Now().Format(time.RFC1123Z), id, content))
	auth := getSMTPAuth()
	addr := fmt.Sprintf("%s:%d", SMTPServer, SMTPPort)
	to := strings.Split(receiver, ";")
	var err error
	if SMTPPort == 465 || SMTPSSLEnabled {
		// SSL/TLS 连接（端口 465）
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         SMTPServer,
		}
		conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", SMTPServer, SMTPPort), tlsConfig)
		if err != nil {
			return err
		}
		client, err := smtp.NewClient(conn, SMTPServer)
		if err != nil {
			return err
		}
		defer client.Close()
		if err = client.Auth(auth); err != nil {
			return err
		}
		if err = client.Mail(SMTPFrom); err != nil {
			return err
		}
		receiverEmails := strings.Split(receiver, ";")
		for _, receiver := range receiverEmails {
			if err = client.Rcpt(receiver); err != nil {
				return err
			}
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = w.Write(mail)
		if err != nil {
			return err
		}
		err = w.Close()
		if err != nil {
			return err
		}
	} else {
		// STARTTLS 连接（端口 587）
		err = smtp.SendMail(addr, auth, SMTPFrom, to, mail)
	}
	if err != nil {
		SysError(fmt.Sprintf("failed to send email to %s: %v", receiver, err))
	}
	return err
}
