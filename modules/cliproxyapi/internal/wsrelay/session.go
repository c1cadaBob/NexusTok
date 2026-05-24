// wsrelay - session.go
// 本文件实现了 WebSocket 中继的单个会话管理，负责消息的收发、心跳维持和连接生命周期管理。
// 每个会话对应一个 WebSocket 连接，维护待处理请求的映射以实现请求-响应关联。
package wsrelay

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// readTimeout 是 WebSocket 读操作的超时时间。
	readTimeout          = 60 * time.Second
	// writeTimeout 是 WebSocket 写操作的超时时间。
	writeTimeout         = 10 * time.Second
	// maxInboundMessageLen 是允许的最大入站消息长度（64 MiB）。
	maxInboundMessageLen = 64 << 20 // 64 MiB
	// heartbeatInterval 是心跳消息的发送间隔。
	heartbeatInterval    = 30 * time.Second
)

// errClosed 是会话关闭时返回的哨兵错误。
var errClosed = errors.New("websocket session closed")

// pendingRequest 代表一个等待响应的待处理请求。
// 包含用于接收响应的 channel 和一次性关闭保护。
type pendingRequest struct {
	// ch 是用于接收响应消息的 channel。
	ch        chan Message
	// closeOnce 确保 channel 只被关闭一次。
	closeOnce sync.Once
}

// close 安全地关闭待处理请求的 channel。
func (pr *pendingRequest) close() {
	if pr == nil {
		return
	}
	pr.closeOnce.Do(func() {
		close(pr.ch)
	})
}

// session 代表一个 WebSocket 连接会话，负责消息收发和连接生命周期管理。
type session struct {
	// conn 是底层的 WebSocket 连接。
	conn       *websocket.Conn
	// manager 是所属的管理器引用。
	manager    *Manager
	// provider 是此会话关联的提供商标识。
	provider   string
	// id 是会话的唯一标识符。
	id         string
	// closed 是会话关闭信号 channel。
	closed     chan struct{}
	// closeOnce 确保会话只被关闭一次。
	closeOnce  sync.Once
	// writeMutex 保护并发写操作。
	writeMutex sync.Mutex
	// pending 存储等待响应的待处理请求，以消息 ID 为键。
	pending    sync.Map // map[string]*pendingRequest
}

// newSession 创建一个新的 WebSocket 会话实例。
func newSession(conn *websocket.Conn, mgr *Manager, id string) *session {
	s := &session{
		conn:     conn,
		manager:  mgr,
		provider: "",
		id:       id,
		closed:   make(chan struct{}),
	}
	conn.SetReadLimit(maxInboundMessageLen)
	conn.SetReadDeadline(time.Now().Add(readTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(readTimeout))
		return nil
	})
	s.startHeartbeat()
	return s
}

func (s *session) startHeartbeat() {
	if s == nil || s.conn == nil {
		return
	}
	ticker := time.NewTicker(heartbeatInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-s.closed:
				return
			case <-ticker.C:
				s.writeMutex.Lock()
				err := s.conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(writeTimeout))
				s.writeMutex.Unlock()
				if err != nil {
					s.cleanup(err)
					return
				}
			}
		}
	}()
}

func (s *session) run(ctx context.Context) {
	defer s.cleanup(errClosed)
	for {
		var msg Message
		if err := s.conn.ReadJSON(&msg); err != nil {
			s.cleanup(err)
			return
		}
		s.dispatch(msg)
	}
}

func (s *session) dispatch(msg Message) {
	if msg.Type == MessageTypePing {
		_ = s.send(context.Background(), Message{ID: msg.ID, Type: MessageTypePong})
		return
	}
	if value, ok := s.pending.Load(msg.ID); ok {
		req := value.(*pendingRequest)
		select {
		case req.ch <- msg:
		default:
		}
		if msg.Type == MessageTypeHTTPResp || msg.Type == MessageTypeError || msg.Type == MessageTypeStreamEnd {
			if actual, loaded := s.pending.LoadAndDelete(msg.ID); loaded {
				actual.(*pendingRequest).close()
			}
		}
		return
	}
	if msg.Type == MessageTypeHTTPResp || msg.Type == MessageTypeError || msg.Type == MessageTypeStreamEnd {
		s.manager.logDebugf("wsrelay: received terminal message for unknown id %s (provider=%s)", msg.ID, s.provider)
	}
}

func (s *session) send(ctx context.Context, msg Message) error {
	select {
	case <-s.closed:
		return errClosed
	default:
	}
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	if err := s.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	if err := s.conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}

func (s *session) request(ctx context.Context, msg Message) (<-chan Message, error) {
	if msg.ID == "" {
		return nil, fmt.Errorf("wsrelay: message id is required")
	}
	if _, loaded := s.pending.LoadOrStore(msg.ID, &pendingRequest{ch: make(chan Message, 8)}); loaded {
		return nil, fmt.Errorf("wsrelay: duplicate message id %s", msg.ID)
	}
	value, _ := s.pending.Load(msg.ID)
	req := value.(*pendingRequest)
	if err := s.send(ctx, msg); err != nil {
		if actual, loaded := s.pending.LoadAndDelete(msg.ID); loaded {
			req := actual.(*pendingRequest)
			req.close()
		}
		return nil, err
	}
	go func() {
		select {
		case <-ctx.Done():
			if actual, loaded := s.pending.LoadAndDelete(msg.ID); loaded {
				actual.(*pendingRequest).close()
			}
		case <-s.closed:
		}
	}()
	return req.ch, nil
}

func (s *session) cleanup(cause error) {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.pending.Range(func(key, value any) bool {
			req := value.(*pendingRequest)
			msg := Message{ID: key.(string), Type: MessageTypeError, Payload: map[string]any{"error": cause.Error()}}
			select {
			case req.ch <- msg:
			default:
			}
			req.close()
			return true
		})
		s.pending = sync.Map{}
		_ = s.conn.Close()
		if s.manager != nil {
			s.manager.handleSessionClosed(s, cause)
		}
	})
}
