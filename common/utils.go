// Package common - utils.go
// 该文件提供了通用的工具函数集合
//
// 功能分类：
// - 浏览器操作：打开系统默认浏览器
// - 网络工具：获取本机 IP 地址
// - 容器检测：判断是否运行在 Docker/Kubernetes 容器中
// - 格式转换：字节大小、时间秒数的人类可读格式
// - 类型转换：interface{} 到 string 的安全转换
// - UUID 生成：生成无连字符的 UUID 字符串
// - 随机数生成：密码学安全的随机字符串和密钥
// - 泛型工具：获取指针、类型转换
// - 文件操作：保存临时文件
// - URL 拼接：安全的 URL 路径拼接
package common

import (
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/big"
	"math/rand"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// OpenBrowser 使用系统默认浏览器打开指定 URL
//
// 跨平台支持：
// - Linux: xdg-open
// - Windows: rundll32 url.dll,FileProtocolHandler
// - macOS: open
//
// 参数：
//   - url: 要打开的 URL
func OpenBrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	}
	if err != nil {
		log.Println(err)
	}
}

// GetIp 获取本机的内网 IPv4 地址
//
// 遍历所有网络接口，返回第一个非回环的 IPv4 地址
// 优先返回常见的内网地址段：10.x.x.x, 172.x.x.x, 192.168.x.x
//
// 返回值：
//   - string: IPv4 地址字符串（未找到返回空字符串）
func GetIp() (ip string) {
	ips, err := net.InterfaceAddrs()
	if err != nil {
		log.Println(err)
		return ip
	}

	for _, a := range ips {
		if ipNet, ok := a.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ip = ipNet.IP.String()
				// 优先返回常见内网地址
				if strings.HasPrefix(ip, "10") {
					return
				}
				if strings.HasPrefix(ip, "172") {
					return
				}
				if strings.HasPrefix(ip, "192.168") {
					return
				}
				ip = ""
			}
		}
	}
	return
}

// GetNetworkIps 获取本机所有内网 IPv4 地址列表
//
// 遍历所有网络接口，返回所有非回环的内网 IPv4 地址
// 包含的内网地址段：10.x.x.x, 172.x.x.x, 192.168.x.x
//
// 返回值：
//   - []string: IPv4 地址列表
func GetNetworkIps() []string {
	var networkIps []string
	ips, err := net.InterfaceAddrs()
	if err != nil {
		log.Println(err)
		return networkIps
	}

	for _, a := range ips {
		if ipNet, ok := a.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ip := ipNet.IP.String()
				// 包含常见私网地址段
				if strings.HasPrefix(ip, "10.") ||
					strings.HasPrefix(ip, "172.") ||
					strings.HasPrefix(ip, "192.168.") {
					networkIps = append(networkIps, ip)
				}
			}
		}
	}
	return networkIps
}

// IsRunningInContainer 检测应用是否运行在容器中
//
// 检测方法（按优先级）：
// 1. 检查 /.dockerenv 文件是否存在（Docker 容器特有）
// 2. 检查 /proc/1/cgroup 是否包含容器标识（docker, containerd, kubepods, lxc）
// 3. 检查容器运行时环境变量（KUBERNETES_SERVICE_HOST, DOCKER_CONTAINER, container）
// 4. 检查 PID 1 的进程名是否为容器运行时（docker, containerd, runc）
//
// 返回值：
//   - bool: 是否运行在容器中
func IsRunningInContainer() bool {
	// 方法 1：检查 .dockerenv 文件（Docker 容器特有）
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// 方法 2：检查 cgroup 中的容器标识
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") ||
			strings.Contains(content, "containerd") ||
			strings.Contains(content, "kubepods") ||
			strings.Contains(content, "/lxc/") {
			return true
		}
	}

	// 方法 3：检查容器运行时环境变量
	containerEnvVars := []string{
		"KUBERNETES_SERVICE_HOST",
		"DOCKER_CONTAINER",
		"container",
	}

	for _, envVar := range containerEnvVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}

	// 方法 4：检查 PID 1 的进程名
	if data, err := os.ReadFile("/proc/1/comm"); err == nil {
		comm := strings.TrimSpace(string(data))
		// 容器中 PID 1 通常不是传统的 init 或 systemd
		if comm != "init" && comm != "systemd" {
			// 检查是否为常见的容器入口进程
			if strings.Contains(comm, "docker") ||
				strings.Contains(comm, "containerd") ||
				strings.Contains(comm, "runc") {
				return true
			}
		}
	}

	return false
}

// 字节大小常量
var sizeKB = 1024
var sizeMB = sizeKB * 1024
var sizeGB = sizeMB * 1024

