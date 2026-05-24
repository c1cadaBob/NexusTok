// codex - oauth_server.go
// 包 codex 提供 OpenAI Codex API 的认证功能。
// 该文件实现了本地 OAuth 回调服务器，用于接收 OAuth 提供商的授权码响应，
// 并捕获完成认证流程所需的参数。
package codex

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
// 监听来自 OAuth 提供商的授权码响应，并捕获完成认证流程所需的参数。
type OAuthServer struct {
	// server 是底层的 HTTP 服务器实例
	server *http.Server
	// port 是服务器监听的端口号
	port int
	// resultChan 是用于发送 OAuth 结果的通道
	resultChan chan *OAuthResult
	// errorChan 是用于发送 OAuth 错误的通道
	errorChan chan error
	// mu 是用于保护服务器状态的互斥锁
	mu sync.Mutex
	// running 指示服务器是否正在运行
	running bool
}

// OAuthResult 包含 OAuth 回调的结果。
// 持有成功认证时的授权码和状态，或认证失败时的错误消息。
type OAuthResult struct {
	// Code 是从 OAuth 提供商收到的授权码
	Code string
	// State 是用于防止 CSRF 攻击的状态参数
	State string
	// Error 包含 OAuth 流程失败时的错误消息
	Error string
}

// NewOAuthServer 创建一个新的 OAuth 回调服务器。
// 使用指定的端口初始化服务器，并创建用于处理 OAuth 结果和错误的通道。
//
// 参数：
//   - port: 服务器监听的端口号
//
// 返回：
//   - *OAuthServer: 新的 OAuthServer 实例
func NewOAuthServer(port int) *OAuthServer {
	return &OAuthServer{
		port:       port,
		resultChan: make(chan *OAuthResult, 1),
		errorChan:  make(chan error, 1),
	}
}

// Start 启动 OAuth 回调服务器。
// 设置回调和成功端点的 HTTP 处理器，并在指定端口上开始监听。
//
// 返回：
//   - error: 服务器启动失败时返回的错误
func (s *OAuthServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server is already running")
	}

	// Check if port is available
	if !s.isPortAvailable() {
		return fmt.Errorf("port %d is already in use", s.port)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", s.handleCallback)
	mux.HandleFunc("/success", s.handleSuccess)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	s.running = true

	// Start server in goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.errorChan <- fmt.Errorf("server failed to start: %w", err)
		}
	}()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Stop 优雅地停止 OAuth 回调服务器。
// 使用超时上下文执行 HTTP 服务器的优雅关闭。
//
// 参数：
//   - ctx: 用于控制关闭过程的上下文
//
// 返回：
//   - error: 服务器未能优雅停止时返回的错误
func (s *OAuthServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.server == nil {
		return nil
	}

	log.Debug("Stopping OAuth callback server")

	// Create a context with timeout for shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := s.server.Shutdown(shutdownCtx)
	s.running = false
	s.server = nil

	return err
}

// WaitForCallback 等待 OAuth 回调，带超时控制。
// 阻塞直到收到 OAuth 结果、发生错误或达到指定的超时时间。
//
// 参数：
//   - timeout: 等待回调的最大时间
//
// 返回：
//   - *OAuthResult: 成功时的 OAuth 结果
//   - error: 回调超时或发生错误时返回的错误
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
// 从回调 URL 中提取授权码和状态参数，验证参数有效性，
// 并将结果发送到等待通道。
//
// 参数：
//   - w: HTTP 响应写入器
//   - r: HTTP 请求
func (s *OAuthServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	log.Debug("Received OAuth callback")

	// Validate request method
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract parameters
	query := r.URL.Query()
	code := query.Get("code")
	state := query.Get("state")
	errorParam := query.Get("error")

	// Validate required parameters
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

	// Send successful result
	result := &OAuthResult{
		Code:  code,
		State: state,
	}
	s.sendResult(result)

	// Redirect to success page
	http.Redirect(w, r, "/success", http.StatusFound)
}

// handleSuccess 处理成功页面端点。
// 提供一个用户友好的 HTML 页面，指示认证已成功完成。
//
// 参数：
//   - w: HTTP 响应写入器
//   - r: HTTP 请求
func (s *OAuthServer) handleSuccess(w http.ResponseWriter, r *http.Request) {
	log.Debug("Serving success page")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// Parse query parameters for customization
	query := r.URL.Query()
	setupRequired := query.Get("setup_required") == "true"
	platformURL := query.Get("platform_url")
	if platformURL == "" {
		platformURL = "https://platform.openai.com"
	}

	// Generate success page HTML with dynamic content
	successHTML := s.generateSuccessHTML(setupRequired, platformURL)

	_, err := w.Write([]byte(successHTML))
	if err != nil {
		log.Errorf("Failed to write success page: %v", err)
	}
}

// generateSuccessHTML 创建成功页面的 HTML 内容。
// 根据是否需要额外设置来自定义页面内容，并包含平台链接。
//
// 参数：
//   - setupRequired: 认证后是否需要额外设置
//   - platformURL: 用于额外设置的平台 URL
//
// 返回：
//   - string: 成功页面的 HTML 内容
func (s *OAuthServer) generateSuccessHTML(setupRequired bool, platformURL string) string {
	html := LoginSuccessHtml

	// Replace platform URL placeholder
	html = strings.Replace(html, "{{PLATFORM_URL}}", platformURL, -1)

	// Add setup notice if required
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
// 参数：
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
// 尝试在该端口上监听以确定其可用性。
//
// 返回：
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
// 返回：
//   - bool: 服务器正在运行返回 true，否则返回 false
func (s *OAuthServer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
