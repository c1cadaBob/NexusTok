/*
Copyright (C) 2023-2026 c1cada

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cada.dev
*/
package common

import (
	"os"
	"strings"
)

// NodeIdentity 描述当前进程用于日志、任务和系统实例心跳的节点身份。
// Name 是所有运行时链路共享的节点名；Source 和 ManuallyConfigured 用来告诉
// 管理界面该值是否来自稳定的 NODE_NAME，ShouldConfigureManually 则用于提醒
// 多实例部署不要长期依赖自动 hostname。
type NodeIdentity struct {
	Name                    string `json:"name"`
	Source                  string `json:"source"`
	ManuallyConfigured      bool   `json:"manually_configured"`
	ShouldConfigureManually bool   `json:"should_configure_manually"`
}

// initNodeNameIdentity 在进程启动时解析节点身份。
// 该函数只依赖环境变量和本机 hostname，必须保持轻量，避免 InitEnv 早期阶段
// 引入数据库、Redis 或 logger 等初始化环路。
func initNodeNameIdentity() {
	if envNodeName := strings.TrimSpace(os.Getenv("NODE_NAME")); envNodeName != "" {
		NodeName = envNodeName
		NodeNameSource = NodeNameSourceManual
		NodeNameManuallyConfigured = true
		return
	}

	hostname, _ := os.Hostname()
	NodeName = strings.TrimSpace(hostname)
	NodeNameSource = NodeNameSourceHostname
	NodeNameManuallyConfigured = false
}

// GetNodeIdentity 返回当前进程的节点身份快照。
// 调用方可以信任返回值里的 Name 已经去除首尾空白；如果极端环境无法取得
// hostname，Name 可能为空，上报前仍应按具体业务决定是否返回错误。
func GetNodeIdentity() NodeIdentity {
	name := strings.TrimSpace(NodeName)
	source := NodeNameSource
	if source == "" {
		source = NodeNameSourceHostname
	}
	manuallyConfigured := NodeNameManuallyConfigured && name != ""

	return NodeIdentity{
		Name:                    name,
		Source:                  source,
		ManuallyConfigured:      manuallyConfigured,
		ShouldConfigureManually: !manuallyConfigured,
	}
}
