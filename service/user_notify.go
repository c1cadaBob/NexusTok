// user_notify.go - 用户通知服务
// 本文件提供多渠道用户通知功能。
// 支持的通知渠道包括：邮件（Email）、Webhook、Bark（iOS 推送）、Gotify。
// 每种渠道都支持 Worker 代理模式和直连模式，直连模式下包含 SSRF 防护。
// 通知内容支持占位符替换和模板变量。
package service

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/system_setting"
)

// NotifyRootUser 向系统管理员（root 用户）发送通知。
// 自动获取 root 用户的信息并调用 NotifyUser 发送。
// 参数:
//   - t: 通知类型
//   - subject: 通知主题
//   - content: 通知内容
func NotifyRootUser(t string, subject string, content string) {
	user := model.GetRootUser().ToBaseUser()
	err := NotifyUser(user.Id, user.Email, user.GetSetting(), dto.NewNotify(t, subject, content, nil))
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to notify root user: %s", err.Error()))
	}
}

// NotifyUpstreamModelUpdateWatchers 向所有启用了上游模型更新通知的管理员发送通知。
// 查询所有启用状态的管理员用户，筛选出开启了 UpstreamModelUpdateNotifyEnabled 的用户，
// 逐个发送上游模型更新通知。
// 参数:
//   - subject: 通知主题
//   - content: 通知内容
func NotifyUpstreamModelUpdateWatchers(subject string, content string) {
	var users []model.User
	if err := model.DB.
		Select("id", "email", "role", "status", "setting").
		Where("status = ? AND role >= ?", common.UserStatusEnabled, common.RoleAdminUser).
		Find(&users).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to query upstream update notification users: %s", err.Error()))
		return
	}

	notification := dto.NewNotify(dto.NotifyTypeChannelUpdate, subject, content, nil)
	sentCount := 0
	for _, user := range users {
		userSetting := user.GetSetting()
		if !userSetting.UpstreamModelUpdateNotifyEnabled {
			continue
		}
		if err := NotifyUser(user.Id, user.Email, userSetting, notification); err != nil {
			common.SysLog(fmt.Sprintf("failed to notify user %d for upstream model update: %s", user.Id, err.Error()))
			continue
		}
		sentCount++
	}
	common.SysLog(fmt.Sprintf("upstream model update notifications sent: %d", sentCount))
}

// NotifyUser 向指定用户发送通知的统一入口。
// 根据用户的通知渠道设置，分发到对应的发送函数。
// 发送前会检查通知频率限制，避免短时间内发送过多通知。
// 支持的通知渠道：Email、Webhook、Bark、Gotify。
// 参数:
//   - userId: 用户 ID（用于通知频率限制检查）
//   - userEmail: 用户邮箱地址（Email 渠道使用）
//   - userSetting: 用户通知设置（包含渠道类型、Webhook URL 等）
//   - data: 通知数据（标题、内容、类型等）
//
// 返回值:
//   - error: 发送失败时返回错误
func NotifyUser(userId int, userEmail string, userSetting dto.UserSetting, data dto.Notify) error {
	notifyType := userSetting.NotifyType
	if notifyType == "" {
		notifyType = dto.NotifyTypeEmail
	}

	// Check notification limit
	canSend, err := CheckNotificationLimit(userId, data.Type)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to check notification limit: %s", err.Error()))
		return err
	}
	if !canSend {
		return fmt.Errorf("notification limit exceeded for user %d with type %s", userId, notifyType)
	}

	switch notifyType {
	case dto.NotifyTypeEmail:
		// 优先使用设置中的通知邮箱，如果为空则使用用户的默认邮箱
		emailToUse := userSetting.NotificationEmail
		if emailToUse == "" {
			emailToUse = userEmail
		}
		if emailToUse == "" {
			common.SysLog(fmt.Sprintf("user %d has no email, skip sending email", userId))
			return nil
		}
		return sendEmailNotify(emailToUse, data)
	case dto.NotifyTypeWebhook:
		webhookURLStr := userSetting.WebhookUrl
		if webhookURLStr == "" {
			common.SysLog(fmt.Sprintf("user %d has no webhook url, skip sending webhook", userId))
			return nil
		}

		// 获取 webhook secret
		webhookSecret := userSetting.WebhookSecret
		return SendWebhookNotify(webhookURLStr, webhookSecret, data)
	case dto.NotifyTypeBark:
		barkURL := userSetting.BarkUrl
		if barkURL == "" {
			common.SysLog(fmt.Sprintf("user %d has no bark url, skip sending bark", userId))
			return nil
		}
		return sendBarkNotify(barkURL, data)
	case dto.NotifyTypeGotify:
		gotifyUrl := userSetting.GotifyUrl
		gotifyToken := userSetting.GotifyToken
		if gotifyUrl == "" || gotifyToken == "" {
			common.SysLog(fmt.Sprintf("user %d has no gotify url or token, skip sending gotify", userId))
			return nil
		}
		return sendGotifyNotify(gotifyUrl, gotifyToken, userSetting.GotifyPriority, data)
	}
	return nil
}

// sendEmailNotify 通过邮件发送通知。
// 处理通知内容中的占位符替换后，调用 common.SendEmail 发送邮件。
// 参数:
//   - userEmail: 收件人邮箱地址
//   - data: 通知数据
//
// 返回值:
//   - error: 发送失败时返回错误
func sendEmailNotify(userEmail string, data dto.Notify) error {
	// make email content
	content := data.Content
	// 处理占位符
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}
	return common.SendEmail(data.Title, userEmail, content)
}

