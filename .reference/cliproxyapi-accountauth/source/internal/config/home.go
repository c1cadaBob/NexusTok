// 包 config - home.go
// 该文件定义了 Home 控制平面的运行时配置结构体。
package config

// HomeConfig 存储从 -home-jwt 获取的仅运行时 Home 控制平面设置。
type HomeConfig struct {
	// Enabled 指示 Home 控制平面是否启用
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Host 是 Home 控制平面的主机地址
	Host string `yaml:"host" json:"-"`
	// Port 是 Home 控制平面的端口
	Port int `yaml:"port" json:"-"`
	// DisableClusterDiscovery 禁用集群发现功能
	DisableClusterDiscovery bool `yaml:"disable-cluster-discovery" json:"-"`
	// TLS 配置 Home 连接的客户端 TLS
	TLS HomeTLSConfig `yaml:"tls" json:"-"`
}

// HomeTLSConfig 为 Home Redis 连接配置客户端 TLS。
type HomeTLSConfig struct {
	// Enable 启用 TLS 连接
	Enable bool `yaml:"enable" json:"-"`
	// ServerName 是 TLS 服务器名称
	ServerName string `yaml:"server-name" json:"-"`
	// InsecureSkipVerify 跳过证书验证
	InsecureSkipVerify bool `yaml:"insecure-skip-verify" json:"-"`
	// CACert 是 CA 证书内容
	CACert string `yaml:"ca-cert" json:"-"`
	// ClientCert 是客户端证书内容
	ClientCert string `yaml:"-" json:"-"`
	// ClientKey 是客户端私钥内容
	ClientKey string `yaml:"-" json:"-"`
	// UseTargetServerName 使用目标服务器名称进行 TLS 验证
	UseTargetServerName bool `yaml:"-" json:"-"`
}
