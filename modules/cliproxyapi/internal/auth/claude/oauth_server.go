// claude - oauth_server.go
// 实现 Claude OAuth 回调的本地 HTTP 服务器，包括授权码接收、成功页面展示、
// 端口可用性检测等功能。
package claude

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// OAuthServer 处理 OAuth 回调的本地 HTTP 服务器。
// 它监听来自 OAuth 提供者的授权码响应，并捕获完成认证流程所需的参数。
type OAuthServer struct {
	// server 是底层的 HTTP 服务器实例
	server *http.Server
	// port 是服务器监听的端口号
	port int
	// resultChan 用于发送 OAuth 结果的通道
	resultChan chan *OAuthResult
	// errorChan 用于发送 OAuth 错误的通道
	errorChan chan error
	// mu 用于保护服务器状态的互斥锁
	mu sync.Mutex
	// running 指示服务器是否正在运行
	running bool
}

// OAuthResult 包含 OAuth 回调的结果。
// 它保存成功的授权码和 state 参数，或认证失败时的错误消息。
type OAuthResult struct {
	// Code 是从 OAuth 提供者收到的授权码
	Code string
	// State 是用于防止 CSRF 攻击的 state 参数
	State string
	// Error 包含 OAuth 流程失败时的错误消息
	Error string
}

// NewOAuthServer 创建新的 OAuth 回调服务器。
// 使用指定端口初始化服务器，并创建用于处理 OAuth 结果和错误的通道。
//
// 参数:
//   - port: 服务器监听的端口号
//
// 返回值:
//   - *OAuthServer: 新的 OAuthServer 实例
func NewOAuthServer(port int) *OAuthServer {
	return &OAuthServer{
		port:       port,
		resultChan: make(chan *OAuthResult, 1),
		errorChan:  make(chan error, 1),
	}
}

