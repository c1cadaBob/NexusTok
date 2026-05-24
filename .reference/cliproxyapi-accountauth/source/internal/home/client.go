// 包 home - client.go
// 该文件实现了 Home 控制平面的 Redis 客户端。
// 提供与 Home 服务器的连接管理、配置获取、认证分发、使用量上报、
// 集群发现和故障转移等功能。
package home

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	log "github.com/sirupsen/logrus"
)

const (
	// redisKeyConfig 是 Redis 中配置的键名
	redisKeyConfig = "config"
	// redisChannelConfig 是配置更新的 Redis 订阅频道
	redisChannelConfig = "config"
	// redisKeyModels 是 Redis 中模型列表的键名
	redisKeyModels = "models"
	// redisKeyUsage 是 Redis 中使用量数据的键名
	redisKeyUsage = "usage"
	// redisKeyRequestLog 是 Redis 中请求日志的键名
	redisKeyRequestLog = "request-log"

	// homeReconnectInterval 是重连间隔
	homeReconnectInterval = time.Second
	// homeReconnectFailoverThreshold 是触发故障转移的重连失败阈值
	homeReconnectFailoverThreshold = 3
	// homeRedisOperationTimeout 是 Redis 操作超时时间
	homeRedisOperationTimeout = 3 * time.Second
	// homeSubscriptionReceiveTimeout 是订阅接收超时时间
	homeSubscriptionReceiveTimeout = 3 * time.Second
	// redisChannelCluster 是集群更新的 Redis 订阅频道
	redisChannelCluster = "cluster"
)

var (
	// ErrDisabled 表示 Home 客户端已禁用
	ErrDisabled = errors.New("home client disabled")
	// ErrNotConnected 表示 Home 未连接
	ErrNotConnected = errors.New("home not connected")
	// ErrEmptyResponse 表示 Home 返回了空响应
	ErrEmptyResponse = errors.New("home returned empty response")
	// ErrAuthNotFound 表示 Home 未找到认证
	ErrAuthNotFound = errors.New("home auth not found")
	// ErrConfigNotFound 表示 Home 未找到配置
	ErrConfigNotFound = errors.New("home config not found")
	// ErrModelsNotFound 表示 Home 未找到模型
	ErrModelsNotFound = errors.New("home models not found")
)

// clusterNode 表示集群中的一个节点。
type clusterNode struct {
	// IP 是节点的 IP 地址
	IP string `json:"ip"`
	// Port 是节点的端口
	Port int `json:"port"`
	// ClientCount 是节点的客户端数量
	ClientCount int `json:"client_count"`
	// IsMaster 指示节点是否为主节点
	IsMaster bool `json:"is_master"`
	// LastSeenAt 是节点最后可见时间
	LastSeenAt time.Time `json:"last_seen_at"`
}

// clusterNodesEnvelope 是集群节点响应的信封结构。
type clusterNodesEnvelope struct {
	// OK 指示请求是否成功
	OK bool `json:"ok"`
	// Nodes 是集群节点列表
	Nodes []clusterNode `json:"nodes"`
}

// Client 是 Home 控制平面的 Redis 客户端。
type Client struct {
	mu sync.Mutex

	homeCfg  config.HomeConfig
	seedHost string
	seedPort int

	cmd *redis.Client
	sub *redis.Client

	heartbeatOK       atomic.Bool
	clusterNodes      []clusterNode
	reconnectFailures int
}

// New 创建新的 Home 客户端实例。
func New(homeCfg config.HomeConfig) *Client {
	return &Client{
		homeCfg:  homeCfg,
		seedHost: strings.TrimSpace(homeCfg.Host),
		seedPort: homeCfg.Port,
	}
}

// Enabled 检查 Home 客户端是否启用。
func (c *Client) Enabled() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.homeCfg.Enabled
}

// HeartbeatOK 检查 Home 心跳是否正常。
func (c *Client) HeartbeatOK() bool {
	if c == nil {
		return false
	}
	if !c.Enabled() {
		return false
	}
	return c.heartbeatOK.Load()
}

// Close 关闭 Home 客户端连接。
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.heartbeatOK.Store(false)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeClientsLocked()
}

func (c *Client) closeClientsLocked() {
	if c.cmd != nil {
		_ = c.cmd.Close()
	}
	if c.sub != nil {
		_ = c.sub.Close()
	}
	c.cmd = nil
	c.sub = nil
}

func (c *Client) addr() (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.addrLocked()
}

