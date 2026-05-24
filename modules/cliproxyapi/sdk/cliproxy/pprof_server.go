// cliproxy - pprof_server.go
// 该文件实现了可选的 pprof HTTP 调试服务器。
// 支持动态启用/禁用、地址变更时的热重载，以及优雅关闭。
// pprof 服务器提供 Go 运行时性能分析端点。

package cliproxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/pprof"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	log "github.com/sirupsen/logrus"
)

// pprofServer 管理可选的 pprof HTTP 调试服务器。
// 支持动态启用/禁用和地址变更时的热重载。
type pprofServer struct {
	mu      sync.Mutex
	server  *http.Server
	addr    string
	enabled bool
}

// newPprofServer 创建一个新的 pprof 服务器实例。
func newPprofServer() *pprofServer {
	return &pprofServer{}
}

// applyPprofConfig 将 pprof 配置应用到服务实例。
// 如果 pprof 服务器尚未初始化，则创建新实例。
func (s *Service) applyPprofConfig(cfg *config.Config) {
	if s == nil || cfg == nil {
		return
	}
	if s.pprofServer == nil {
		s.pprofServer = newPprofServer()
	}
	s.pprofServer.Apply(cfg)
}

// shutdownPprof 关闭 pprof 服务器。
func (s *Service) shutdownPprof(ctx context.Context) error {
	if s == nil || s.pprofServer == nil {
		return nil
	}
	return s.pprofServer.Shutdown(ctx)
}

// Apply 根据配置启用或禁用 pprof 服务器。
// 如果地址变更，会先停止旧服务器再启动新服务器。
func (p *pprofServer) Apply(cfg *config.Config) {
	if p == nil || cfg == nil {
		return
	}
	addr := strings.TrimSpace(cfg.Pprof.Addr)
	if addr == "" {
		addr = config.DefaultPprofAddr
	}
	enabled := cfg.Pprof.Enable

	p.mu.Lock()
	currentServer := p.server
	currentAddr := p.addr
	p.addr = addr
	p.enabled = enabled
	if !enabled {
		p.server = nil
		p.mu.Unlock()
		if currentServer != nil {
			p.stopServer(currentServer, currentAddr, "disabled")
		}
		return
	}
	if currentServer != nil && currentAddr == addr {
		p.mu.Unlock()
		return
	}
	p.server = nil
	p.mu.Unlock()

	if currentServer != nil {
		p.stopServer(currentServer, currentAddr, "restarted")
	}

	p.startServer(addr)
}

// Shutdown 优雅关闭 pprof 服务器，使用提供的上下文控制超时。
func (p *pprofServer) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	currentServer := p.server
	currentAddr := p.addr
	p.server = nil
	p.enabled = false
	p.mu.Unlock()

	if currentServer == nil {
		return nil
	}
	return p.stopServerWithContext(ctx, currentServer, currentAddr, "shutdown")
}

// startServer 在指定地址启动 pprof HTTP 服务器。
func (p *pprofServer) startServer(addr string) {
	mux := newPprofMux()
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	p.mu.Lock()
	if !p.enabled || p.addr != addr || p.server != nil {
		p.mu.Unlock()
		return
	}
	p.server = server
	p.mu.Unlock()

	log.Infof("pprof server starting on %s", addr)
	go func() {
		if errServe := server.ListenAndServe(); errServe != nil && !errors.Is(errServe, http.ErrServerClosed) {
			log.Errorf("pprof server failed on %s: %v", addr, errServe)
			p.mu.Lock()
			if p.server == server {
				p.server = nil
			}
			p.mu.Unlock()
		}
	}()
}

// stopServer 使用默认上下文停止 pprof 服务器。
func (p *pprofServer) stopServer(server *http.Server, addr string, reason string) {
	_ = p.stopServerWithContext(context.Background(), server, addr, reason)
}

// stopServerWithContext 使用提供的上下文停止 pprof 服务器。
func (p *pprofServer) stopServerWithContext(ctx context.Context, server *http.Server, addr string, reason string) error {
	if server == nil {
		return nil
	}
	stopCtx := ctx
	if stopCtx == nil {
		stopCtx = context.Background()
	}
	stopCtx, cancel := context.WithTimeout(stopCtx, 5*time.Second)
	defer cancel()
	if errStop := server.Shutdown(stopCtx); errStop != nil {
		log.Errorf("pprof server stop failed on %s: %v", addr, errStop)
		return errStop
	}
	log.Infof("pprof server stopped on %s (%s)", addr, reason)
	return nil
}

// newPprofMux 创建注册了所有标准 pprof 端点的 HTTP 路由复用器。
func newPprofMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	return mux
}
