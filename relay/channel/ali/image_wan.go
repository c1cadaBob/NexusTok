// Package ali 实现阿里云通义千问渠道的图像生成处理。
// 该文件负责将 OpenAI 格式的图像请求转换为阿里云 WanX（万象）图像模型的请求格式。
package ali

// 标准库导入

// 第三方库导入

// 项目内部导入

// oaiFormEdit2WanxImageEdit 将 OpenAI 格式的图像编辑请求转换为阿里云 WanX 万象图像编辑请求。
// 从 gin.Context 中解析表单数据和 Base64 编码的图像，构建阿里云图像生成 API 请求体。
//
// 参数:
//   - c: gin 请求上下文，用于读取表单数据和请求体
//   - info: 中继信息，包含价格计算等元数据
//   - request: OpenAI 格式的图像请求
//
// 返回值:
//   - *AliImageRequest: 转换后的阿里云图像请求
//   - error: 转换过程中的错误
func oaiFormEdit2WanxImageEdit(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (*AliImageRequest, error) {
	var err error
	var imageRequest AliImageRequest
	imageRequest.Model = request.Model
	imageRequest.ResponseFormat = request.ResponseFormat
	wanInput := WanImageInput{
		Prompt: request.Prompt,
	}

	if err := common.UnmarshalBodyReusable(c, &wanInput); err != nil {
		return nil, err
	}
	if wanInput.Images, err = getImageBase64sFromForm(c, "image"); err != nil {
		return nil, fmt.Errorf("get image base64s from form failed: %w", err)
	}
	//wanParams := WanImageParameters{
	//	N: int(request.N),
	//}
	imageRequest.Input = wanInput
	imageRequest.Parameters = AliImageParameters{
		N: int(lo.FromPtrOr(request.N, uint(1))),
	}
	info.PriceData.AddOtherRatio("n", float64(imageRequest.Parameters.N))

	return &imageRequest, nil
}

// isOldWanModel 判断模型名称是否为旧版 Wan 模型（排除 wan2.6 和 wan2.7 及更高版本）。
// 旧版 Wan 模型的请求格式可能与新版不同，需要区别处理。
//
// 参数:
//   - modelName: 模型名称字符串
//
// 返回值:
//   - bool: 如果是旧版 Wan 模型则返回 true
func isOldWanModel(modelName string) bool {
	return strings.Contains(modelName, "wan") &&
		!lo.SomeBy([]string{"wan2.6", "wan2.7"}, func(v string) bool { return strings.Contains(modelName, v) })
}

// isWanModel 判断模型名称是否为 Wan（万象）系列图像模型。
// 通过检查模型名称中是否包含 "wan" 关键字来判断。
//
// 参数:
//   - modelName: 模型名称字符串
//
// 返回值:
//   - bool: 如果是 Wan 系列模型则返回 true
func isWanModel(modelName string) bool {
	return strings.Contains(modelName, "wan")
}
