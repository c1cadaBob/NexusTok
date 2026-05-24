// collector - collector_test.go
// 采集管理器（Manager）的单元测试。
// 测试覆盖以下场景：
//   - HTTP 队列消费流程：验证事件能通过 HTTP 接口拉取并正确入库，包括认证快照的补充
//   - auto 模式降级：HTTP 队列不支持时自动降级到 RESP 模式
//   - Redis Pub/Sub 订阅模式：验证事件能通过 SUBSCRIBE 订阅并正确入库
//   - RESP 模拟服务器：提供最小 RESP 协议实现用于测试 SUBSCRIBE 消费
package collector

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seakee/cpa-manager/usage-service/internal/config"
	"github.com/seakee/cpa-manager/usage-service/internal/store"
)

// TestManagerConsumesHTTPUsageQueue 验证采集管理器通过 HTTP 队列消费使用量事件的完整流程。
// 测试内容：
// 1. 启动模拟的上游 CPA 服务（支持 auth-files 和 usage-queue 接口）
// 2. 启动采集管理器，验证事件能被正确解析并入库
// 3. 验证认证快照（account、label、project_id）被正确补充到事件中
// 4. 验证采集器状态（transport、totalInserted）正确反映运行情况
func TestManagerConsumesHTTPUsageQueue(t *testing.T) {
	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v0/management/auth-files" {
			if r.Header.Get("Authorization") != "Bearer management-key" {
				http.Error(w, "bad key", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"files":[{"auth_index":"auth-1","account":"alice@example.com","label":"Alice","name":"alice.json","provider":"codex","project_id":"vertex-project-1"}]}`))
			return
		}
		if r.URL.Path != "/v0/management/usage-queue" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer management-key" {
			http.Error(w, "bad key", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt32(&calls, 1) == 1 {
			_, _ = w.Write([]byte(`[{
				"timestamp": "2026-05-06T00:00:00Z",
				"model": "gpt-test",
				"endpoint": "POST /v1/chat/completions",
				"auth_index": "auth-1",
				"input_tokens": 10,
				"output_tokens": 5
			}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(upstream.Close)

	db := newTestStore(t)
	cfg := testConfig(t, "auto")
	manager := NewManager(cfg, db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager.Start(ctx, RuntimeConfig{
		CPAUpstreamURL: upstream.URL,
		ManagementKey:  "management-key",
	})

	waitFor(t, func() bool {
		events, _, err := db.Counts(context.Background())
		return err == nil && events == 1
	})

	status := manager.Status()
	if status.Transport != "http" {
		t.Fatalf("transport = %q, want http", status.Transport)
	}
	if status.TotalInserted != 1 {
		t.Fatalf("total inserted = %d, want 1", status.TotalInserted)
	}
	events, err := db.RecentEvents(context.Background(), 10)
	if err != nil {
		t.Fatalf("recent events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].AccountSnapshot != "alice@example.com" {
		t.Fatalf("account snapshot = %q", events[0].AccountSnapshot)
	}
	if events[0].AuthLabelSnapshot != "Alice" {
		t.Fatalf("auth label snapshot = %q", events[0].AuthLabelSnapshot)
	}
	if events[0].AuthProjectIDSnapshot != "vertex-project-1" {
		t.Fatalf("auth project id snapshot = %q", events[0].AuthProjectIDSnapshot)
	}
}

// TestManagerFallsBackToRESPWhenHTTPQueueUnsupported 验证 auto 模式下的降级行为。
// 当上游不支持 HTTP usage-queue 接口（返回 404）时，采集器应自动降级到 RESP 模式。
// 验证 transport 状态从 http 切换为 resp，并且最后错误信息包含 RESP 相关提示。
func TestManagerFallsBackToRESPWhenHTTPQueueUnsupported(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(upstream.Close)

	db := newTestStore(t)
	cfg := testConfig(t, "auto")
	manager := NewManager(cfg, db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager.Start(ctx, RuntimeConfig{
		CPAUpstreamURL: upstream.URL,
		ManagementKey:  "management-key",
	})

	waitFor(t, func() bool {
		status := manager.Status()
		return status.Transport == "resp" && strings.Contains(status.LastError, "unsupported RESP prefix")
	})
}

// newTestStore 创建一个临时的 SQLite 存储实例用于测试。
// 数据库文件在测试结束后自动清理。
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

// testConfig 创建测试用的配置对象，使用临时目录存储数据库。
// 参数 mode 指定采集模式（auto/http/resp/subscribe）。
func testConfig(t *testing.T, mode string) config.Config {
	t.Helper()
	return config.Config{
		DBPath:        filepath.Join(t.TempDir(), "usage.sqlite"),
		CollectorMode: mode,
		Queue:         "usage",
		PopSide:       "right",
		BatchSize:     10,
		PollInterval:  10 * time.Millisecond,
	}
}

// waitFor 轮询等待指定条件满足，超时时间为 2 秒。
// 每 10 毫秒检查一次条件，超时未满足则标记测试失败。
// 用于异步操作的测试等待。
func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before deadline")
}

// startMockRESPServer 启动一个最小 RESP 模拟服务端，支持 AUTH/SUBSCRIBE/PING，
// 订阅成功后将 payloads 依次以 message 帧推送给客户端。
func startMockRESPServer(t *testing.T, payloads []string) (upstreamURL string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for {
			args, err := readRESPCommand(reader)
			if err != nil {
				return
			}
			if len(args) == 0 {
				return
			}
			switch strings.ToUpper(args[0]) {
			case "AUTH":
				_, _ = conn.Write([]byte("+OK\r\n"))
			case "SUBSCRIBE":
				if len(args) < 2 {
					return
				}
				channel := args[1]
				_, _ = conn.Write([]byte(fmt.Sprintf("*3\r\n$9\r\nsubscribe\r\n$%d\r\n%s\r\n:1\r\n", len(channel), channel)))
				for _, payload := range payloads {
					_, _ = conn.Write([]byte(fmt.Sprintf("*3\r\n$7\r\nmessage\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(channel), channel, len(payload), payload)))
				}
			case "PING":
				_, _ = conn.Write([]byte("+PONG\r\n"))
			default:
				return
			}
		}
	}()

	return "http://" + listener.Addr().String()
}

// readRESPCommand 从 RESP 连接中解析一条命令。
// 按 RESP 协议格式读取数组头部（*N）和 N 个批量字符串（$N\r\n...）。
// 返回命令参数列表。
func readRESPCommand(reader *bufio.Reader) ([]string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	header = strings.TrimRight(header, "\r\n")
	if !strings.HasPrefix(header, "*") {
		return nil, fmt.Errorf("expected array header, got %q", header)
	}
	argc, err := strconv.Atoi(header[1:])
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, argc)
	for i := 0; i < argc; i++ {
		lengthLine, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		lengthLine = strings.TrimRight(lengthLine, "\r\n")
		if !strings.HasPrefix(lengthLine, "$") {
			return nil, fmt.Errorf("expected bulk header, got %q", lengthLine)
		}
		length, err := strconv.Atoi(lengthLine[1:])
		if err != nil {
			return nil, err
		}
		buf := make([]byte, length+2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:length]))
	}
	return args, nil
}

// TestManagerConsumesSubscribeStream 验证采集管理器通过 Redis Pub/Sub 订阅模式消费使用量事件。
// 测试流程：
// 1. 启动模拟 RESP 服务端，发送一条使用量消息
// 2. 以 subscribe 模式启动采集管理器
// 3. 验证事件被正确接收并入库
// 4. 验证采集器 transport 状态为 "subscribe"
func TestManagerConsumesSubscribeStream(t *testing.T) {
	payload := `{"timestamp":"2026-05-19T10:00:00Z","model":"gpt-test","endpoint":"POST /v1/chat/completions","input_tokens":10,"output_tokens":3}`
	upstreamURL := startMockRESPServer(t, []string{payload})

	db := newTestStore(t)
	cfg := testConfig(t, "subscribe")
	manager := NewManager(cfg, db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager.Start(ctx, RuntimeConfig{
		CPAUpstreamURL: upstreamURL,
		ManagementKey:  "management-key",
		Queue:          "usage",
	})

	waitFor(t, func() bool {
		events, _, err := db.Counts(context.Background())
		return err == nil && events == 1
	})

	status := manager.Status()
	if status.Transport != "subscribe" {
		t.Fatalf("transport = %q, want subscribe", status.Transport)
	}
	if status.TotalInserted != 1 {
		t.Fatalf("total inserted = %d, want 1", status.TotalInserted)
	}
}
