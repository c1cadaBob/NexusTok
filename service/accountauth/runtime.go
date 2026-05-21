package accountauth

import (
	"fmt"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
)

const (
	recentRequestBucketSeconds int64 = 10 * 60
	recentRequestBucketCount         = 20
)

type recentRequestState struct {
	Buckets []recentRequestStoredBucket `json:"buckets"`
}

type recentRequestStoredBucket struct {
	BucketID int64 `json:"bucket_id"`
	Success  int64 `json:"success"`
	Failed   int64 `json:"failed"`
}

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

func ParseQuotaState(raw string) QuotaState {
	var quota QuotaState
	if strings.TrimSpace(raw) == "" {
		return quota
	}
	_ = common.UnmarshalJsonStr(raw, &quota)
	return quota
}

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

func RecordRecentRequest(raw string, now time.Time, success bool) string {
	state := decodeRecentRequestState(raw)
	bucketID := recentRequestBucketID(now)
	found := false
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
	if !found {
		bucket := recentRequestStoredBucket{BucketID: bucketID}
		if success {
			bucket.Success = 1
		} else {
			bucket.Failed = 1
		}
		state.Buckets = append(state.Buckets, bucket)
	}
	minBucketID := bucketID - int64(recentRequestBucketCount) + 1
	kept := state.Buckets[:0]
	for _, bucket := range state.Buckets {
		if bucket.BucketID >= minBucketID {
			kept = append(kept, bucket)
		}
	}
	state.Buckets = kept
	data, err := common.Marshal(state)
	if err != nil {
		return raw
	}
	return string(data)
}

func RecentRequestsSnapshot(raw string, now time.Time) []RecentRequestBucket {
	state := decodeRecentRequestState(raw)
	byID := map[int64]recentRequestStoredBucket{}
	for _, bucket := range state.Buckets {
		byID[bucket.BucketID] = bucket
	}
	currentBucketID := recentRequestBucketID(now)
	out := make([]RecentRequestBucket, 0, recentRequestBucketCount)
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

func accountRuntimeStatus(account *model.PoolAccount) string {
	if account.Status != common.ChannelStatusEnabled || !account.Schedulable {
		return StatusDisabled
	}
	now := common.GetTimestamp()
	if account.IsCoolingDown(now) {
		return StatusCooling
	}
	if account.Unavailable {
		return StatusError
	}
	return StatusReady
}

func parseProviderError(raw string) *ProviderError {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var err ProviderError
	if decodeErr := common.UnmarshalJsonStr(raw, &err); decodeErr == nil && (err.Message != "" || err.Code != "" || err.HTTPStatus != 0) {
		return &err
	}
	return &ProviderError{Message: raw}
}

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

func recentRequestBucketID(now time.Time) int64 {
	if now.IsZero() {
		return 0
	}
	return now.Unix() / recentRequestBucketSeconds
}

func formatRecentRequestBucketLabel(bucketID int64) string {
	start := time.Unix(bucketID*recentRequestBucketSeconds, 0).In(time.Local)
	end := start.Add(time.Duration(recentRequestBucketSeconds) * time.Second)
	return fmt.Sprintf("%s-%s", start.Format("15:04"), end.Format("15:04"))
}
