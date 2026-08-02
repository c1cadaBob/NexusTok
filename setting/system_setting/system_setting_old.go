// system_setting_old.go — 旧版系统配置（服务器地址与 Worker 节点配置）
// 职责：管理服务器地址和 Worker 工作节点的相关配置。
// Worker 节点用于分担主服务器的请求处理负载。

package system_setting

// ServerAddress 当前服务器的访问地址
var ServerAddress = "http://localhost:3030"

// WorkerUrl Worker 工作节点的 URL 地址，为空表示不使用 Worker
var WorkerUrl = ""

// WorkerValidKey Worker 节点验证密钥，用于身份验证
var WorkerValidKey = ""

// WorkerAllowHttpImageRequestEnabled 控制 Worker 是否允许 HTTP（非 HTTPS）图片请求
var WorkerAllowHttpImageRequestEnabled = false

// EnableWorker 判断是否启用了 Worker 工作节点
// 返回值：WorkerUrl 不为空时返回 true
func EnableWorker() bool {
	return WorkerUrl != ""
}
