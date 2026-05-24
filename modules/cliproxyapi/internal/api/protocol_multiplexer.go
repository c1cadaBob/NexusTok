// api - protocol_multiplexer.go
// 协议多路复用器，在单个 TCP 端口上同时提供 HTTP/HTTPS 和 Redis RESP 协议服务。
// 该模块通过检测连接的前几个字节（协议嗅探）来判断连接使用的协议类型，
// 然后将连接路由到对应的处理器。支持 TLS 握手、ALPN 协议协商和 RESP 前缀检测。
package api

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// normalizeHTTPServeError 标准化 HTTP 服务错误。
// 将 net.ErrClosed 和 http.ErrServerClosed 视为正常关闭，返回 nil；
// 其他错误原样返回。用于避免在正常关闭场景下记录错误日志。
func normalizeHTTPServeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// normalizeListenerError 标准化监听器错误。
// 将 net.ErrClosed 视为正常关闭返回 nil，其他错误原样返回。
func normalizeListenerError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

// acceptMuxConnections 持续接受新的网络连接，并将每个连接分发到独立的 goroutine 进行协议路由。
// 该方法是协议多路复用器的主循环，确保慢速或空闲客户端不会阻塞连接接受过程。
// 每个连接都会在独立的 goroutine 中进行 TLS 握手和协议探测，避免单个连接阻塞整个接受循环。
func (s *Server) acceptMuxConnections(listener net.Listener, httpListener *muxListener) error {
	if s == nil || listener == nil {
		return net.ErrClosed
	}

	for {
		conn, errAccept := listener.Accept()
		if errAccept != nil {
			return errAccept
		}
		if conn == nil {
			continue
		}

		// Dispatch each connection to a goroutine so that slow/idle clients
		// cannot block the accept loop. Previously, TLS handshake and
		// reader.Peek(1) were performed inline; an idle TCP connection that
		// never sent bytes would block Peek indefinitely, preventing all
		// subsequent connections from being accepted (issue #3267).
		go s.routeMuxConnection(conn, httpListener)
	}
}

// routeMuxConnection 对单个连接执行协议检测和路由。
// 检测流程：
//  1. 设置 10 秒读取超时，防止空闲连接泄漏 goroutine 和文件描述符
//  2. 检测是否为 TLS 连接，如果是则执行 TLS 握手并通过 ALPN 判断协议
//  3. ALPN 协商为 h2 或 http/1.1 的连接路由到 HTTP 监听器
//  4. 非 TLS 连接通过读取第一个字节判断是否为 Redis RESP 协议
//  5. RESP 协议连接直接交给 Redis 处理器
//  6. 其他连接包装为 bufferedConn 后路由到 HTTP 监听器
//
// 连接成功路由后会清除读取超时，由后续处理器自行管理超时。
func (s *Server) routeMuxConnection(conn net.Conn, httpListener *muxListener) {
	// Set a read deadline so that idle connections that never send bytes do not
	// leak goroutines and file descriptors. The deadline is cleared once the
	// connection is successfully routed to its handler.
	const muxSniffDeadline = 10 * time.Second
	_ = conn.SetReadDeadline(time.Now().Add(muxSniffDeadline))

	tlsConn, ok := conn.(*tls.Conn)
	if ok {
		if errHandshake := tlsConn.Handshake(); errHandshake != nil {
			if errClose := conn.Close(); errClose != nil {
				log.Errorf("failed to close connection after TLS handshake error: %v", errClose)
			}
			return
		}
		proto := strings.TrimSpace(tlsConn.ConnectionState().NegotiatedProtocol)
		if proto == "h2" || proto == "http/1.1" {
			if httpListener == nil {
				if errClose := conn.Close(); errClose != nil {
					log.Errorf("failed to close connection: %v", errClose)
				}
				return
			}
			if errPut := httpListener.Put(tlsConn); errPut != nil {
				if errClose := conn.Close(); errClose != nil {
					log.Errorf("failed to close connection after HTTP routing failure: %v", errClose)
				}
			} else {
				_ = conn.SetReadDeadline(time.Time{})
			}
			return
		}
	}

	reader := bufio.NewReader(conn)
	prefix, errPeek := reader.Peek(1)
	if errPeek != nil {
		if errClose := conn.Close(); errClose != nil {
			log.Errorf("failed to close connection after protocol peek failure: %v", errClose)
		}
		return
	}

	if isRedisRESPPrefix(prefix[0]) {
		_ = conn.SetReadDeadline(time.Time{})
		s.handleRedisConnection(conn, reader)
		return
	}

	if httpListener == nil {
		if errClose := conn.Close(); errClose != nil {
			log.Errorf("failed to close connection without HTTP listener: %v", errClose)
		}
		return
	}

	if errPut := httpListener.Put(&bufferedConn{Conn: conn, reader: reader}); errPut != nil {
		if errClose := conn.Close(); errClose != nil {
			log.Errorf("failed to close connection after HTTP routing failure: %v", errClose)
		}
	} else {
		_ = conn.SetReadDeadline(time.Time{})
	}
}