func (c *Client) addrLocked() (string, bool) {
	host := strings.TrimSpace(c.homeCfg.Host)
	if host == "" {
		return "", false
	}
	if c.homeCfg.Port <= 0 {
		return "", false
	}
	return net.JoinHostPort(host, strconv.Itoa(c.homeCfg.Port)), true
}

func (c *Client) ensureClients() error {
	if c == nil {
		return ErrDisabled
	}
	if !c.Enabled() {
		return ErrDisabled
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	addr, ok := c.addrLocked()
	if !ok {
		return fmt.Errorf("home: invalid address (host=%q port=%d)", c.homeCfg.Host, c.homeCfg.Port)
	}

	if c.cmd == nil {
		options, errOptions := c.redisOptionsLocked(addr)
		if errOptions != nil {
			return errOptions
		}
		c.cmd = redis.NewClient(options)
	}
	if c.sub == nil {
		options, errOptions := c.redisOptionsLocked(addr)
		if errOptions != nil {
			return errOptions
		}
		c.sub = redis.NewClient(options)
	}
	return nil
}

func (c *Client) redisOptionsLocked(addr string) (*redis.Options, error) {
	tlsConfig, errTLS := c.homeTLSConfigLocked(addr)
	if errTLS != nil {
		return nil, errTLS
	}
	return &redis.Options{
		Addr:                  addr,
		TLSConfig:             tlsConfig,
		DialTimeout:           homeRedisOperationTimeout,
		ReadTimeout:           homeRedisOperationTimeout,
		WriteTimeout:          homeRedisOperationTimeout,
		MaxRetries:            -1,
		DialerRetries:         1,
		ContextTimeoutEnabled: true,
	}, nil
}

func (c *Client) homeTLSConfigLocked(addr string) (*tls.Config, error) {
	serverName := strings.TrimSpace(c.homeCfg.TLS.ServerName)
	if serverName == "" {
		if c.homeCfg.TLS.UseTargetServerName {
			serverName = hostFromAddress(addr)
		} else {
			serverName = strings.TrimSpace(c.seedHost)
		}
	}
	if serverName == "" {
		serverName = strings.TrimSpace(c.homeCfg.Host)
	}
	return newHomeTLSConfig(c.homeCfg.TLS, serverName)
}

func hostFromAddress(addr string) string {
	host, _, errSplit := net.SplitHostPort(strings.TrimSpace(addr))
	if errSplit == nil {
		return strings.TrimSpace(host)
	}
	return strings.TrimSpace(addr)
}

func newHomeTLSConfig(cfg config.HomeTLSConfig, fallbackServerName string) (*tls.Config, error) {
	if !cfg.Enable {
		return nil, nil
	}

	serverName := strings.TrimSpace(cfg.ServerName)
	if serverName == "" {
		serverName = strings.TrimSpace(fallbackServerName)
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         serverName,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}

	clientCertPath := strings.TrimSpace(cfg.ClientCert)
	clientKeyPath := strings.TrimSpace(cfg.ClientKey)
	if clientCertPath != "" || clientKeyPath != "" {
		if clientCertPath == "" || clientKeyPath == "" {
			return nil, fmt.Errorf("home tls: client certificate and key must be set together")
		}
		certPair, errLoad := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if errLoad != nil {
			return nil, fmt.Errorf("home tls: load client certificate: %w", errLoad)
		}
		tlsConfig.Certificates = []tls.Certificate{certPair}
	}

	caCertPath := strings.TrimSpace(cfg.CACert)
	if caCertPath == "" {
		return tlsConfig, nil
	}

	caCertPEM, errRead := os.ReadFile(caCertPath)
	if errRead != nil {
		return nil, fmt.Errorf("home tls: read ca-cert: %w", errRead)
	}

	certPool, errPool := x509.SystemCertPool()
	if errPool != nil || certPool == nil {
		certPool = x509.NewCertPool()
	}
	if !certPool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("home tls: ca-cert contains no PEM certificates")
	}
	tlsConfig.RootCAs = certPool

	return tlsConfig, nil
}

func (c *Client) commandClient() (*redis.Client, error) {
	if errEnsure := c.ensureClients(); errEnsure != nil {
		return nil, errEnsure
	}
	c.mu.Lock()
	cmd := c.cmd
	c.mu.Unlock()
	if cmd == nil {
		return nil, ErrNotConnected
	}
	return cmd, nil
}

func (c *Client) subscriptionClient() (*redis.Client, error) {
	if errEnsure := c.ensureClients(); errEnsure != nil {
		return nil, errEnsure
	}
	c.mu.Lock()
	sub := c.sub
	c.mu.Unlock()
	if sub == nil {
		return nil, ErrNotConnected
	}
	return sub, nil
}

