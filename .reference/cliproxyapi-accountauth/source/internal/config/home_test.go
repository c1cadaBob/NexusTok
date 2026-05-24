// config - home_test.go
// Home 配置解析测试
// 验证 ParseConfigBytes 函数在解析客户端配置时，
// 会忽略 Home 相关的配置节（Home 配置仅在服务端生效）。
package config

import "testing"

// TestParseConfigBytesIgnoresHomeConfig 验证即使 YAML 中包含完整的 home 配置节
// （包括 enabled、host、port、TLS 等字段），ParseConfigBytes 也会将其忽略，
// 返回的 Config 对象中 Home 字段保持零值。这是安全设计，防止客户端配置
// 意外覆盖服务端的 Home 集群配置。
func TestParseConfigBytesIgnoresHomeConfig(t *testing.T) {
	cfg, err := ParseConfigBytes([]byte(`
home:
  enabled: true
  host: home.example.com
  port: 444
  disable-cluster-discovery: true
  tls:
    enable: true
    server-name: home.example.com
    ca-cert: C:/certs/ca.pem
    insecure-skip-verify: true
`))
	if err != nil {
		t.Fatalf("ParseConfigBytes() error = %v", err)
	}

	if cfg.Home.Enabled {
		t.Fatal("Home.Enabled = true, want false")
	}
	if cfg.Home.Host != "" {
		t.Fatalf("Home.Host = %q, want empty", cfg.Home.Host)
	}
	if cfg.Home.Port != 0 {
		t.Fatalf("Home.Port = %d, want 0", cfg.Home.Port)
	}
	if cfg.Home.DisableClusterDiscovery {
		t.Fatal("Home.DisableClusterDiscovery = true, want false")
	}
	if cfg.Home.TLS.Enable {
		t.Fatal("Home.TLS.Enable = true, want false")
	}
	if cfg.Home.TLS.ServerName != "" {
		t.Fatalf("Home.TLS.ServerName = %q, want empty", cfg.Home.TLS.ServerName)
	}
	if cfg.Home.TLS.CACert != "" {
		t.Fatalf("Home.TLS.CACert = %q, want empty", cfg.Home.TLS.CACert)
	}
	if cfg.Home.TLS.InsecureSkipVerify {
		t.Fatal("Home.TLS.InsecureSkipVerify = true, want false")
	}
}