// Bytes2Size 将字节数转换为人类可读的大小字符串
//
// 转换规则：
// - >= 1 GB：显示为 "x.xx GB"
// - >= 1 MB：显示为 "xxx MB"
// - >= 1 KB：显示为 "xxx KB"
// - < 1 KB：显示为 "xxx B"
//
// 参数：
//   - num: 字节数
//
// 返回值：
//   - string: 格式化后的大小字符串
func Bytes2Size(num int64) string {
	numStr := ""
	unit := "B"
	if num/int64(sizeGB) > 1 {
		numStr = fmt.Sprintf("%.2f", float64(num)/float64(sizeGB))
		unit = "GB"
	} else if num/int64(sizeMB) > 1 {
		numStr = fmt.Sprintf("%d", int(float64(num)/float64(sizeMB)))
		unit = "MB"
	} else if num/int64(sizeKB) > 1 {
		numStr = fmt.Sprintf("%d", int(float64(num)/float64(sizeKB)))
		unit = "KB"
	} else {
		numStr = fmt.Sprintf("%d", num)
	}
	return numStr + " " + unit
}

// Seconds2Time 将秒数转换为人类可读的时间字符串
//
// 转换结果包含年、月、天、小时、分钟、秒
// 例如：3661 秒 → "1 小时 1 分钟 1 秒"
//
// 参数：
//   - num: 秒数
//
// 返回值：
//   - time: 格式化后的时间字符串
func Seconds2Time(num int) (time string) {
	if num/31104000 > 0 {
		time += strconv.Itoa(num/31104000) + " 年 "
		num %= 31104000
	}
	if num/2592000 > 0 {
		time += strconv.Itoa(num/2592000) + " 个月 "
		num %= 2592000
	}
	if num/86400 > 0 {
		time += strconv.Itoa(num/86400) + " 天 "
		num %= 86400
	}
	if num/3600 > 0 {
		time += strconv.Itoa(num/3600) + " 小时 "
		num %= 3600
	}
	if num/60 > 0 {
		time += strconv.Itoa(num/60) + " 分钟 "
		num %= 60
	}
	time += strconv.Itoa(num) + " 秒"
	return
}

// Interface2String 将 interface{} 安全转换为字符串
//
// 支持的类型：
// - string：直接返回
// - int：格式化为十进制数字
// - float64：格式为浮点数（保留必要小数位）
// - bool：返回 "true" 或 "false"
// - nil：返回空字符串
// - 其他：使用 fmt.Sprintf("%v") 格式化
//
// 参数：
//   - inter: 任意类型的值
//
// 返回值：
//   - string: 字符串表示
func Interface2String(inter interface{}) string {
	switch inter.(type) {
	case string:
		return inter.(string)
	case int:
		return fmt.Sprintf("%d", inter.(int))
	case float64:
		return strconv.FormatFloat(inter.(float64), 'f', -1, 64)
	case bool:
		if inter.(bool) {
			return "true"
		} else {
			return "false"
		}
	case nil:
		return ""
	}
	return fmt.Sprintf("%v", inter)
}

// UnescapeHTML 将字符串标记为安全的 HTML 内容
//
// 返回 template.HTML 类型，Gin 模板引擎不会对其进行 HTML 转义
// 注意：仅对可信内容使用，避免 XSS 攻击
//
// 参数：
//   - x: HTML 字符串
//
// 返回值：
//   - interface{}: template.HTML 类型的值
func UnescapeHTML(x string) interface{} {
	return template.HTML(x)
}

// IntMax 返回两个整数中较大的一个
//
// 参数：
//   - a: 第一个整数
//   - b: 第二个整数
//
// 返回值：
//   - int: 较大的值
func IntMax(a int, b int) int {
	if a >= b {
		return a
	} else {
		return b
	}
}

// GetUUID 生成无连字符的 UUID 字符串
//
// 生成 UUID v4 并移除所有连字符
// 例如：550e8400e29b41d4a716446655440000
//
// 返回值：
//   - string: 32 位十六进制 UUID 字符串
func GetUUID() string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	return code
}

// keyChars 用于生成随机密钥的字符集
const keyChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// GenerateRandomCharsKey 生成指定长度的随机字符密钥
//
// 使用 crypto/rand 生成密码学安全的随机数
// 字符集：0-9, a-z, A-Z（共 62 个字符）
//
// 参数：
//   - length: 密钥长度
//
// 返回值：
//   - string: 随机密钥
//   - error: 随机数生成错误
func GenerateRandomCharsKey(length int) (string, error) {
	b := make([]byte, length)
	maxI := big.NewInt(int64(len(keyChars)))

	for i := range b {
		n, err := crand.Int(crand.Reader, maxI)
		if err != nil {
			return "", err
		}
		b[i] = keyChars[n.Int64()]
	}

	return string(b), nil
}