// Ping 向 Home 服务器发送 Ping 请求。
func (c *Client) Ping(ctx context.Context) error {
	cmd, errClient := c.commandClient()
	if errClient != nil {
		return errClient
	}
	return cmd.Ping(ctx).Err()
}

func (c *Client) clusterDiscoveryEnabled() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.clusterDiscoveryEnabledLocked()
}

func (c *Client) clusterDiscoveryEnabledLocked() bool {
	return !c.homeCfg.DisableClusterDiscovery
}

func (c *Client) refreshBestClusterNode(ctx context.Context) {
	if !c.clusterDiscoveryEnabled() {
		return
	}
	switched, errRefresh := c.refreshClusterNodes(ctx)
	if errRefresh != nil {
		log.Debugf("home cluster nodes unavailable: %v", errRefresh)
		return
	}
	if switched {
		if addr, ok := c.addr(); ok {
			log.Infof("home cluster target switched to %s", addr)
		}
	}
}

func (c *Client) refreshClusterNodes(ctx context.Context) (bool, error) {
	if !c.clusterDiscoveryEnabled() {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cmd, errClient := c.commandClient()
	if errClient != nil {
		return false, errClient
	}
	raw, errDo := cmd.Do(ctx, "CLUSTER", "NODES").Text()
	if errDo != nil {
		return false, errDo
	}

	nodes, errParse := parseClusterNodesPayload([]byte(raw))
	if errParse != nil {
		return false, errParse
	}
	if len(nodes) == 0 {
		return false, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.clusterNodes = nodes
	c.reconnectFailures = 0
	return c.switchToNodeLocked(nodes[0]), nil
}

func parseClusterNodesPayload(raw []byte) ([]clusterNode, error) {
	var envelope clusterNodesEnvelope
	if errUnmarshal := json.Unmarshal(raw, &envelope); errUnmarshal != nil {
		return nil, errUnmarshal
	}
	return normalizeClusterNodes(envelope.Nodes), nil
}

func (c *Client) updateClusterNodesFromPayload(raw []byte) error {
	if c == nil || !c.clusterDiscoveryEnabled() {
		return nil
	}
	nodes, errParse := parseClusterNodesPayload(raw)
	if errParse != nil {
		return errParse
	}
	c.mu.Lock()
	c.clusterNodes = nodes
	c.mu.Unlock()
	return nil
}

func normalizeClusterNodes(nodes []clusterNode) []clusterNode {
	out := make([]clusterNode, 0, len(nodes))
	for _, node := range nodes {
		node.IP = strings.TrimSpace(node.IP)
		if node.IP == "" || node.Port <= 0 {
			continue
		}
		if node.ClientCount < 0 {
			node.ClientCount = 0
		}
		out = append(out, node)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ClientCount < out[j].ClientCount
	})
	return out
}

func (c *Client) switchToNodeLocked(node clusterNode) bool {
	host := strings.TrimSpace(node.IP)
	if host == "" || node.Port <= 0 {
		return false
	}
	if strings.TrimSpace(c.homeCfg.Host) == host && c.homeCfg.Port == node.Port {
		return false
	}
	c.homeCfg.Host = host
	c.homeCfg.Port = node.Port
	c.closeClientsLocked()
	return true
}

func (c *Client) markReconnectFailure(reason string) {
	switched, addr := c.failoverAfterReconnectFailure()
	if switched {
		log.Warnf("home control center unavailable after repeated %s failures; switching to %s", reason, addr)
	}
}

func (c *Client) failoverAfterReconnectFailure() (bool, string) {
	if c == nil {
		return false, ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.clusterDiscoveryEnabledLocked() {
		c.reconnectFailures = 0
		return false, ""
	}
	c.reconnectFailures++
	if c.reconnectFailures < homeReconnectFailoverThreshold {
		return false, ""
	}
	c.reconnectFailures = 0

	return c.switchToNextNodeLocked()
}

func (c *Client) failoverAfterSubscriptionTimeout() (bool, string) {
	if c == nil {
		return false, ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.clusterDiscoveryEnabledLocked() {
		c.reconnectFailures = 0
		return false, ""
	}
	c.reconnectFailures = 0
	return c.switchToNextNodeLocked()
}

func (c *Client) switchToNextNodeLocked() (bool, string) {
	currentHost := strings.TrimSpace(c.homeCfg.Host)
	currentPort := c.homeCfg.Port
	candidates := append([]clusterNode(nil), c.clusterNodes...)
	if strings.TrimSpace(c.seedHost) != "" && c.seedPort > 0 {
		candidates = append(candidates, clusterNode{IP: c.seedHost, Port: c.seedPort})
	}
	for _, node := range candidates {
		host := strings.TrimSpace(node.IP)
		if host == "" || node.Port <= 0 {
			continue
		}
		if host == currentHost && node.Port == currentPort {
			continue
		}
		if c.switchToNodeLocked(clusterNode{IP: host, Port: node.Port}) {
			addr, _ := c.addrLocked()
			return true, addr
		}
	}
	return false, ""
}

func (c *Client) markSubscriptionTimeout() {
	switched, addr := c.failoverAfterSubscriptionTimeout()
	if switched {
		log.Warnf("home subscription heartbeat timeout; switching to %s", addr)
	}
}

func (c *Client) resetReconnectFailures() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.reconnectFailures = 0
	c.mu.Unlock()
}

// GetConfig 从 Home 服务器获取配置。
func (c *Client) GetConfig(ctx context.Context) ([]byte, error) {
	c.refreshBestClusterNode(ctx)
	cmd, errClient := c.commandClient()
	if errClient != nil {
		return nil, errClient
	}
	raw, err := cmd.Get(ctx, redisKeyConfig).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrConfigNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, ErrEmptyResponse
	}
	return raw, nil
}

// GetModels 从 Home 服务器获取模型列表。
func (c *Client) GetModels(ctx context.Context) ([]byte, error) {
	cmd, errClient := c.commandClient()
	if errClient != nil {
		return nil, errClient
	}
	raw, err := cmd.Get(ctx, redisKeyModels).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrModelsNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, ErrEmptyResponse
	}
	return raw, nil
}