// Start 启动 OAuth 回调服务器。
// 设置回调和成功端点的 HTTP 处理器，并开始在指定端口上监听。
//
// 返回值:
//   - error: 服务器启动失败时返回错误
func (s *OAuthServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server is already running")
	}

	// 检查端口是否可用
	if !s.isPortAvailable() {
		return fmt.Errorf("port %d is already in use", s.port)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", s.handleCallback)
	mux.HandleFunc("/success", s.handleSuccess)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	s.running = true

	// 在协程中启动服务器
	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.errorChan <- fmt.Errorf("server failed to start: %w", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Stop 优雅地停止 OAuth 回调服务器。
// 使用超时上下文执行 HTTP 服务器的优雅关闭。
//
// 参数:
//   - ctx: 用于控制关闭过程的上下文
//
// 返回值:
//   - error: 服务器未能优雅停止时返回错误
func (s *OAuthServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.server == nil {
		return nil
	}

	log.Debug("Stopping OAuth callback server")

	// 创建带超时的上下文用于关闭
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := s.server.Shutdown(shutdownCtx)
	s.running = false
	s.server = nil

	return err
}

// WaitForCallback 等待 OAuth 回调（带超时）。
// 阻塞直到收到 OAuth 结果、发生错误或达到指定超时。
//
// 参数:
//   - timeout: 等待回调的最长时间
//
// 返回值:
//   - *OAuthResult: 成功时返回 OAuth 结果
//   - error: 回调超时或发生错误时返回错误
func (s *OAuthServer) WaitForCallback(timeout time.Duration) (*OAuthResult, error) {
	select {
	case result := <-s.resultChan:
		return result, nil
	case err := <-s.errorChan:
		return nil, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for OAuth callback")
	}
}

// handleCallback 处理 OAuth 回调端点。
// 从回调 URL 中提取授权码和 state 参数，验证参数，并将结果发送到等待通道。
//
// 参数:
//   - w: HTTP 响应写入器
//   - r: HTTP 请求
func (s *OAuthServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	log.Debug("Received OAuth callback")

	// 验证请求方法
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 提取参数
	query := r.URL.Query()
	code := query.Get("code")
	state := query.Get("state")
	errorParam := query.Get("error")

	// 验证必需参数
	if errorParam != "" {
		log.Errorf("OAuth error received: %s", errorParam)
		result := &OAuthResult{
			Error: errorParam,
		}
		s.sendResult(result)
		http.Error(w, fmt.Sprintf("OAuth error: %s", errorParam), http.StatusBadRequest)
		return
	}

	if code == "" {
		log.Error("No authorization code received")
		result := &OAuthResult{
			Error: "no_code",
		}
		s.sendResult(result)
		http.Error(w, "No authorization code received", http.StatusBadRequest)
		return
	}

	if state == "" {
		log.Error("No state parameter received")
		result := &OAuthResult{
			Error: "no_state",
		}
		s.sendResult(result)
		http.Error(w, "No state parameter received", http.StatusBadRequest)
		return
	}

	// 发送成功结果
	result := &OAuthResult{
		Code:  code,
		State: state,
	}
	s.sendResult(result)

	// 重定向到成功页面
	http.Redirect(w, r, "/success", http.StatusFound)
}

// handleSuccess 处理成功页面端点。
// 提供一个用户友好的 HTML 页面，指示认证已成功。
//
// 参数:
//   - w: HTTP 响应写入器
//   - r: HTTP 请求
func (s *OAuthServer) handleSuccess(w http.ResponseWriter, r *http.Request) {
	log.Debug("Serving success page")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// 解析查询参数用于自定义
	query := r.URL.Query()
	setupRequired := query.Get("setup_required") == "true"
	platformURL := query.Get("platform_url")
	if platformURL == "" {
		platformURL = "https://console.anthropic.com/"
	}

	// 生成带有动态内容的成功页面 HTML
	successHTML := s.generateSuccessHTML(setupRequired, platformURL)

	_, err := w.Write([]byte(successHTML))
	if err != nil {
		log.Errorf("Failed to write success page: %v", err)
	}
}

// generateSuccessHTML 生成成功页面的 HTML 内容。
// 根据是否需要额外设置来定制页面，并包含平台链接。
//
// 参数:
//   - setupRequired: 认证后是否需要额外设置
//   - platformURL: 用于额外设置的平台 URL
//
// 返回值:
//   - string: 成功页面的 HTML 内容
func (s *OAuthServer) generateSuccessHTML(setupRequired bool, platformURL string) string {
	html := LoginSuccessHtml

	// 替换平台 URL 占位符
	html = strings.Replace(html, "{{PLATFORM_URL}}", platformURL, -1)

	// 如果需要额外设置则添加设置提示
	if setupRequired {
		setupNotice := strings.Replace(SetupNoticeHtml, "{{PLATFORM_URL}}", platformURL, -1)
		html = strings.Replace(html, "{{SETUP_NOTICE}}", setupNotice, 1)
	} else {
		html = strings.Replace(html, "{{SETUP_NOTICE}}", "", 1)
	}

	return html
}

// sendResult 将 OAuth 结果发送到等待通道。
// 确保结果发送不会阻塞处理器。
//
// 参数:
//   - result: 要发送的 OAuth 结果
func (s *OAuthServer) sendResult(result *OAuthResult) {
	select {
	case s.resultChan <- result:
		log.Debug("OAuth result sent to channel")
	default:
		log.Warn("OAuth result channel is full, result dropped")
	}
}

// isPortAvailable 检查指定端口是否可用。
// 通过尝试在该端口上监听来判断可用性。
//
// 返回值:
//   - bool: 端口可用返回 true，否则返回 false
func (s *OAuthServer) isPortAvailable() bool {
	addr := fmt.Sprintf(":%d", s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	defer func() {
		_ = listener.Close()
	}()
	return true
}

// IsRunning 返回服务器是否正在运行。
//
// 返回值:
//   - bool: 服务器运行中返回 true，否则返回 false
func (s *OAuthServer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
