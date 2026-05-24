// redisqueue - queue_test.go
// 该文件包含使用量队列广播和订阅者行为的单元测试，验证 Enqueue 在有订阅者时
// 直接广播而不入队、取消订阅后消息回退到队列、以及 SetEnabled(false) 关闭订阅者通道的行为。
package redisqueue

import (
	"testing"
	"time"
)

// TestEnqueueBroadcastsToUsageSubscribersAndSkipsQueue 测试 Enqueue 在存在使用量订阅者时
// 将消息直接广播给所有订阅者而非入队，取消订阅后消息回退到队列存储。
func TestEnqueueBroadcastsToUsageSubscribersAndSkipsQueue(t *testing.T) {
	withEnabledQueue(t, func() {
		first, unsubscribeFirst := SubscribeUsage()
		defer unsubscribeFirst()
		second, unsubscribeSecond := SubscribeUsage()
		defer unsubscribeSecond()

		Enqueue([]byte("usage-record"))

		requireUsageSubscriberPayload(t, first, "usage-record")
		requireUsageSubscriberPayload(t, second, "usage-record")

		if items := PopOldest(1); len(items) != 0 {
			t.Fatalf("PopOldest() items = %q, want empty after subscriber broadcast", items)
		}

		unsubscribeFirst()
		unsubscribeSecond()

		Enqueue([]byte("queued-record"))
		items := PopOldest(1)
		if len(items) != 1 || string(items[0]) != "queued-record" {
			t.Fatalf("PopOldest() items = %q, want queued record after unsubscribe", items)
		}
	})
}

// TestSetEnabledFalseClosesUsageSubscribers 测试调用 SetEnabled(false) 时
// 所有使用量订阅者的通道会被正确关闭。
func TestSetEnabledFalseClosesUsageSubscribers(t *testing.T) {
	withEnabledQueue(t, func() {
		subscriber, unsubscribe := SubscribeUsage()
		defer unsubscribe()

		SetEnabled(false)

		select {
		case _, ok := <-subscriber:
			if ok {
				t.Fatalf("subscriber channel remained open after SetEnabled(false)")
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for subscriber close")
		}
	})
}

// requireUsageSubscriberPayload 断言订阅者通道在超时前接收到期望的载荷字符串。
func requireUsageSubscriberPayload(t *testing.T, subscriber <-chan []byte, want string) {
	t.Helper()

	select {
	case got, ok := <-subscriber:
		if !ok {
			t.Fatalf("subscriber closed before receiving %q", want)
		}
		if string(got) != want {
			t.Fatalf("subscriber payload = %q, want %q", string(got), want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for subscriber payload %q", want)
	}
}
