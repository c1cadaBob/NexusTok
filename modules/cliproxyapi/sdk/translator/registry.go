// translator - registry.go
// 该文件实现了翻译注册表（Registry），用于管理不同格式之间的请求和响应转换函数。
// 注册表支持线程安全的注册和查询操作，并提供默认的全局注册表实例。
// 当没有注册转换函数时，会自动规范化请求中的 model 字段。

package translator

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Registry 管理跨格式的翻译函数注册表。
// 使用读写锁保证并发安全，支持请求转换和响应转换的独立注册。
type Registry struct {
	mu        sync.RWMutex
	requests  map[Format]map[Format]RequestTransform
	responses map[Format]map[Format]ResponseTransform
}

// NewRegistry 创建一个空的翻译注册表实例。
func NewRegistry() *Registry {
	return &Registry{
		requests:  make(map[Format]map[Format]RequestTransform),
		responses: make(map[Format]map[Format]ResponseTransform),
	}
}

// Register 在两个格式之间注册请求和响应转换函数。
// 如果请求转换函数为 nil，则不注册请求转换；响应转换始终注册（可为 nil）。
func (r *Registry) Register(from, to Format, request RequestTransform, response ResponseTransform) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.requests[from]; !ok {
		r.requests[from] = make(map[Format]RequestTransform)
	}
	if request != nil {
		r.requests[from][to] = request
	}

	if _, ok := r.responses[from]; !ok {
		r.responses[from] = make(map[Format]ResponseTransform)
	}
	r.responses[from][to] = response
}

// TranslateRequest 在两个格式之间转换请求载荷。
// 如果没有注册转换函数，则返回原始载荷，但会将 model 字段更新为解析后的模型名称，
// 以避免客户端前缀（如 "copilot/gpt-5-mini"）泄漏到上游服务。
func (r *Registry) TranslateRequest(from, to Format, model string, rawJSON []byte, stream bool) []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if byTarget, ok := r.requests[from]; ok {
		if fn, isOk := byTarget[to]; isOk && fn != nil {
			return fn(model, rawJSON, stream)
		}
	}
	if model != "" && gjson.GetBytes(rawJSON, "model").String() != model {
		if updated, err := sjson.SetBytes(rawJSON, "model", model); err != nil {
			log.Warnf("translator: failed to normalize model in request fallback: %v", err)
		} else {
			return updated
		}
	}
	return rawJSON
}

// HasResponseTransformer 检查是否已注册指定格式对之间的响应转换器。
func (r *Registry) HasResponseTransformer(from, to Format) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if byTarget, ok := r.responses[from]; ok {
		if _, isOk := byTarget[to]; isOk {
			return true
		}
	}
	return false
}

// TranslateStream 应用已注册的流式响应转换函数。
// 如果没有注册转换函数，则将原始数据作为单个块返回。
func (r *Registry) TranslateStream(ctx context.Context, from, to Format, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if byTarget, ok := r.responses[to]; ok {
		if fn, isOk := byTarget[from]; isOk && fn.Stream != nil {
			return fn.Stream(ctx, model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
		}
	}
	return [][]byte{rawJSON}
}

// TranslateNonStream 应用已注册的非流式响应转换函数。
// 如果没有注册转换函数，则返回原始响应。
func (r *Registry) TranslateNonStream(ctx context.Context, from, to Format, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if byTarget, ok := r.responses[to]; ok {
		if fn, isOk := byTarget[from]; isOk && fn.NonStream != nil {
			return fn.NonStream(ctx, model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
		}
	}
	return rawJSON
}

// TranslateTokenCount 应用已注册的 token 计数转换函数。
// 如果没有注册转换函数，则返回原始响应。
func (r *Registry) TranslateTokenCount(ctx context.Context, from, to Format, count int64, rawJSON []byte) []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if byTarget, ok := r.responses[to]; ok {
		if fn, isOk := byTarget[from]; isOk && fn.TokenCount != nil {
			return fn.TokenCount(ctx, count)
		}
	}
	return rawJSON
}

// defaultRegistry 是包级别的默认全局注册表实例。
var defaultRegistry = NewRegistry()

// Default 返回用于共享的全局注册表实例。
func Default() *Registry {
	return defaultRegistry
}

// Register 将转换函数附加到默认全局注册表。
func Register(from, to Format, request RequestTransform, response ResponseTransform) {
	defaultRegistry.Register(from, to, request, response)
}

// TranslateRequest 是默认全局注册表的请求转换辅助函数。
func TranslateRequest(from, to Format, model string, rawJSON []byte, stream bool) []byte {
	return defaultRegistry.TranslateRequest(from, to, model, rawJSON, stream)
}

// HasResponseTransformer 检查默认全局注册表中是否存在指定格式对的响应转换器。
func HasResponseTransformer(from, to Format) bool {
	return defaultRegistry.HasResponseTransformer(from, to)
}

// TranslateStream 是默认全局注册表的流式响应转换辅助函数。
func TranslateStream(ctx context.Context, from, to Format, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	return defaultRegistry.TranslateStream(ctx, from, to, model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
}

// TranslateNonStream 是默认全局注册表的非流式响应转换辅助函数。
func TranslateNonStream(ctx context.Context, from, to Format, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
	return defaultRegistry.TranslateNonStream(ctx, from, to, model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
}

// TranslateTokenCount 是默认全局注册表的 token 计数转换辅助函数。
func TranslateTokenCount(ctx context.Context, from, to Format, count int64, rawJSON []byte) []byte {
	return defaultRegistry.TranslateTokenCount(ctx, from, to, count, rawJSON)
}
