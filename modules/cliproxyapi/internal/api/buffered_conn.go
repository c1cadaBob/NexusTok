// api - buffered_conn.go
// 带缓冲读取器的网络连接包装器。
// 该模块通过 bufio.Reader 包装底层 net.Conn，使得在协议探测阶段读取的字节
// 能够被保留并供后续读取使用，实现"窥探（peek）"功能而不丢失数据。
package api

import (
	"bufio"
	"crypto/tls"
	"net"
)

// bufferedConn 包装了 net.Conn，为其添加 bufio.Reader 缓冲功能。
// 这在协议多路复用场景中至关重要：当需要预读取连接的前几个字节来判断协议类型时，
// 缓冲读取器确保这些已读字节不会丢失，后续的正常读取可以无缝继续。
type bufferedConn struct {
	net.Conn                    // 嵌入原始网络连接，继承其所有方法
	reader    *bufio.Reader     // 缓冲读取器，用于保存预读取的字节数据
}

// Read 从缓冲连接中读取数据。
// 如果缓冲读取器存在，则通过缓冲读取器读取（这会使用预读取阶段缓存的数据）；
// 否则直接从底层连接读取。当连接为 nil 时返回 net.ErrClosed 错误。
func (c *bufferedConn) Read(p []byte) (int, error) {
	if c == nil {
		return 0, net.ErrClosed
	}
	if c.reader == nil {
		return c.Conn.Read(p)
	}
	return c.reader.Read(p)
}

// ConnectionState 返回 TLS 连接状态。
// 如果底层连接实现了 ConnectionState() 方法（例如 tls.Conn），则委托给底层连接；
// 否则返回空的 tls.ConnectionState。当连接为 nil 时也返回空状态。
func (c *bufferedConn) ConnectionState() tls.ConnectionState {
	if c == nil || c.Conn == nil {
		return tls.ConnectionState{}
	}
	if stater, ok := c.Conn.(interface{ ConnectionState() tls.ConnectionState }); ok {
		return stater.ConnectionState()
	}
	return tls.ConnectionState{}
}