// GenerateRandomKey 生成指定长度的 Base64 编码随机密钥
//
// 使用 crypto/rand 生成随机字节，然后 Base64 编码
// 输出长度约为 length * 4/3 字符
//
// 参数：
//   - length: 期望的密钥长度（实际输出可能略长）
//
// 返回值：
//   - string: Base64 编码的随机密钥
//   - error: 随机数生成错误
func GenerateRandomKey(length int) (string, error) {
	bytes := make([]byte, length*3/4) // 对于48位的输出，这里应该是36
	if _, err := crand.Read(bytes); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// GenerateKey 生成 48 字符的随机密钥
//
// 使用 GenerateRandomCharsKey 生成，字符集为字母+数字
//
// 返回值：
//   - string: 48 位随机密钥
//   - error: 生成错误
func GenerateKey() (string, error) {
	return GenerateRandomCharsKey(48)
}

// GetRandomInt 返回 [0, max) 范围内的随机整数
//
// 使用 math/rand 生成，适用于非安全场景
//
// 参数：
//   - max: 上限（不包含）
//
// 返回值：
//   - int: 随机整数
func GetRandomInt(max int) int {
	return rand.Intn(max)
}

// GetTimestamp 获取当前 Unix 时间戳（秒）
//
// 返回值：
//   - int64: Unix 时间戳
func GetTimestamp() int64 {
	return time.Now().Unix()
}

// GetTimeString 获取当前 UTC 时间的精确时间字符串
//
// 格式：YYYYMMDDHHmmss + 9 位纳秒
// 例如：20240101120000123456789
//
// 返回值：
//   - string: 精确时间字符串
func GetTimeString() string {
	now := time.Now().UTC()
	return fmt.Sprintf("%s%d", now.Format("20060102150405"), now.UnixNano()%1e9)
}

// Max 返回两个整数中较大的一个
//
// 参数：
//   - a: 第一个整数
//   - b: 第二个整数
//
// 返回值：
//   - int: 较大的值
func Max(a int, b int) int {
	if a >= b {
		return a
	} else {
		return b
	}
}

// MessageWithRequestId 在消息末尾附加请求 ID
//
// 用于日志和错误消息中标识请求，便于问题追踪
//
// 参数：
//   - message: 原始消息
//   - id: 请求 ID
//
// 返回值：
//   - string: 附加了请求 ID 的消息
func MessageWithRequestId(message string, id string) string {
	return fmt.Sprintf("%s (request id: %s)", message, id)
}

// RandomSleep 随机休眠 0-3000 毫秒
//
// 用于：
// - 响应枚举攻击时增加时间不确定性
// - 限流退避时添加随机延迟
func RandomSleep() {
	// 休眠 0-3000 毫秒
	time.Sleep(time.Duration(rand.Intn(3000)) * time.Millisecond)
}

// GetPointer 获取值的指针
//
// 泛型函数，用于将值转换为指针类型
// 常用于构造包含可选字段的结构体
//
// 参数：
//   - v: 任意类型的值
//
// 返回值：
//   - *T: 值的指针
func GetPointer[T any](v T) *T {
	return &v
}

// Any2Type 将任意类型转换为指定的目标类型
//
// 通过 JSON 序列化/反序列化实现类型转换
// 适用于结构体之间的字段映射
//
// 参数：
//   - data: 源数据
//
// 返回值：
//   - T: 目标类型的值
//   - error: 转换错误
func Any2Type[T any](data any) (T, error) {
	var zero T
	bytes, err := json.Marshal(data)
	if err != nil {
		return zero, err
	}
	var res T
	err = json.Unmarshal(bytes, &res)
	if err != nil {
		return zero, err
	}
	return res, nil
}

// SaveTmpFile 将数据保存到临时文件
//
// 文件名会自动添加随机后缀，避免冲突
// 文件保存在系统临时目录中
//
// 参数：
//   - filename: 文件名前缀
//   - data: 数据流
//
// 返回值：
//   - string: 临时文件的完整路径
//   - error: 文件操作错误
func SaveTmpFile(filename string, data io.Reader) (string, error) {
	f, err := os.CreateTemp(os.TempDir(), filename)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create temporary file %s", filename)
	}
	defer f.Close()

	_, err = io.Copy(f, data)
	if err != nil {
		return "", errors.Wrapf(err, "failed to copy data to temporary file %s", filename)
	}

	return f.Name(), nil
}

// BuildURL 拼接基础 URL 和端点路径
//
// 安全地解析基础 URL 和端点，使用 url.URL.ResolveReference 进行拼接
// 处理各种边界情况（空端点、相对路径等）
//
// 参数：
//   - base: 基础 URL
//   - endpoint: 端点路径
//
// 返回值：
//   - string: 拼接后的完整 URL
func BuildURL(base string, endpoint string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base + endpoint
	}
	end := endpoint
	if end == "" {
		end = "/"
	}
	ref, err := url.Parse(end)
	if err != nil {
		return base + endpoint
	}
	return u.ResolveReference(ref).String()
}