func headersToLowerMap(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		k := strings.ToLower(strings.TrimSpace(key))
		if k == "" {
			continue
		}
		if len(values) == 0 {
			out[k] = ""
			continue
		}
		trimmed := make([]string, 0, len(values))
		for _, v := range values {
			trimmed = append(trimmed, strings.TrimSpace(v))
		}
		out[k] = strings.Join(trimmed, ", ")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func newAuthDispatchRequest(requestedModel string, sessionID string, headers http.Header, count int) authDispatchRequest {
	if count <= 0 {
		count = 1
	}
	return authDispatchRequest{
		Type:      "auth",
		Model:     requestedModel,
		Count:     count,
		SessionID: strings.TrimSpace(sessionID),
		Headers:   headersToLowerMap(headers),
	}
}

// RPopAuth 从 Home 服务器弹出认证信息。
func (c *Client) RPopAuth(ctx context.Context, requestedModel string, sessionID string, headers http.Header, count int) ([]byte, error) {
	cmd, errClient := c.commandClient()
	if errClient != nil {
		return nil, errClient
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return nil, fmt.Errorf("home: requested model is empty")
	}
	req := newAuthDispatchRequest(requestedModel, sessionID, headers, count)
	keyBytes, err := json.Marshal(&req)
	if err != nil {
		return nil, err
	}

	raw, err := cmd.RPop(ctx, string(keyBytes)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrAuthNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, ErrEmptyResponse
	}
	return raw, nil
}

// GetRefreshAuth 从 Home 服务器获取刷新的认证信息。
func (c *Client) GetRefreshAuth(ctx context.Context, authIndex string) ([]byte, error) {
	cmd, errClient := c.commandClient()
	if errClient != nil {
		return nil, errClient
	}
	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" {
		return nil, fmt.Errorf("home: auth_index is empty")
	}
	req := refreshRequest{
		Type:      "refresh",
		AuthIndex: authIndex,
	}
	keyBytes, err := json.Marshal(&req)
	if err != nil {
		return nil, err
	}

	raw, err := cmd.Get(ctx, string(keyBytes)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrAuthNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, ErrEmptyResponse
	}
	return raw, nil
}

// LPushUsage 向 Home 服务器推送使用量数据。
func (c *Client) LPushUsage(ctx context.Context, payload []byte) error {
	cmd, errClient := c.commandClient()
	if errClient != nil {
		return errClient
	}
	if len(payload) == 0 {
		return nil
	}
	return cmd.LPush(ctx, redisKeyUsage, payload).Err()
}

// RPushRequestLog 向 Home 服务器推送请求日志。
func (c *Client) RPushRequestLog(ctx context.Context, payload []byte) error {
	cmd, errClient := c.commandClient()
	if errClient != nil {
		return errClient
	}
	if len(payload) == 0 {
		return nil
	}
	return cmd.RPush(ctx, redisKeyRequestLog, payload).Err()
}

