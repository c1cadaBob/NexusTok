// translator - pipeline.go
// 该文件实现了翻译管道（Pipeline），用于编排请求/响应的转换流程。
// 管道支持中间件模式，允许在实际转换前后插入自定义处理逻辑。

package translator

import "context"

// RequestEnvelope 表示翻译管道中的请求信封。
// 包含格式标识、模型名称、流式标志和原始请求体。
type RequestEnvelope struct {
	Format Format
	Model  string
	Stream bool
	Body   []byte
}

// ResponseEnvelope 表示翻译管道中的响应信封。
// 包含格式标识、模型名称、流式标志、响应体和流式数据块。
type ResponseEnvelope struct {
	Format Format
	Model  string
	Stream bool
	Body   []byte
	Chunks [][]byte
}

// RequestMiddleware 是装饰请求转换的中间件函数类型。
// 中间件可以在调用 next 处理器之前和之后执行自定义逻辑。
type RequestMiddleware func(ctx context.Context, req RequestEnvelope, next RequestHandler) (RequestEnvelope, error)

// ResponseMiddleware 是装饰响应转换的中间件函数类型。
type ResponseMiddleware func(ctx context.Context, resp ResponseEnvelope, next ResponseHandler) (ResponseEnvelope, error)

// RequestHandler 是执行请求格式间转换的处理器函数类型。
type RequestHandler func(ctx context.Context, req RequestEnvelope) (RequestEnvelope, error)

// ResponseHandler 是执行响应格式间转换的处理器函数类型。
type ResponseHandler func(ctx context.Context, resp ResponseEnvelope) (ResponseEnvelope, error)

// Pipeline 编排请求/响应的转换流程，支持中间件链式处理。
// 使用注册表（Registry）执行实际的格式转换，中间件在转换前后插入自定义逻辑。
type Pipeline struct {
	registry           *Registry
	requestMiddleware  []RequestMiddleware
	responseMiddleware []ResponseMiddleware
}

// NewPipeline 创建一个绑定到指定注册表的翻译管道。
// 如果注册表为 nil，则使用默认全局注册表。
func NewPipeline(registry *Registry) *Pipeline {
	if registry == nil {
		registry = Default()
	}
	return &Pipeline{registry: registry}
}

// UseRequest 添加按注册顺序执行的请求中间件。
func (p *Pipeline) UseRequest(mw RequestMiddleware) {
	if mw != nil {
		p.requestMiddleware = append(p.requestMiddleware, mw)
	}
}

// UseResponse 添加按注册顺序执行的响应中间件。
func (p *Pipeline) UseResponse(mw ResponseMiddleware) {
	if mw != nil {
		p.responseMiddleware = append(p.responseMiddleware, mw)
	}
}

// TranslateRequest 应用请求中间件链和注册表转换，将请求从一个格式转换为另一个格式。
func (p *Pipeline) TranslateRequest(ctx context.Context, from, to Format, req RequestEnvelope) (RequestEnvelope, error) {
	terminal := func(ctx context.Context, input RequestEnvelope) (RequestEnvelope, error) {
		translated := p.registry.TranslateRequest(from, to, input.Model, input.Body, input.Stream)
		input.Body = translated
		input.Format = to
		return input, nil
	}

	handler := terminal
	for i := len(p.requestMiddleware) - 1; i >= 0; i-- {
		mw := p.requestMiddleware[i]
		next := handler
		handler = func(ctx context.Context, r RequestEnvelope) (RequestEnvelope, error) {
			return mw(ctx, r, next)
		}
	}

	return handler(ctx, req)
}

// TranslateResponse 应用响应中间件链和注册表转换，将响应从一个格式转换为另一个格式。
// 根据响应是否为流式，分别调用 TranslateStream 或 TranslateNonStream。
func (p *Pipeline) TranslateResponse(ctx context.Context, from, to Format, resp ResponseEnvelope, originalReq, translatedReq []byte, param *any) (ResponseEnvelope, error) {
	terminal := func(ctx context.Context, input ResponseEnvelope) (ResponseEnvelope, error) {
		if input.Stream {
			input.Chunks = p.registry.TranslateStream(ctx, from, to, input.Model, originalReq, translatedReq, input.Body, param)
		} else {
			input.Body = p.registry.TranslateNonStream(ctx, from, to, input.Model, originalReq, translatedReq, input.Body, param)
		}
		input.Format = to
		return input, nil
	}

	handler := terminal
	for i := len(p.responseMiddleware) - 1; i >= 0; i-- {
		mw := p.responseMiddleware[i]
		next := handler
		handler = func(ctx context.Context, r ResponseEnvelope) (ResponseEnvelope, error) {
			return mw(ctx, r, next)
		}
	}

	return handler(ctx, resp)
}
