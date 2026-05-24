// runtime.go 提供账号池账号的运行时状态管理和数据序列化功能。
// 包括账号状态判断、配额解析、最近请求统计（滑动窗口）、凭证元数据解析等。
package accountauth

import (
	"fmt"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common" // 公共工具：JSON、时间戳等
	"github.com/c1cada/NexusTok/model"   // 数据模型：PoolAccount 等
)

// 最近请求统计的滑动窗口配置
const (
	recentRequestBucketSeconds int64 = 10 * 60 // 每个时间桶的跨度：10 分钟
	recentRequestBucketCount         = 20      // 保留的时间桶数量：20 个（共 200 分钟）
)

// recentRequestState 表示最近请求统计的完整状态
type recentRequestState struct {
	Buckets []recentRequestStoredBucket `json:"buckets"` // 按时间桶分组的统计数据
}

// recentRequestStoredBucket 表示单个时间桶的请求统计
type recentRequestStoredBucket struct {
	BucketID int64 `json:"bucket_id"` // 时间桶 ID（基于 Unix 时间戳 / 桶跨度）
	Success  int64 `json:"success"`   // 成功请求数
	Failed   int64 `json:"failed"`    // 失败请求数
}

// MetadataToJSON 将元数据 map 序列化为 JSON 字符串。
// 空 map 返回空字符串。
//
// 参数：
//   - metadata: 元数据键值对
//
// 返回：
//   - string: JSON 字符串，或空字符串
func MetadataToJSON(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	data, err := common.Marshal(metadata)
	if err != nil {
		return ""
	}
	return string(data)
}

// AttributesToJSON 将属性 map 序列化为 JSON 字符串。
// 空 map 返回空字符串。
//
// 参数：
//   - attributes: 属性键值对
//
// 返回：
//   - string: JSON 字符串，或空字符串
func AttributesToJSON(attributes map[string]string) string {
	if len(attributes) == 0 {
		return ""
	}
	data, err := common.Marshal(attributes)
	if err != nil {
		return ""
	}
	return string(data)
}

// ParseMetadata 从 JSON 字符串解析元数据 map。
// 解析失败时返回空 map。
//
// 参数：
//   - raw: JSON 字符串
//
// 返回：
//   - map[string]any: 解析后的元数据
func ParseMetadata(raw string) map[string]any {
	result := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	if err := common.UnmarshalJsonStr(raw, &result); err != nil {
		return map[string]any{}
	}
	return result
}

// ParseAttributes 从 JSON 字符串解析属性 map。
// 解析失败时返回空 map。
//
// 参数：
//   - raw: JSON 字符串
//
// 返回：
//   - map[string]string: 解析后的属性
func ParseAttributes(raw string) map[string]string {
	result := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	if err := common.UnmarshalJsonStr(raw, &result); err != nil {
		return map[string]string{}
	}
	return result
}

// ParseQuotaState 从 JSON 字符串解析配额状态。
//
// 参数：
//   - raw: JSON 字符串
//
// 返回：
//   - QuotaState: 解析后的配额状态
func ParseQuotaState(raw string) QuotaState {
	var quota QuotaState
	if strings.TrimSpace(raw) == "" {
		return quota
	}
	_ = common.UnmarshalJsonStr(raw, &quota)
	return quota
}

