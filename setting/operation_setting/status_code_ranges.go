// status_code_ranges.go — HTTP 状态码范围配置与匹配
// 职责：管理基于 HTTP 状态码的自动渠道禁用和自动重试策略。
// 支持配置状态码范围（如 "401,403,500-599"），并提供高效的
// 范围解析、排序、合并和匹配功能。

package operation_setting

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/types"
)

// StatusCodeRange 表示一个闭合的 HTTP 状态码范围 [Start, End]
type StatusCodeRange struct {
	Start int
	End   int
}

// AutomaticDisableStatusCodeRanges 自动禁用渠道的状态码范围列表
// 当上游返回的状态码命中此范围时，对应渠道将被自动禁用
var AutomaticDisableStatusCodeRanges = []StatusCodeRange{{Start: 401, End: 401}}

// AutomaticRetryStatusCodeRanges 自动重试的状态码范围列表
// 默认行为与 controller/relay.go 中 shouldRetry 的硬编码逻辑一致：
//   - 重试: 1xx, 3xx, 4xx（除 400/408）, 5xx（除 504/524）
//   - 不重试: 2xx
var AutomaticRetryStatusCodeRanges = []StatusCodeRange{
	{Start: 100, End: 199},
	{Start: 300, End: 399},
	{Start: 401, End: 407},
	{Start: 409, End: 499},
	{Start: 500, End: 503},
	{Start: 505, End: 523},
	{Start: 525, End: 599},
}

// alwaysSkipRetryStatusCodes 始终跳过重试的 HTTP 状态码集合
var alwaysSkipRetryStatusCodes = map[int]struct{}{
	504: {},
	524: {},
}

// alwaysSkipRetryCodes 始终跳过重试的业务错误码集合
var alwaysSkipRetryCodes = map[types.ErrorCode]struct{}{
	types.ErrorCodeBadResponseBody: {},
}

// AutomaticDisableStatusCodesToString 将自动禁用状态码范围列表转为字符串
// 返回值：逗号分隔的状态码范围字符串，如 "401,500-599"
func AutomaticDisableStatusCodesToString() string {
	return statusCodeRangesToString(AutomaticDisableStatusCodeRanges)
}

// AutomaticDisableStatusCodesFromString 从字符串解析并更新自动禁用状态码范围
// 参数：
//   - s: 逗号分隔的状态码范围字符串
//
// 返回值：解析失败时返回错误
func AutomaticDisableStatusCodesFromString(s string) error {
	ranges, err := ParseHTTPStatusCodeRanges(s)
	if err != nil {
		return err
	}
	AutomaticDisableStatusCodeRanges = ranges
	return nil
}

// ShouldDisableByStatusCode 判断指定状态码是否应触发渠道自动禁用
// 参数：
//   - code: HTTP 状态码
//
// 返回值：如果应禁用则返回 true
func ShouldDisableByStatusCode(code int) bool {
	return shouldMatchStatusCodeRanges(AutomaticDisableStatusCodeRanges, code)
}

// AutomaticRetryStatusCodesToString 将自动重试状态码范围列表转为字符串
// 返回值：逗号分隔的状态码范围字符串
func AutomaticRetryStatusCodesToString() string {
	return statusCodeRangesToString(AutomaticRetryStatusCodeRanges)
}

// AutomaticRetryStatusCodesFromString 从字符串解析并更新自动重试状态码范围
// 参数：
//   - s: 逗号分隔的状态码范围字符串
//
// 返回值：解析失败时返回错误
func AutomaticRetryStatusCodesFromString(s string) error {
	ranges, err := ParseHTTPStatusCodeRanges(s)
	if err != nil {
		return err
	}
	AutomaticRetryStatusCodeRanges = ranges
	return nil
}

// IsAlwaysSkipRetryStatusCode 判断指定 HTTP 状态码是否始终跳过重试
// 参数：
//   - code: HTTP 状态码
//
// 返回值：如果始终跳过重试则返回 true
func IsAlwaysSkipRetryStatusCode(code int) bool {
	_, exists := alwaysSkipRetryStatusCodes[code]
	return exists
}

// IsAlwaysSkipRetryCode 判断指定业务错误码是否始终跳过重试
// 参数：
//   - errorCode: 业务错误码
//
// 返回值：如果始终跳过重试则返回 true
func IsAlwaysSkipRetryCode(errorCode types.ErrorCode) bool {
	_, exists := alwaysSkipRetryCodes[errorCode]
	return exists
}

// ShouldRetryByStatusCode 判断指定状态码是否应触发自动重试
// 先检查是否在始终跳过列表中，再检查是否在重试范围中
// 参数：
//   - code: HTTP 状态码
//
// 返回值：如果应重试则返回 true
func ShouldRetryByStatusCode(code int) bool {
	if IsAlwaysSkipRetryStatusCode(code) {
		return false
	}
	return shouldMatchStatusCodeRanges(AutomaticRetryStatusCodeRanges, code)
}

