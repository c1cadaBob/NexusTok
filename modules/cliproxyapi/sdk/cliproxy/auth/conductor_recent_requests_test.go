// auth - conductor_recent_requests_test.go
// 测试 Manager 的最近请求记录功能，验证 MarkResult 正确记录成功/失败计数，
// 以及 Update 操作能保留已有的请求统计和总计数据。
package auth

import (
	"context"
	"testing"
	"time"
)

// TestManagerMarkResultRecordsRecentRequests 测试 MarkResult 正确累加成功和失败计数，
// 并通过 RecentRequestsSnapshot 返回一致的统计快照。
func TestManagerMarkResultRecordsRecentRequests(t *testing.T) {
	mgr := NewManager(nil, nil, nil)
	auth := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Attributes: map[string]string{
			"runtime_only": "true",
		},
		Metadata: map[string]any{
			"type": "antigravity",
		},
	}

	if _, err := mgr.Register(WithSkipPersist(context.Background()), auth); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	mgr.MarkResult(context.Background(), Result{AuthID: "auth-1", Provider: "antigravity", Model: "gpt-5", Success: true})
	mgr.MarkResult(context.Background(), Result{AuthID: "auth-1", Provider: "antigravity", Model: "gpt-5", Success: false})

	gotAuth, ok := mgr.GetByID("auth-1")
	if !ok || gotAuth == nil {
		t.Fatalf("GetByID returned ok=%v auth=%v", ok, gotAuth)
	}

	if gotAuth.Success != 1 || gotAuth.Failed != 1 {
		t.Fatalf("auth totals = success=%d failed=%d, want 1/1", gotAuth.Success, gotAuth.Failed)
	}

	snapshot := gotAuth.RecentRequestsSnapshot(time.Now())
	var successTotal int64
	var failedTotal int64
	for _, bucket := range snapshot {
		successTotal += bucket.Success
		failedTotal += bucket.Failed
	}
	if successTotal != 1 || failedTotal != 1 {
		t.Fatalf("totals = success=%d failed=%d, want 1/1", successTotal, failedTotal)
	}
}

// TestManagerUpdatePreservesRecentRequestsAndTotals 测试 Update 操作能保留已有的
// 请求统计数据和成功/失败总计。
func TestManagerUpdatePreservesRecentRequestsAndTotals(t *testing.T) {
	mgr := NewManager(nil, nil, nil)
	auth := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Metadata: map[string]any{
			"type": "antigravity",
		},
	}
	if _, err := mgr.Register(WithSkipPersist(context.Background()), auth); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	mgr.MarkResult(context.Background(), Result{AuthID: "auth-1", Provider: "antigravity", Model: "gpt-5", Success: true})

	updated := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Metadata: map[string]any{
			"type": "antigravity",
			"note": "updated",
		},
	}
	if _, err := mgr.Update(WithSkipPersist(context.Background()), updated); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	gotAuth, ok := mgr.GetByID("auth-1")
	if !ok || gotAuth == nil {
		t.Fatalf("GetByID returned ok=%v auth=%v", ok, gotAuth)
	}
	if gotAuth.Success != 1 || gotAuth.Failed != 0 {
		t.Fatalf("auth totals = success=%d failed=%d, want 1/0", gotAuth.Success, gotAuth.Failed)
	}

	snapshot := gotAuth.RecentRequestsSnapshot(time.Now())
	var successTotal int64
	var failedTotal int64
	for _, bucket := range snapshot {
		successTotal += bucket.Success
		failedTotal += bucket.Failed
	}
	if successTotal != 1 || failedTotal != 0 {
		t.Fatalf("bucket totals = success=%d failed=%d, want 1/0", successTotal, failedTotal)
	}
}