// ParseModelStates 从 JSON 字符串解析模型状态 map。
// 解析失败时返回空 map。
//
// 参数：
//   - raw: JSON 字符串
//
// 返回：
//   - map[string]*ModelState: 模型名 -> 模型状态
func ParseModelStates(raw string) map[string]*ModelState {
	result := map[string]*ModelState{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	if err := common.UnmarshalJsonStr(raw, &result); err != nil {
		return map[string]*ModelState{}
	}
	return result
}

// RuntimeView 从数据库模型 PoolAccount 构建账号的运行时视图。
// 汇总了账号状态、配额、模型状态、最近请求统计、错误信息等。
//
// 参数：
//   - account: 数据库中的账号池账号对象
//
// 返回：
//   - *AccountRuntimeView: 运行时视图对象，nil 表示输入为空
func RuntimeView(account *model.PoolAccount) *AccountRuntimeView {
	if account == nil {
		return nil
	}
	return &AccountRuntimeView{
		Status:             accountRuntimeStatus(account),
		StatusMessage:      account.StatusMessage,
		Unavailable:        account.Unavailable,
		Quota:              ParseQuotaState(account.QuotaSnapshot),
		ModelStates:        ParseModelStates(account.ModelStates),
		RecentRequests:     RecentRequestsSnapshot(account.RecentRequests, time.Now()),
		LastError:          parseProviderError(account.LastError),
		LastRefreshedTime:  account.LastRefreshedTime,
		NextRefreshTime:    account.NextRefreshTime,
		NextRetryTime:      account.NextRetryTime,
		SuccessCount:       account.SuccessCount,
		FailedCount:        account.FailedCount,
		CredentialMetadata: ParseMetadata(account.CredentialMetadata),
		CredentialAttrs:    ParseAttributes(account.CredentialAttrs),
	}
}

// RecordRecentRequest 记录一次请求到最近请求统计中。
// 使用滑动窗口机制，将请求归入对应的时间桶，同时清理过期的桶。
//
// 参数：
//   - raw: 现有的最近请求 JSON 字符串
//   - now: 当前时间
//   - success: 请求是否成功
//
// 返回：
//   - string: 更新后的 JSON 字符串
func RecordRecentRequest(raw string, now time.Time, success bool) string {
	state := decodeRecentRequestState(raw)
	// 计算当前时间所属的桶 ID
	bucketID := recentRequestBucketID(now)
	found := false
	// 查找并更新已有的桶
	for i := range state.Buckets {
		if state.Buckets[i].BucketID != bucketID {
			continue
		}
		if success {
			state.Buckets[i].Success++
		} else {
			state.Buckets[i].Failed++
		}
		found = true
		break
	}
	// 未找到对应桶时创建新桶
	if !found {
		bucket := recentRequestStoredBucket{BucketID: bucketID}
		if success {
			bucket.Success = 1
		} else {
			bucket.Failed = 1
		}
		state.Buckets = append(state.Buckets, bucket)
	}
	// 清理过期的桶（保留最近 N 个桶）
	minBucketID := bucketID - int64(recentRequestBucketCount) + 1
	kept := state.Buckets[:0] // 原地过滤，避免额外内存分配
	for _, bucket := range state.Buckets {
		if bucket.BucketID >= minBucketID {
			kept = append(kept, bucket)
		}
	}
	state.Buckets = kept
	// 序列化为 JSON
	data, err := common.Marshal(state)
	if err != nil {
		return raw
	}
	return string(data)
}

// RecentRequestsSnapshot 生成最近请求统计的快照视图。
// 按时间顺序返回最近 N 个时间桶的统计数据，用于界面展示。
//
// 参数：
//   - raw: 存储的最近请求 JSON 字符串
//   - now: 当前时间
//
// 返回：
//   - []RecentRequestBucket: 按时间正序排列的桶统计列表
func RecentRequestsSnapshot(raw string, now time.Time) []RecentRequestBucket {
	state := decodeRecentRequestState(raw)
	// 建立桶 ID 到桶数据的映射
	byID := map[int64]recentRequestStoredBucket{}
	for _, bucket := range state.Buckets {
		byID[bucket.BucketID] = bucket
	}
	currentBucketID := recentRequestBucketID(now)
	out := make([]RecentRequestBucket, 0, recentRequestBucketCount)
	// 从旧到新填充所有时间桶（缺失的桶显示为 0）
	for i := recentRequestBucketCount - 1; i >= 0; i-- {
		bucketID := currentBucketID - int64(i)
		bucket := byID[bucketID]
		out = append(out, RecentRequestBucket{
			Time:    formatRecentRequestBucketLabel(bucketID),
			Success: bucket.Success,
			Failed:  bucket.Failed,
		})
	}
	return out
}

// accountRuntimeStatus 判断账号的运行时状态。
// 优先级：disabled > cooling > error > ready
//
// 参数：
//   - account: 账号池账号对象
//
// 返回：
//   - string: 状态标识（StatusDisabled/StatusCooling/StatusError/StatusReady）
func accountRuntimeStatus(account *model.PoolAccount) string {
	// 账号未启用或不可调度时为 disabled
	if account.Status != common.ChannelStatusEnabled || !account.Schedulable {
		return StatusDisabled
	}
	now := common.GetTimestamp()
	// 账号正在冷却期时为 cooling
	if account.IsCoolingDown(now) {
		return StatusCooling
	}
	// 账号标记为不可用时为 error
	if account.Unavailable {
		return StatusError
	}
	return StatusReady
}

// parseProviderError 从 JSON 字符串解析上游提供者错误信息。
// 如果 JSON 解析失败，将原始字符串作为错误消息返回。
//
// 参数：
//   - raw: JSON 字符串或纯文本错误消息
//
// 返回：
//   - *ProviderError: 解析后的错误对象，空输入返回 nil
func parseProviderError(raw string) *ProviderError {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var err ProviderError
	if decodeErr := common.UnmarshalJsonStr(raw, &err); decodeErr == nil && (err.Message != "" || err.Code != "" || err.HTTPStatus != 0) {
		return &err
	}
	// JSON 解析失败时，将原始文本作为错误消息
	return &ProviderError{Message: raw}
}

// decodeRecentRequestState 从 JSON 字符串解码最近请求状态。
// 解析失败时返回空状态。
func decodeRecentRequestState(raw string) recentRequestState {
	state := recentRequestState{}
	if strings.TrimSpace(raw) == "" {
		return state
	}
	if err := common.UnmarshalJsonStr(raw, &state); err != nil {
		return recentRequestState{}
	}
	return state
}

// recentRequestBucketID 根据时间计算所属的桶 ID。
// 桶 ID = Unix 时间戳 / 桶跨度（秒），零时间返回 0。
func recentRequestBucketID(now time.Time) int64 {
	if now.IsZero() {
		return 0
	}
	return now.Unix() / recentRequestBucketSeconds
}

// formatRecentRequestBucketLabel 将桶 ID 格式化为可读的时间范围标签。
// 格式为 "HH:MM-HH:MM"，如 "14:20-14:30"。
func formatRecentRequestBucketLabel(bucketID int64) string {
	start := time.Unix(bucketID*recentRequestBucketSeconds, 0).In(time.Local)
	end := start.Add(time.Duration(recentRequestBucketSeconds) * time.Second)
	return fmt.Sprintf("%s-%s", start.Format("15:04"), end.Format("15:04"))
}
