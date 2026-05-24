// Package config - home.go
// 定义 Home 控制平面的运行时配置，通过 -home-jwt 参数注入。
package config

// HomeConfig 存储 Home 控制平面的运行时配置，通过 -home-jwt 参数注入。
// 用于管理 Home 集群发现和 Redis 连接等设置。
//
// HomeConfig stores runtime-only Home control plane settings from -home-jwt.
type HomeConfig struct {
	// Enabled 表示 Home 控制平面是否启用。
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Host 是 Home 服务的主机地址。
	Host string `yaml:"host" json:"-"`
	// Port 是 Home 服务的端口号。
	Port int `yaml:"port" json:"-"`
	// DisableClusterDiscovery 表示是否禁用集群发现功能。
	DisableClusterDiscovery bool `yaml:"disable-cluster-discovery" json:"-"`
	// TLS 是 Home Redis 连接的 TLS 配置。
	TLS HomeTLSConfig `yaml:"tls" json:"-"`
}

// HomeTLSConfig 配置 Home Redis 连接的客户端 TLS 设置。
//
// HomeTLSConfig configures client-side TLS for the home Redis connection.
type HomeTLSConfig struct {
	// Enable 表示是否启用 TLS。
	Enable bool `yaml:"enable" json:"-"`
	// ServerName 是 TLS 服务器名称。
	ServerName string `yaml:"server-name" json:"-"`
	// InsecureSkipVerify 表示是否跳过证书验证。
	InsecureSkipVerify bool `yaml:"insecure-skip-verify" json:"-"`
	// CACert 是 CA 证书路径。
	CACert string `yaml:"ca-cert" json:"-"`
	// ClientCert 是客户端证书路径。
	ClientCert string `yaml:"-" json:"-"`
	// ClientKey 是客户端私钥路径。
	ClientKey string `yaml:"-" json:"-"`
	// UseTargetServerName 表示是否使用目标服务器名称。
	UseTargetServerName bool `yaml:"-" json:"-"`
}
