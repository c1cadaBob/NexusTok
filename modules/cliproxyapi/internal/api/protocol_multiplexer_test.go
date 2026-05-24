// api - protocol_multiplexer_test.go
// 该文件测试协议多路复用器的行为。
// 验证空闲 TCP 连接不会阻塞接受循环中的其他连接处理。

package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestAcceptMuxNotBlockedByIdleConnection 测试空闲 TCP 连接不会阻塞接受循环。
// 修复前，接受循环会在空闲连接的 Peek(1) 上阻塞，导致正常请求超时。
func TestAcceptMuxNotBlockedByIdleConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	var routed atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routed.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewUnstartedServer(handler)
	defer srv.Close()

	muxLn := newMuxListener(listener.Addr(), 1024)
	server := &Server{managementRoutesEnabled: atomic.Bool{}}
	server.managementRoutesEnabled.Store(false)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.acceptMuxConnections(listener, muxLn)
	}()

	srv.Listener = muxLn
	srv.Start()

	// Open an idle TCP connection that never sends any bytes.
	idleConn, err := net.DialTimeout("tcp", listener.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to dial idle connection: %v", err)
	}
	defer idleConn.Close()

	// Give the accept loop time to pick up the idle connection.
	time.Sleep(50 * time.Millisecond)

	// Send a real HTTP request. Before the fix, the accept loop would be
	// blocked on Peek(1) for the idle connection, causing this request to
	// time out.
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + listener.Addr().String() + "/")
	if err != nil {
		listener.Close()
		t.Fatalf("HTTP request failed (accept loop may be blocked by idle connection): %v", err)
	}
	resp.Body.Close()

	listener.Close()

	if routed.Load() == 0 {
		t.Error("expected at least one request to be routed")
	}
}
