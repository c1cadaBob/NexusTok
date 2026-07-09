package common

import (
	"io"

	rootcommon "github.com/c1cada/NexusTok/common"
)

// NewOutboundJSONBody 将已经序列化完成的上游 JSON 请求体包装为 BodyStorage。
//
// 这样做的主要目的有两个：
//  1. 当请求中包含较大的 base64 图片、音频或 Responses 输入时，BodyStorage 可以按全局阈值
//     自动选择内存或临时文件，降低等待上游响应期间的堆内存驻留。
//  2. 返回的 reader 通过 ReaderOnly 隐藏 io.Closer，避免 net/http 在请求生命周期中提前关闭
//     底层 BodyStorage；调用方必须在上游请求完成后关闭返回的 closer。
//
// 返回的 size 需要写入 RelayInfo.UpstreamRequestBodySize，由通用请求构造层回填
// http.Request.ContentLength。ReaderOnly 会抹掉 bytes.Reader/Buffer 等具体类型信息，
// 若不手动设置，部分上游会收到 chunked 请求并拒绝处理。
func NewOutboundJSONBody(data []byte) (body io.Reader, size int64, closer io.Closer, err error) {
	storage, err := rootcommon.CreateBodyStorage(data)
	if err != nil {
		return nil, 0, nil, err
	}
	return rootcommon.ReaderOnly(storage), storage.Size(), storage, nil
}
