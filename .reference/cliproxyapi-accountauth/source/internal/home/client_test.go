// home - client_test.go
// Home 客户端的单元测试文件，用于验证认证分发请求构造、Redis 连接选项
// （含 TLS 配置）、集群节点发现以及故障转移等核心功能的正确性。
package home

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

// TestAuthDispatchRequestIncludesCount 验证通过 newAuthDispatchRequest 构造的
// 认证分发请求在序列化为 JSON 后，"count" 字段的值与传入参数一致。
// 同时验证该结构体能正确序列化/反序列化。
func TestAuthDispatchRequestIncludesCount(t *testing.T) {
	req := newAuthDispatchRequest("gpt-5.4", "session-1", http.Header{"Authorization": {"Bearer test"}}, 2)

	raw, err := json.Marshal(&req)
	if err != nil {
		t.Fatalf("marshal auth dispatch request: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal auth dispatch request: %v", err)
	}
	if got := int(payload["count"].(float64)); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
}

// TestAuthDispatchRequestDefaultsCountToOne 验证当传入 count 参数为 0 时，
// newAuthDispatchRequest 会将 Count 默认设置为 1（确保至少请求 1 个凭证）。
func TestAuthDispatchRequestDefaultsCountToOne(t *testing.T) {
	req := newAuthDispatchRequest("gpt-5.4", "", nil, 0)

	if req.Count != 1 {
		t.Fatalf("count = %d, want 1", req.Count)
	}
}

// TestRedisOptionsHomeTLSDisabled 验证当 Home 配置未启用 TLS 时，
// redisOptionsLocked 返回的 Redis 选项中 TLSConfig 为 nil，Password 为空字符串。
func TestRedisOptionsHomeTLSDisabled(t *testing.T) {
	client := New(config.HomeConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    6379,
	})

	client.mu.Lock()
	options, err := client.redisOptionsLocked("127.0.0.1:6379")
	client.mu.Unlock()
	if err != nil {
		t.Fatalf("redisOptionsLocked() error = %v", err)
	}

	if options.TLSConfig != nil {
		t.Fatalf("TLSConfig = %#v, want nil", options.TLSConfig)
	}
	if options.Password != "" {
		t.Fatalf("Password = %q, want empty", options.Password)
	}
}

// TestRedisOptionsHomeTLSEnabledUsesSeedHostAsServerName 验证当 TLS 启用但未
// 显式指定 ServerName 时，Redis TLS 配置会使用初始种子主机名（home.example.com）
// 作为 ServerName，而非后续被替换的 IP 地址（127.0.0.1）。
// 同时验证 TLS 最低版本为 1.2。
func TestRedisOptionsHomeTLSEnabledUsesSeedHostAsServerName(t *testing.T) {
	client := New(config.HomeConfig{
		Enabled: true,
		Host:    "home.example.com",
		Port:    444,
		TLS: config.HomeTLSConfig{
			Enable: true,
		},
	})
	// 将 Host 修改为 IP 地址，模拟后续连接地址变化的场景
	client.homeCfg.Host = "127.0.0.1"

	client.mu.Lock()
	options, err := client.redisOptionsLocked("127.0.0.1:444")
	client.mu.Unlock()
	if err != nil {
		t.Fatalf("redisOptionsLocked() error = %v", err)
	}

	if options.TLSConfig == nil {
		t.Fatal("TLSConfig is nil")
	}
	if options.TLSConfig.ServerName != "home.example.com" {
		t.Fatalf("ServerName = %q, want home.example.com", options.TLSConfig.ServerName)
	}
	if options.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %d, want TLS 1.2", options.TLSConfig.MinVersion)
	}
}

// TestRedisOptionsHomeTLSEnabledUsesExplicitServerName 验证当 TLS 启用且显式
// 指定了 ServerName 时，Redis TLS 配置使用指定的 ServerName（而非种子主机名），
// 并且 InsecureSkipVerify 设置与配置一致。
func TestRedisOptionsHomeTLSEnabledUsesExplicitServerName(t *testing.T) {
	client := New(config.HomeConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    444,
		TLS: config.HomeTLSConfig{
			Enable:             true,
			ServerName:         "home.example.com",
			InsecureSkipVerify: true,
		},
	})

	client.mu.Lock()
	options, err := client.redisOptionsLocked("127.0.0.1:444")
	client.mu.Unlock()
	if err != nil {
		t.Fatalf("redisOptionsLocked() error = %v", err)
	}

	if options.TLSConfig == nil {
		t.Fatal("TLSConfig is nil")
	}
	if options.TLSConfig.ServerName != "home.example.com" {
		t.Fatalf("ServerName = %q, want home.example.com", options.TLSConfig.ServerName)
	}
	if !options.TLSConfig.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify = false, want true")
	}
}

// TestRefreshClusterNodesDisabledSkipsRedisCommand 验证当集群发现功能被禁用
// （DisableClusterDiscovery = true）时，refreshClusterNodes 函数不会执行
// 任何 Redis 命令，返回 switched = false，且 Redis 客户端不会被初始化。
func TestRefreshClusterNodesDisabledSkipsRedisCommand(t *testing.T) {
	client := New(config.HomeConfig{
		Enabled:                 true,
		Host:                    "127.0.0.1",
		Port:                    1,
		DisableClusterDiscovery: true,
	})

	switched, err := client.refreshClusterNodes(context.Background())
	if err != nil {
		t.Fatalf("refreshClusterNodes() error = %v", err)
	}
	if switched {
		t.Fatal("refreshClusterNodes() switched = true, want false")
	}
	if client.cmd != nil || client.sub != nil {
		t.Fatalf("redis clients were initialized when cluster discovery was disabled")
	}
}

// TestFailoverAfterReconnectFailureDisabledDoesNotSwitchToClusterNode 验证当集群
// 发现功能被禁用时，即使重连失败次数达到阈值，failoverAfterReconnectFailure 也不会
// 切换到其他集群节点，地址保持为初始种子地址。
func TestFailoverAfterReconnectFailureDisabledDoesNotSwitchToClusterNode(t *testing.T) {
	client := New(config.HomeConfig{
		Enabled:                 true,
		Host:                    "seed.example.com",
		Port:                    8327,
		DisableClusterDiscovery: true,
	})
	client.mu.Lock()
	// 设置集群节点列表，模拟存在可切换的备用节点
	client.clusterNodes = []clusterNode{{IP: "other.example.com", Port: 8327}}
	// 将重连失败次数设为阈值减 1（再失败一次就会触发故障转移逻辑）
	client.reconnectFailures = homeReconnectFailoverThreshold - 1
	client.mu.Unlock()

	switched, addr := client.failoverAfterReconnectFailure()
	if switched {
		t.Fatalf("failoverAfterReconnectFailure() switched to %s, want no switch", addr)
	}
	// 验证地址仍为种子地址，未发生故障转移
	if got, _ := client.addr(); got != "seed.example.com:8327" {
		t.Fatalf("addr() = %q, want seed.example.com:8327", got)
	}
}