// sendBarkNotify 通过 Bark（iOS 推送服务）发送通知。
// 支持 URL 模板变量（{{title}}、{{content}}）替换。
// 支持 Worker 代理模式和直连模式，直连模式下包含 SSRF 防护。
// 参数:
//   - barkURL: Bark 推送 URL（可包含模板变量）
//   - data: 通知数据
//
// 返回值:
//   - error: 发送失败时返回错误
func sendBarkNotify(barkURL string, data dto.Notify) error {
	// 处理占位符
	content := data.Content
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}

	// 替换模板变量
	finalURL := strings.ReplaceAll(barkURL, "{{title}}", url.QueryEscape(data.Title))
	finalURL = strings.ReplaceAll(finalURL, "{{content}}", url.QueryEscape(content))

	// 发送GET请求到Bark
	var req *http.Request
	var resp *http.Response
	var err error

	if system_setting.EnableWorker() {
		// 使用worker发送请求
		workerReq := &WorkerRequest{
			URL:    finalURL,
			Key:    system_setting.WorkerValidKey,
			Method: http.MethodGet,
			Headers: map[string]string{
				"User-Agent": "OneAPI-Bark-Notify/1.0",
			},
		}

		resp, err = DoWorkerRequest(workerReq)
		if err != nil {
			return fmt.Errorf("failed to send bark request through worker: %v", err)
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("bark request failed with status code: %d", resp.StatusCode)
		}
	} else {
		// SSRF防护：验证Bark URL（非Worker模式）
		fetchSetting := system_setting.GetFetchSetting()
		if err := common.ValidateURLWithFetchSetting(finalURL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
			return fmt.Errorf("request reject: %v", err)
		}

		// 直接发送请求
		req, err = http.NewRequest(http.MethodGet, finalURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create bark request: %v", err)
		}

		// 设置User-Agent
		req.Header.Set("User-Agent", "OneAPI-Bark-Notify/1.0")

		// 发送请求。Bark URL 由用户配置，必须使用受保护 client，在 Dial 阶段
		// 再次校验 DNS 解析结果，避免 URL 预校验后的 DNS rebinding。
		client := GetSSRFProtectedHTTPClient()
		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send bark request: %v", err)
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("bark request failed with status code: %d", resp.StatusCode)
		}
	}

	return nil
}

// sendGotifyNotify 通过 Gotify（自托管推送服务）发送通知。
// 构建 JSON 格式的 Gotify 消息，支持优先级设置（0-10，超出范围使用默认值 5）。
// 支持 Worker 代理模式和直连模式，直连模式下包含 SSRF 防护。
// 参数:
//   - gotifyUrl: Gotify 服务器 URL
//   - gotifyToken: Gotify 应用 Token
//   - priority: 消息优先级（0-10）
//   - data: 通知数据
//
// 返回值:
//   - error: 发送失败时返回错误
func sendGotifyNotify(gotifyUrl string, gotifyToken string, priority int, data dto.Notify) error {
	// 处理占位符
	content := data.Content
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}

	// 构建完整的 Gotify API URL
	// 确保 URL 以 /message 结尾
	finalURL := strings.TrimSuffix(gotifyUrl, "/") + "/message?token=" + url.QueryEscape(gotifyToken)

	// Gotify优先级范围0-10，如果超出范围则使用默认值5
	if priority < 0 || priority > 10 {
		priority = 5
	}

	// 构建 JSON payload
	type GotifyMessage struct {
		Title    string `json:"title"`
		Message  string `json:"message"`
		Priority int    `json:"priority"`
	}

	payload := GotifyMessage{
		Title:    data.Title,
		Message:  content,
		Priority: priority,
	}

	// 序列化为 JSON
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal gotify payload: %v", err)
	}

	var req *http.Request
	var resp *http.Response

	if system_setting.EnableWorker() {
		// 使用worker发送请求
		workerReq := &WorkerRequest{
			URL:    finalURL,
			Key:    system_setting.WorkerValidKey,
			Method: http.MethodPost,
			Headers: map[string]string{
				"Content-Type": "application/json; charset=utf-8",
				"User-Agent":   "OneAPI-Gotify-Notify/1.0",
			},
			Body: payloadBytes,
		}

		resp, err = DoWorkerRequest(workerReq)
		if err != nil {
			return fmt.Errorf("failed to send gotify request through worker: %v", err)
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("gotify request failed with status code: %d", resp.StatusCode)
		}
	} else {
		// SSRF防护：验证Gotify URL（非Worker模式）
		fetchSetting := system_setting.GetFetchSetting()
		if err := common.ValidateURLWithFetchSetting(finalURL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
			return fmt.Errorf("request reject: %v", err)
		}

		// 直接发送请求
		req, err = http.NewRequest(http.MethodPost, finalURL, bytes.NewBuffer(payloadBytes))
		if err != nil {
			return fmt.Errorf("failed to create gotify request: %v", err)
		}

		// 设置请求头
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", "NexusTok-Gotify-Notify/1.0")

		// 发送请求。Gotify URL 由用户配置，必须使用受保护 client，在 Dial 阶段
		// 再次校验 DNS 解析结果，避免 URL 预校验后的 DNS rebinding。
		client := GetSSRFProtectedHTTPClient()
		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send gotify request: %v", err)
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("gotify request failed with status code: %d", resp.StatusCode)
		}
	}

	return nil
}
