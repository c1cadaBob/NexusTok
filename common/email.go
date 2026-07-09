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
// - NTLM 认证（用于只开放 AUTH NTLM 的企业 SMTP/Exchange 服务器）
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
// 根据服务器能力和历史兼容配置自适应选择认证方式：
// - LOGIN 认证：用于强制 LOGIN、Outlook 或历史白名单服务器
// - NTLM 认证：用于只开放 AUTH NTLM 的企业 SMTP/Exchange 服务器
// - PLAIN 认证：用于大多数标准 SMTP 服务器
//
// 返回值：
//   - smtp.Auth: SMTP 认证对象
func getSMTPAuth() smtp.Auth {
	return AutoSMTPAuth(SMTPAccount, SMTPToken)
}

// shouldAuthenticateSMTP 判断当前配置是否需要执行 SMTP AUTH。
//
// 部分企业内网 SMTP relay 只要求合法发件地址，不要求账号认证；账号或令牌任一为空时跳过
// AUTH，可以避免向不支持认证的 relay 发送无效 AUTH 命令。账号与令牌都存在时继续复用
// AutoSMTPAuth 的 PLAIN/LOGIN/NTLM 自适应逻辑。
func shouldAuthenticateSMTP() bool {
	return SMTPAccount != "" && SMTPToken != ""
}

// smtpTLSConfig 生成 SMTP 连接专用 TLS 配置。
//
// 默认校验服务端证书，只有管理员显式开启 SMTPInsecureSkipVerify 时才跳过校验；该兼容开关
// 主要用于自签名证书、内网域名与证书名称不一致的历史 SMTP 服务。
func smtpTLSConfig() *tls.Config {
	return &tls.Config{
		ServerName:         SMTPServer,
		InsecureSkipVerify: SMTPInsecureSkipVerify, // #nosec G402 -- 管理员显式配置用于兼容自签名 SMTP 证书。
	}
}

// newSMTPClient 根据当前 SMTP 配置创建客户端连接。
//
// 连接策略：
// 1. SMTPSSLEnabled=true 或端口为 465 且未显式启用 STARTTLS 时，使用传统隐式 TLS。
// 2. SMTPStartTLSEnabled=true 时，先建立明文 SMTP 连接，再要求服务端声明并执行 STARTTLS。
// 3. 其他场景保持普通 SMTP 连接，由上游 relay 或本地网络策略承担安全边界。
func newSMTPClient(addr string) (*smtp.Client, error) {
	if SMTPSSLEnabled || (SMTPPort == 465 && !SMTPStartTLSEnabled) {
		conn, err := tls.Dial("tcp", addr, smtpTLSConfig())
		if err != nil {
			return nil, err
		}
		client, err := smtp.NewClient(conn, SMTPServer)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return client, nil
	}

	client, err := smtp.Dial(addr)
	if err != nil {
		return nil, err
	}

	if SMTPStartTLSEnabled {
		startTLSSupported, _ := client.Extension("STARTTLS")
		if !startTLSSupported {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(smtpTLSConfig()); err != nil {
			_ = client.Close()
			return nil, err
		}
	}

	return client, nil
}

// SendEmail 发送邮件
//
// 支持两种连接方式：
// - SSL/TLS（端口 465）：直接建立 TLS 连接
// - STARTTLS：管理员显式开启时，先建立普通连接，再升级为 TLS
// - 普通 SMTP：未启用 TLS/STARTTLS 时保持明文连接，适用于本地或内网 relay
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
	client, err := newSMTPClient(addr)
	if err != nil {
		return err
	}
	defer client.Close()
	if shouldAuthenticateSMTP() {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(SMTPFrom); err != nil {
		return err
	}
	for _, receiver := range to {
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
	err = client.Quit()
	if err != nil {
		SysError(fmt.Sprintf("failed to send email to %s: %v", receiver, err))
	}
	return err
}
