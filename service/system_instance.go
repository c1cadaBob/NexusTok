// Package service - system_instance.go
// 该文件实现系统实例心跳上报服务。心跳数据用于 Root 在系统信息页查看
// 当前节点、主从角色、版本和资源快照，并为后续 SystemTask 多节点调度提供基础观测。
package service

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"

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
	Node          SystemInstanceNodeInfo     `json:"node"`
	Role          SystemInstanceRoleInfo     `json:"role"`
	Runtime       SystemInstanceRuntimeInfo  `json:"runtime"`
	Host          SystemInstanceHostInfo     `json:"host"`
	Resources     SystemInstanceResourceInfo `json:"resources"`
	Extra         map[string]any             `json:"extra,omitempty"`
}

type SystemInstanceNodeInfo struct {
	Name                    string `json:"name"`
	Source                  string `json:"source"`
	ManuallyConfigured      bool   `json:"manually_configured"`
	ShouldConfigureManually bool   `json:"should_configure_manually"`
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
	Hostname string `json:"hostname"`
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
			Hostname: hostNameOrEmpty(),
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
func ResolveSystemInstanceNode() (SystemInstanceNodeInfo, error) {
	name := strings.TrimSpace(common.NodeName)
	if name != "" {
		return SystemInstanceNodeInfo{
			Name:               name,
			Source:             "env",
			ManuallyConfigured: true,
		}, nil
	}

	hostname, err := os.Hostname()
	hostname = strings.TrimSpace(hostname)
	if err != nil || hostname == "" {
		return SystemInstanceNodeInfo{}, fmt.Errorf("system instance node name is empty")
	}
	return SystemInstanceNodeInfo{
		Name:                    hostname,
		Source:                  "hostname",
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