func (c *Client) handleSubscriptionPayload(channel string, payload string, onConfig func([]byte) error) error {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(channel)) {
	case redisChannelConfig:
		if onConfig == nil {
			return nil
		}
		return onConfig([]byte(payload))
	case redisChannelCluster:
		return c.updateClusterNodesFromPayload([]byte(payload))
	default:
		return nil
	}
}

// StartConfigSubscriber 连接到 Home，通过 GET config 获取一次配置，然后订阅
// "config" 频道以接收运行时配置更新。
//
// 订阅连接被视为 Home 心跳。仅在初始 GET config 成功且 SUBSCRIBE 连接建立后，
// HeartbeatOK 才设置为 true。当订阅意外结束时，HeartbeatOK 变为 false，循环重新连接。
func (c *Client) StartConfigSubscriber(ctx context.Context, onConfig func([]byte) error) {
	if c == nil {
		return
	}
	if !c.Enabled() {
		return
	}
	if onConfig == nil {
		return
	}

	for {
		if ctx != nil {
			select {
			case <-ctx.Done():
				c.heartbeatOK.Store(false)
				return
			default:
			}
		}

		c.heartbeatOK.Store(false)
		c.Close()

		if errEnsure := c.ensureClients(); errEnsure != nil {
			log.Warn("unable to connect to home control center, retrying in 1 second")
			c.markReconnectFailure("connect")
			sleepWithContext(ctx, homeReconnectInterval)
			continue
		}

		if errPing := c.Ping(ctx); errPing != nil {
			log.Warn("unable to connect to home control center, retrying in 1 second")
			c.markReconnectFailure("ping")
			sleepWithContext(ctx, homeReconnectInterval)
			continue
		}

		raw, errGet := c.GetConfig(ctx)
		if errGet != nil {
			log.Warn("unable to fetch config from home control center, retrying in 1 second")
			c.markReconnectFailure("config fetch")
			sleepWithContext(ctx, homeReconnectInterval)
			continue
		}
		if errApply := onConfig(raw); errApply != nil {
			log.Warn("unable to apply config from home control center, retrying in 1 second")
			sleepWithContext(ctx, homeReconnectInterval)
			continue
		}

		sub, errSubClient := c.subscriptionClient()
		if errSubClient != nil {
			c.markReconnectFailure("subscribe client")
			sleepWithContext(ctx, homeReconnectInterval)
			continue
		}

		pubsub := sub.Subscribe(ctx, redisChannelConfig)
		if pubsub == nil {
			c.markReconnectFailure("subscribe")
			sleepWithContext(ctx, homeReconnectInterval)
			continue
		}

		// Ensure the subscription is established before marking heartbeat OK.
		if _, errReceive := pubsub.ReceiveTimeout(ctx, homeSubscriptionReceiveTimeout); errReceive != nil {
			_ = pubsub.Close()
			c.markReconnectFailure("subscribe")
			sleepWithContext(ctx, homeReconnectInterval)
			continue
		}

		c.resetReconnectFailures()
		c.heartbeatOK.Store(true)

		for {
			event, errMsg := pubsub.ReceiveTimeout(ctx, homeSubscriptionReceiveTimeout)
			if errMsg != nil {
				_ = pubsub.Close()
				c.heartbeatOK.Store(false)
				if isTimeoutError(errMsg) {
					c.markSubscriptionTimeout()
				} else {
					c.markReconnectFailure("subscription")
				}
				sleepWithContext(ctx, homeReconnectInterval)
				break
			}
			switch msg := event.(type) {
			case *redis.Message:
				if msg == nil {
					continue
				}
				if errApply := c.handleSubscriptionPayload(msg.Channel, msg.Payload, onConfig); errApply != nil {
					if strings.EqualFold(strings.TrimSpace(msg.Channel), redisChannelCluster) {
						log.Warn("failed to apply cluster update from home control center, ignoring")
					} else {
						log.Warn("failed to apply config update from home control center, ignoring")
					}
				}
			case *redis.Pong:
				c.resetReconnectFailures()
			case *redis.Subscription:
				continue
			default:
				log.Debugf("home subscription returned unsupported message type %T", event)
			}
		}
	}
}

// isTimeoutError 检查错误是否为超时错误。
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// sleepWithContext 在指定时间内休眠，支持上下文取消。
func sleepWithContext(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	if ctx == nil {
		<-timer.C
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		return
	}
}
