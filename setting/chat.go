// Package setting - chat.go
// 该文件管理第三方聊天客户端的配置列表
//
// 功能：
// - 维护支持的第三方 AI 聊客端列表（如 Cherry Studio、Lobe Chat 等）
// - 提供客户端配置的 JSON 序列化/反序列化
// - 客户端配置使用模板变量 {key} 和 {address} 用于动态替换
package setting

import (
	"encoding/json"

	"github.com/c1cada/NexusTok/common"
)

// Chats 第三方聊天客户端配置列表
// 每个元素是一个 map，键为客户端名称，值为客户端的配置 URL 或协议
// 配置中支持模板变量：
// - {key}: 用户的 API Key
// - {address}: 服务端地址
// - {cherryConfig}: Cherry Studio 专用配置
// - {aionuiConfig}: AionUI 专用配置
// - {deepchatConfig}: DeepChat 专用配置
var Chats = []map[string]string{
	//{
	//	"ChatGPT Next Web 官方示例": "https://app.nextchat.dev/#/?settings={\"key\":\"{key}\",\"url\":\"{address}\"}",
	//},
	{
		"Cherry Studio": "cherrystudio://providers/api-keys?v=1&data={cherryConfig}",
	},
	{
		"AionUI": "aionui://provider/add?v=1&data={aionuiConfig}",
	},
	{
		"流畅阅读": "fluentread",
	},
	{
		"CC Switch": "ccswitch",
	},
	{
		"DeepChat": "deepchat://provider/install?v=1&data={deepchatConfig}",
	},
	{
		"Lobe Chat 官方示例": "https://chat-preview.lobehub.com/?settings={\"keyVaults\":{\"openai\":{\"apiKey\":\"{key}\",\"baseURL\":\"{address}/v1\"}}}",
	},
	{
		"AI as Workspace": "https://aiaw.app/set-provider?provider={\"type\":\"openai\",\"settings\":{\"apiKey\":\"{key}\",\"baseURL\":\"{address}/v1\",\"compatibility\":\"strict\"}}",
	},
	{
		"AMA 问天": "ama://set-api-key?server={address}&key={key}",
	},
	{
		"OpenCat": "opencat://team/join?domain={address}&token={key}",
	},
}

// UpdateChatsByJsonString 从 JSON 字符串更新聊天客户端列表
//
// 参数：
//   - jsonString: JSON 格式的客户端配置字符串
//
// 返回值：
//   - error: 解析错误
func UpdateChatsByJsonString(jsonString string) error {
	Chats = make([]map[string]string, 0)
	return json.Unmarshal([]byte(jsonString), &Chats)
}

// Chats2JsonString 将聊天客户端列表序列化为 JSON 字符串
//
// 返回值：
//   - string: JSON 格式的客户端配置字符串
func Chats2JsonString() string {
	jsonBytes, err := json.Marshal(Chats)
	if err != nil {
		common.SysLog("error marshalling chats: " + err.Error())
		return "[]"
	}
	return string(jsonBytes)
}
