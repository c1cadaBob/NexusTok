// auth - conductor_availability_test.go
// 认证聚合可用性更新测试
// 验证 updateAggregatedAvailability 函数能够根据模型状态
// 正确更新认证的聚合可用性：
// - 无 NextRetryAfter 的 Unavailable 状态不阻塞认证
// - 未来时间的 NextRetryAfter 阻塞认证
package auth

import (
	"testing"
	"time"
)

// TestUpdateAggregatedAvailability_UnavailableWithoutNextRetryDoesNotBlockAuth 验证：
// 当模型状态为 Unavailable 但没有设置 NextRetryAfter 时，
// 认证整体不应被标记为不可用。
func TestUpdateAggregatedAvailability_UnavailableWithoutNextRetryDoesNotBlockAuth(t *testing.T) {
	t.Parallel()

	now := time.Now()
	model := "test-model"
	auth := &Auth{
		ID: "a",
		ModelStates: map[string]*ModelState{
			model: {
				Status:      StatusError,
				Unavailable: true,
			},
		},
	}

	updateAggregatedAvailability(auth, now)

	if auth.Unavailable {
		t.Fatalf("auth.Unavailable = true, want false")
	}
	if !auth.NextRetryAfter.IsZero() {
		t.Fatalf("auth.NextRetryAfter = %v, want zero", auth.NextRetryAfter)
	}
}

// TestUpdateAggregatedAvailability_FutureNextRetryBlocksAuth 验证：
// 当模型状态为 Unavailable 且 NextRetryAfter 为未来时间时，
// 认证整体应被标记为不可用。
func TestUpdateAggregatedAvailability_FutureNextRetryBlocksAuth(t *testing.T) {
	t.Parallel()

	now := time.Now()
	model := "test-model"
	next := now.Add(5 * time.Minute)
	auth := &Auth{
		ID: "a",
		ModelStates: map[string]*ModelState{
			model: {
				Status:         StatusError,
				Unavailable:    true,
				NextRetryAfter: next,
			},
		},
	}

	updateAggregatedAvailability(auth, now)

	if !auth.Unavailable {
		t.Fatalf("auth.Unavailable = false, want true")
	}
	if auth.NextRetryAfter.IsZero() {
		t.Fatalf("auth.NextRetryAfter = zero, want %v", next)
	}
	if auth.NextRetryAfter.Sub(next) > time.Second || next.Sub(auth.NextRetryAfter) > time.Second {
		t.Fatalf("auth.NextRetryAfter = %v, want %v", auth.NextRetryAfter, next)
	}
}
