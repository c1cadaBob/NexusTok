// Package service - system_instance.go
// 该文件实现系统实例心跳上报服务。心跳数据用于 Root 在系统信息页查看
// 当前节点、主从角色、版本和资源快照，并为后续 SystemTask 多节点调度提供基础观测。
package service

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/system_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

const systemInstanceReportInterval = 30 * time.Second

var systemInstanceReporterOnce sync.Once

// SystemInstanceInfo 是保存到 SystemInstance.Info 的 JSON 结构。
//
// SchemaVersion 用于后续兼容字段扩展；节点身份优先使用 NODE_NAME，缺失时回退
// 到主机名，并通过 should_configure_manually 提醒多副本部署应显式设置稳定名称。
type SystemInstanceInfo struct {
	SchemaVersion int                        `json:"schema_version"`
	Node          common.NodeIdentity        `json:"node"`
	Role          SystemInstanceRoleInfo     `json:"role"`
	Runtime       SystemInstanceRuntimeInfo  `json:"runtime"`
	Host          SystemInstanceHostInfo     `json:"host"`
	Resources     SystemInstanceResourceInfo `json:"resources"`
	Extra         map[string]any             `json:"extra,omitempty"`
}

type SystemInstanceRoleInfo struct {
	IsMaster bool `json:"is_master"`
}

type SystemInstanceRuntimeInfo struct {
	Version   string `json:"version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	StartedAt int64  `json:"started_at"`
}

type SystemInstanceHostInfo struct {
	Hostname        string   `json:"hostname"`
	IPAddress       string   `json:"ip_address,omitempty"`
	IPAddresses     []string `json:"ip_addresses,omitempty"`
	PlatformAddress string   `json:"platform_address,omitempty"`
}

type SystemInstanceResourceInfo struct {
	CPU     SystemInstanceResourceUsage  `json:"cpu"`
	Memory  SystemInstanceResourceUsage  `json:"memory"`
	Storage SystemInstanceStorageMetrics `json:"storage"`
}

type SystemInstanceResourceUsage struct {
	UsagePercent float64 `json:"usage_percent"`
}

type SystemInstanceStorageMetrics struct {
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

// StartSystemInstanceReporter 启动系统实例定时心跳上报。
func StartSystemInstanceReporter() {
	systemInstanceReporterOnce.Do(func() {
		gopool.Go(func() {
			reportSystemInstanceWithLog()

			ticker := time.NewTicker(systemInstanceReportInterval)
			defer ticker.Stop()
			for range ticker.C {
				reportSystemInstanceWithLog()
			}
		})
	})
}

// ReportCurrentSystemInstance 立即上报当前节点状态。
func ReportCurrentSystemInstance() error {
	node, err := ResolveSystemInstanceNode()
	if err != nil {
		return err
	}
	status := common.GetSystemStatus()
	diskInfo := common.GetDiskSpaceInfo()
	ipAddresses := hostIPAddresses()
	info := SystemInstanceInfo{
		SchemaVersion: 1,
		Node:          node,
		Role: SystemInstanceRoleInfo{
			IsMaster: common.IsMasterNode,
		},
		Runtime: SystemInstanceRuntimeInfo{
			Version:   common.Version,
			GOOS:      runtime.GOOS,
			GOARCH:    runtime.GOARCH,
			StartedAt: common.StartTime,
		},
		Host: SystemInstanceHostInfo{
			Hostname:        hostNameOrEmpty(),
			IPAddress:       firstHostIPAddress(ipAddresses),
			IPAddresses:     ipAddresses,
			PlatformAddress: platformAddress(ipAddresses),
		},
		Resources: SystemInstanceResourceInfo{
			CPU: SystemInstanceResourceUsage{
				UsagePercent: status.CPUUsage,
			},
			Memory: SystemInstanceResourceUsage{
				UsagePercent: status.MemoryUsage,
			},
			Storage: SystemInstanceStorageMetrics{
				TotalBytes:  diskInfo.Total,
				UsedBytes:   diskInfo.Used,
				FreeBytes:   diskInfo.Free,
				UsedPercent: diskInfo.UsedPercent,
			},
		},
	}
	return model.UpsertSystemInstance(node.Name, info, common.StartTime, common.GetTimestamp())
}

// ResolveSystemInstanceNode 解析当前节点身份。
//
// 多节点部署必须使用稳定的 NODE_NAME；未配置时使用主机名兜底，使单节点部署
// 开箱可观测，同时在返回结构中标记 should_configure_manually 便于页面提醒。
func ResolveSystemInstanceNode() (common.NodeIdentity, error) {
	identity := common.GetNodeIdentity()
	if strings.TrimSpace(identity.Name) != "" {
		identity.Name = strings.TrimSpace(identity.Name)
		if identity.Source == "" {
			identity.Source = common.NodeNameSourceHostname
		}
		identity.ShouldConfigureManually = !identity.ManuallyConfigured
		return identity, nil
	}

	hostname, err := os.Hostname()
	hostname = strings.TrimSpace(hostname)
	if err != nil || hostname == "" {
		return common.NodeIdentity{}, fmt.Errorf("system instance node name is empty")
	}
	return common.NodeIdentity{
		Name:                    hostname,
		Source:                  common.NodeNameSourceHostname,
		ManuallyConfigured:      false,
		ShouldConfigureManually: true,
	}, nil
}

func reportSystemInstanceWithLog() {
	if err := ReportCurrentSystemInstance(); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("system instance report failed: %v", err))
	}
}

func hostNameOrEmpty() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(hostname)
}

// hostIPAddresses 收集当前主机可用于节点识别的地址。
//
// loopback、未指定地址、组播地址和 link-local 地址不能作为平台访问地址；
// IPv4 优先于 IPv6，确保常见内网部署在页面上显示更容易复制使用的地址。
func hostIPAddresses() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	addresses := make([]string, 0)
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := networkInterface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addrs {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			default:
				host, _, splitErr := net.SplitHostPort(address.String())
				if splitErr == nil {
					ip = net.ParseIP(host)
				} else {
					ip = net.ParseIP(strings.Split(address.String(), "/")[0])
				}
			}
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() ||
				ip.IsLinkLocalUnicast() || ip.IsMulticast() {
				continue
			}
			value := ip.String()
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			addresses = append(addresses, value)
		}
	}
	sort.SliceStable(addresses, func(i, j int) bool {
		i4 := net.ParseIP(addresses[i]).To4() != nil
		j4 := net.ParseIP(addresses[j]).To4() != nil
		return i4 && !j4
	})
	return addresses
}

func firstHostIPAddress(addresses []string) string {
	if len(addresses) == 0 {
		return ""
	}
	return addresses[0]
}

// platformAddress 生成实例所在平台的可访问地址。
//
// ServerAddress 是管理员明确配置的访问入口时优先使用它；没有配置时才根据
// 主机 IP 和监听端口推导，避免把多节点页面误导到仅本机可访问的地址。
func platformAddress(addresses []string) string {
	configured := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	if configured != "" && !isLocalhostPlatformAddress(configured) {
		return configured
	}
	ip := firstHostIPAddress(addresses)
	if ip == "" {
		return ""
	}
	port := 3030
	if common.Port != nil && *common.Port > 0 {
		port = *common.Port
	}
	return "http://" + net.JoinHostPort(ip, strconv.Itoa(port))
}

func isLocalhostPlatformAddress(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
