// 包 interfaces 提供用于翻译器函数向后兼容的类型别名。
// 该文件定义了 CLI Proxy API 中用于请求和响应转换操作的通用接口类型，
// 保持与 SDK translator 包的兼容性。
package interfaces

import sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"

// 以下是翻译器函数类型的向后兼容别名。

// TranslateRequestFunc 是请求转换函数的类型别名
type TranslateRequestFunc = sdktranslator.RequestTransform

// TranslateResponseFunc 是响应流转换函数的类型别名
type TranslateResponseFunc = sdktranslator.ResponseStreamTransform

// TranslateResponseNonStreamFunc 是非流式响应转换函数的类型别名
type TranslateResponseNonStreamFunc = sdktranslator.ResponseNonStreamTransform

// TranslateResponse 是响应转换的类型别名
type TranslateResponse = sdktranslator.ResponseTransform