// statusCodeRangesToString 将状态码范围列表转为逗号分隔的字符串
// 参数：
//   - ranges: 状态码范围列表
//
// 返回值：格式如 "401,403,500-599" 的字符串
func statusCodeRangesToString(ranges []StatusCodeRange) string {
	if len(ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r.Start == r.End {
			// 单个状态码，直接输出数字
			parts = append(parts, strconv.Itoa(r.Start))
			continue
		}
		// 范围状态码，输出 "起始-结束" 格式
		parts = append(parts, fmt.Sprintf("%d-%d", r.Start, r.End))
	}
	return strings.Join(parts, ",")
}

// shouldMatchStatusCodeRanges 判断状态码是否命中已排序的范围列表
// 要求 ranges 已按 Start 升序排列
// 参数：
//   - ranges: 已排序的状态码范围列表
//   - code: 待匹配的 HTTP 状态码
//
// 返回值：如果命中则返回 true
func shouldMatchStatusCodeRanges(ranges []StatusCodeRange, code int) bool {
	// 状态码有效范围校验
	if code < 100 || code > 599 {
		return false
	}
	// 利用排序特性进行高效查找：ranges 已按 Start 升序排列
	for _, r := range ranges {
		if code < r.Start {
			// 当前范围的起始已大于 code，后续范围不可能匹配
			return false
		}
		if code <= r.End {
			return true
		}
	}
	return false
}

// ParseHTTPStatusCodeRanges 解析逗号分隔的状态码范围字符串
// 支持格式：
//   - 单个状态码: "401"
//   - 范围: "500-599"
//   - 混合: "401,403,500-599"
//   - 中文逗号会自动替换为英文逗号
//
// 解析后会自动排序并合并重叠/相邻的范围。
// 参数：
//   - input: 状态码范围字符串
//
// 返回值：
//   - []StatusCodeRange: 解析合并后的状态码范围列表
//   - error: 解析失败时返回错误
func ParseHTTPStatusCodeRanges(input string) ([]StatusCodeRange, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}

	// 自动处理中文逗号
	input = strings.NewReplacer("，", ",").Replace(input)
	segments := strings.Split(input, ",")

	var ranges []StatusCodeRange
	var invalid []string

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		r, err := parseHTTPStatusCodeToken(seg)
		if err != nil {
			invalid = append(invalid, seg)
			continue
		}
		ranges = append(ranges, r)
	}

	if len(invalid) > 0 {
		return nil, fmt.Errorf("invalid http status code rules: %s", strings.Join(invalid, ", "))
	}
	if len(ranges) == 0 {
		return nil, nil
	}

	// 按起始值升序排序，起始值相同时按结束值升序
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start == ranges[j].Start {
			return ranges[i].End < ranges[j].End
		}
		return ranges[i].Start < ranges[j].Start
	})

	// 合并重叠或相邻的范围（如 401-403 和 404 合并为 401-404）
	merged := []StatusCodeRange{ranges[0]}
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if r.Start <= last.End+1 {
			// 范围重叠或相邻，扩展当前范围
			if r.End > last.End {
				last.End = r.End
			}
			continue
		}
		// 不相邻，新增一个范围
		merged = append(merged, r)
	}

	return merged, nil
}

// parseHTTPStatusCodeToken 解析单个状态码或状态码范围 token
// 支持格式：
//   - 单个状态码: "401"
//   - 范围: "500-599"
//
// 参数：
//   - token: 单个状态码或范围字符串
//
// 返回值：
//   - StatusCodeRange: 解析后的状态码范围
//   - error: 解析失败时返回错误
func parseHTTPStatusCodeToken(token string) (StatusCodeRange, error) {
	token = strings.TrimSpace(token)
	token = strings.ReplaceAll(token, " ", "")
	if token == "" {
		return StatusCodeRange{}, fmt.Errorf("empty token")
	}

	// 处理范围格式（包含 "-" 分隔符）
	if strings.Contains(token, "-") {
		parts := strings.Split(token, "-")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return StatusCodeRange{}, fmt.Errorf("invalid range token: %s", token)
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return StatusCodeRange{}, fmt.Errorf("invalid range start: %s", token)
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return StatusCodeRange{}, fmt.Errorf("invalid range end: %s", token)
		}
		if start > end {
			return StatusCodeRange{}, fmt.Errorf("range start > end: %s", token)
		}
		// 校验状态码有效范围 [100, 599]
		if start < 100 || end > 599 {
			return StatusCodeRange{}, fmt.Errorf("range out of bounds: %s", token)
		}
		return StatusCodeRange{Start: start, End: end}, nil
	}

	// 处理单个状态码格式
	code, err := strconv.Atoi(token)
	if err != nil {
		return StatusCodeRange{}, fmt.Errorf("invalid status code: %s", token)
	}
	if code < 100 || code > 599 {
		return StatusCodeRange{}, fmt.Errorf("status code out of bounds: %s", token)
	}
	return StatusCodeRange{Start: code, End: code}, nil
}
