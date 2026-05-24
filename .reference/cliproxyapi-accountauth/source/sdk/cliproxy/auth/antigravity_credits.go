// 包 auth - antigravity_credits.go
// 该文件提供了 Antigravity AI 积分（Credits）的上下文控制和状态缓存。
// 包括积分请求标志、积分状态提示的存储和检索等功能。
package auth

import (
	"context"
	"strings"
	"sync"
	"time"
)

type antigravityUseCreditsContextKey struct{} // 上下文键类型，用于标记启用积分

// WithAntigravityCredits 返回一个子上下文，信号执行器将 enabledCreditTypes 注入请求载荷。
func WithAntigravityCredits(ctx context.Context) context.Context {
	return context.WithValue(ctx, antigravityUseCreditsContextKey{}, true)
}

// AntigravityCreditsRequested 报告上下文是否携带积分标志。
func AntigravityCreditsRequested(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(antigravityUseCreditsContextKey{}).(bool)
	return v
}

// AntigravityCreditsHint 存储一个认证的最新已知 AI 积分状态。
type AntigravityCreditsHint struct {
	Known           bool      // 是否已知积分状态
	Available       bool      // 积分是否可用
	CreditAmount    float64   // 当前积分余额
	MinCreditAmount float64   // 最低积分余额阈值
	PaidTierID      string    // 付费层级 ID
	UpdatedAt       time.Time // 最后更新时间
}

var antigravityCreditsHintByAuth sync.Map // 认证 ID 到积分状态提示的并发安全映射

// SetAntigravityCreditsHint 更新一个认证的最新已知 AI 积分状态。
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
