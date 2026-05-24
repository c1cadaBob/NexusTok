// Package ali 实现阿里云通义千问渠道的文本对话请求转换逻辑。
// 该文件负责将 OpenAI 格式的请求参数适配为阿里云 DashScope API 的参数规范。
package ali

// 项目内部导入

// 参考文档: https://help.aliyun.com/document_detail/613695.html

// EnableSearchModelSuffix 定义启用互联网搜索功能的模型名称后缀。
// 在模型名称末尾添加 "-internet" 后缀可启用阿里云的互联网搜索增强功能。
const EnableSearchModelSuffix = "-internet"

// requestOpenAI2Ali 将 OpenAI 格式的通用请求转换为阿里云兼容的请求格式。
// 主要处理 TopP 参数的范围限制：阿里云要求 TopP 在 (0, 1) 开区间内，
// 因此将 >= 1 的值调整为 0.999，将 <= 0 的值调整为 0.001。
//
// 参数:
//   - request: OpenAI 格式的通用请求
//
// 返回值:
//   - *dto.GeneralOpenAIRequest: 调整后的请求（直接修改原请求并返回指针）
func requestOpenAI2Ali(request dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	topP := lo.FromPtrOr(request.TopP, 0)
	if topP >= 1 {
		request.TopP = lo.ToPtr(0.999)
	} else if topP <= 0 {
		request.TopP = lo.ToPtr(0.001)
	}
	return &request
}
