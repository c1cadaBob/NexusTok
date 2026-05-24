// Package replicate 的数据传输对象（DTO）定义文件。
// 定义了 Replicate API 请求和响应的结构体。
package replicate

// PredictionResponse 是 Replicate 预测（Prediction）API 的响应结构体。
// 包含预测状态、输出结果和可能的错误信息。
type PredictionResponse struct {
	Status string           `json:"status"` // 预测状态，如 "succeeded"、"processing" 等
	Output any              `json:"output"` // 预测输出，类型不确定，可能是字符串或字符串数组
	Error  *PredictionError `json:"error"`  // 预测错误信息，成功时为 nil
}

// PredictionError 表示 Replicate 预测请求中的错误信息。
type PredictionError struct {
	Code    string `json:"code"`    // 错误代码
	Message string `json:"message"` // 错误消息
	Detail  string `json:"detail"`  // 错误详情
}

// FileUploadResponse 是 Replicate 文件上传 API 的响应结构体。
// 上传成功后返回文件的访问 URL。
type FileUploadResponse struct {
	Urls struct {
		Get string `json:"get"` // 上传文件的 GET 访问 URL
	} `json:"urls"`
}
