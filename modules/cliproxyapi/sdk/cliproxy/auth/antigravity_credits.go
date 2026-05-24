// auth - antigravity_credits.go
// 该文件实现了 Antigravity AI 积分的上下文传递、提示存储和可用性检查功能。
// 用于在请求执行链中携带积分启用标志，并缓存每个认证凭据的积分状态。
package auth

import (
	"context"
	"strings"
	"sync"
	"time"
)

// antigravityUseCreditsContextKey 是上下文中 Antigravity 积分启用标志的键类型。
type antigravityUseCreditsContextKey struct{}

// WithAntigravityCredits returns a child context that signals the executor to
// inject enabledCreditTypes into the request payload.
func WithAntigravityCredits(ctx context.Context) context.Context {
	return context.WithValue(ctx, antigravityUseCreditsContextKey{}, true)
}

// AntigravityCreditsRequested reports whether the context carries the credits flag.
func AntigravityCreditsRequested(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(antigravityUseCreditsContextKey{}).(bool)
	return v
}

// AntigravityCreditsHint 存储单个认证凭据的最新 AI 积分状态信息。
type AntigravityCreditsHint struct {
	Known           bool      // 是否已发现积分状态
	Available       bool      // 积分是否可用
	CreditAmount    float64   // 当前积分余额
	MinCreditAmount float64   // 最低积分要求
	PaidTierID      string    // 付费层级 ID
	UpdatedAt       time.Time // 最后更新时间
}

// antigravityCreditsHintByAuth 存储每个认证凭据的积分提示信息（线程安全）。
var antigravityCreditsHintByAuth sync.Map

// SetAntigravityCreditsHint updates the latest known AI credits state for an auth.
func SetAntigravityCreditsHint(authID string, hint AntigravityCreditsHint) {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return
	}
	if hint.UpdatedAt.IsZero() {
		hint.UpdatedAt = time.Now()
	}
	antigravityCreditsHintByAuth.Store(authID, hint)
}

// GetAntigravityCreditsHint returns the latest known AI credits state for an auth.
func GetAntigravityCreditsHint(authID string) (AntigravityCreditsHint, bool) {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return AntigravityCreditsHint{}, false
	}
	value, ok := antigravityCreditsHintByAuth.Load(authID)
	if !ok {
		return AntigravityCreditsHint{}, false
	}
	hint, ok := value.(AntigravityCreditsHint)
	if !ok {
		antigravityCreditsHintByAuth.Delete(authID)
		return AntigravityCreditsHint{}, false
	}
	return hint, true
}

// HasKnownAntigravityCreditsHint reports whether credits state has been discovered for an auth.
func HasKnownAntigravityCreditsHint(authID string) bool {
	hint, ok := GetAntigravityCreditsHint(authID)
	return ok && hint.Known
}

// antigravityCreditsAvailableForModel 检查指定认证凭据对指定模型是否有可用的 Antigravity 积分。
// 仅对 Antigravity 提供商的 Claude 模型有效。
func antigravityCreditsAvailableForModel(auth *Auth, model string) bool {
	if auth == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(auth.Provider), "antigravity") {
		return false
	}
	if !strings.Contains(strings.ToLower(strings.TrimSpace(model)), "claude") {
		return false
	}
	hint, ok := GetAntigravityCreditsHint(auth.ID)
	if !ok || !hint.Known {
		return false
	}
	return hint.Available
}
