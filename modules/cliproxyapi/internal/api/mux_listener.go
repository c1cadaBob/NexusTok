// api - mux_listener.go
// 协议多路复用虚拟监听器实现。
// 该模块提供了一个虚拟的 net.Listener 实现，用于在协议多路复用场景中
// 将不同协议的连接分发到各自对应的处理器。它通过带缓冲的 channel
// 实现连接的异步传递，支持优雅关闭。
package api

import (
	"net"
	"sync"
)

// muxListener 是一个虚拟网络监听器，实现了 net.Listener 接口。
// 它不直接监听网络端口，而是通过 channel 接收由协议多路复用器
// 路由过来的连接。每个协议（HTTP、Redis 等）都有自己的 muxListener 实例。
type muxListener struct {
	addr    net.Addr      // 监听器关联的网络地址
	connCh  chan net.Conn  // 连接传递通道，用于接收多路复用器路由的连接
	closeCh chan struct{}  // 关闭信号通道，用于通知监听器停止
	once    sync.Once     // 确保关闭操作只执行一次
}

// newMuxListener 创建一个新的虚拟多路复用监听器。
// 参数 addr 指定监听器关联的地址，buffer 指定连接通道的缓冲大小。
// 如果 buffer <= 0，则默认为 1。
func newMuxListener(addr net.Addr, buffer int) *muxListener {
	if buffer <= 0 {
		buffer = 1
	}
	return &muxListener{
		addr:    addr,
		connCh:  make(chan net.Conn, buffer),
		closeCh: make(chan struct{}),
	}
}

// Put 将一个网络连接放入监听器的连接通道中。
// 如果监听器已关闭，返回 net.ErrClosed 错误。
// 如果 conn 为 nil，直接返回 nil（静默忽略空连接）。
// 该方法由协议多路复用器调用，用于将路由后的连接传递给对应协议的处理器。
func (l *muxListener) Put(conn net.Conn) error {
	if conn == nil {
		return nil
	}
	select {
	case <-l.closeCh:
		return net.ErrClosed
	case l.connCh <- conn:
		return nil
	}
}

// Accept 等待并接收下一个可用的网络连接。
// 该方法会阻塞直到有新连接到达或监听器被关闭。
// 当监听器关闭时返回 net.ErrClosed 错误。
// 这是 net.Listener 接口的核心方法，供 HTTP 服务器等调用。
func (l *muxListener) Accept() (net.Conn, error) {
	select {
	case <-l.closeCh:
		return nil, net.ErrClosed
	case conn := <-l.connCh:
		if conn == nil {
			return nil, net.ErrClosed
		}
		return conn, nil
	}
}

// Close 关闭监听器，释放相关资源。
// 通过 sync.Once 确保关闭信号通道的操作只执行一次，防止重复关闭导致 panic。
// 关闭后，所有阻塞在 Accept 或 Put 上的 goroutine 都会收到通知。
func (l *muxListener) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		close(l.closeCh)
	})
	return nil
}

// Addr 返回监听器关联的网络地址。
// 当监听器为 nil 或地址未设置时，返回空的 TCPAddr。
// 这是 net.Listener 接口要求的方法。
func (l *muxListener) Addr() net.Addr {
	if l == nil {
		return &net.TCPAddr{}
	}
	if l.addr == nil {
		return &net.TCPAddr{}
	}
	return l.addr
}
