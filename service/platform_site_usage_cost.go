package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/constant"
	"github.com/c1cadaBob/NexusTok/model"

	"github.com/gin-gonic/gin"
)

const (
	platformSiteUsageCostWindowSeconds      = int64(5 * 60)
	platformSiteUsageCostRetrySeconds       = int64(5 * 60)
	platformSiteUsageCostMaxRetries         = 3
	platformSiteUsageCostPageSize           = 100
	platformSiteUsageCostMaxPages           = 1000
	platformSiteUsageCostBatchWindows       = 24
	platformSiteUsageCostLookupTimeout      = 8 * time.Second
	platformSiteUsageCostMatchWindowSeconds = int64(3)
	platformSiteUsageCostMatchWindow        = 3 * time.Second
)

type platformSiteUsageCostRecord struct {
	requestID  string
	model      string
	modelNames []string
	createdAt  time.Time
	cost       int64
}

type PlatformSiteUsageCostTaskResult struct {
	WindowsProcessed int `json:"windows_processed"`
	UpstreamCosts    int `json:"upstream_costs"`
	EstimatedCosts   int `json:"estimated_costs"`
	RetryWindows     int `json:"retry_windows"`
	UpstreamFailures int `json:"upstream_failures"`
}

type PlatformSiteUsageCostRequest struct {
	RequestID           string
	ChannelID           int
	UpstreamRequestID   string
	ModelName           string
	CreatedAt           time.Time
	Quota               int
	Other               *model.LogOther
	IsError             bool
	CarriedCost         *int64
	CarriedCostPlatform string
}

type platformSiteUsageCostState struct {
	errorHandled   bool
	queryAttempted bool
}

type platformSiteCarriedCost struct {
	Platform string
	Cost     int64
}

// SetPlatformSiteResponseCost allows a platform-specific response parser to
// attach a verified cost without making generic usage.cost values authoritative.
func SetPlatformSiteResponseCost(c *gin.Context, platform string, cost int64) {
	if c == nil || (platform != model.PlatformSub2API && platform != model.PlatformNewAPI) ||
		cost < 0 {
		return
	}
	common.SetContextKey(c, constant.ContextKeyPlatformSiteUsageCost, platformSiteCarriedCost{
		Platform: platform,
		Cost:     cost,
	})
}

func ResolvePlatformSiteUsageCost(ctx *gin.Context, request PlatformSiteUsageCostRequest) error {
	if ctx == nil || request.ChannelID <= 0 {
		return nil
	}
	if request.RequestID == "" {
		request.RequestID = ctx.GetString(common.RequestIdKey)
		if request.RequestID == "" {
			request.RequestID = common.NewRequestId()
			ctx.Set(common.RequestIdKey, request.RequestID)
		}
	}
	if request.UpstreamRequestID == "" {
		request.UpstreamRequestID = ctx.GetString(common.UpstreamRequestIdKey)
	}
	if request.CreatedAt.IsZero() {
		request.CreatedAt = common.GetContextKeyTime(ctx, constant.ContextKeyRequestStartTime)
		if request.CreatedAt.IsZero() {
			request.CreatedAt = time.Now().UTC()
		}
	}

	account, err := model.GetPlatformSiteUsageCostAccount(request.ChannelID)
	if err != nil {
		return err
	}
	if account == nil ||
		(account.Platform != model.PlatformSub2API && account.Platform != model.PlatformNewAPI) {
		return nil
	}

	state, _ := common.GetContextKeyType[*platformSiteUsageCostState](
		ctx,
		constant.ContextKeyPlatformSiteUsageCostState,
	)
	if state == nil {
		state = &platformSiteUsageCostState{}
		common.SetContextKey(ctx, constant.ContextKeyPlatformSiteUsageCostState, state)
	}
	if request.IsError {
		if state.errorHandled {
			return nil
		}
		state.errorHandled = true
		return model.UpsertPlatformSiteUsageCost(model.PlatformSiteUsageCost{
			RequestID:         request.RequestID,
			ChannelID:         request.ChannelID,
			UpstreamRequestID: request.UpstreamRequestID,
			Cost:              0,
			Source:            model.PlatformSiteUsageCostSourceEstimated,
			UpdatedAt:         request.CreatedAt.Unix(),
		})
	}
	existing, err := model.FindPlatformSiteUsageCost(request.RequestID)
	if err != nil {
		return err
	}
	if existing != nil && existing.Source == model.PlatformSiteUsageCostSourceUpstream {
		return nil
	}

	if request.CarriedCost != nil &&
		request.CarriedCostPlatform == account.Platform &&
		*request.CarriedCost >= 0 {
		return model.UpsertPlatformSiteUsageCost(model.PlatformSiteUsageCost{
			RequestID:         request.RequestID,
			ChannelID:         request.ChannelID,
			UpstreamRequestID: request.UpstreamRequestID,
			Cost:              *request.CarriedCost,
			Source:            model.PlatformSiteUsageCostSourceUpstream,
			UpdatedAt:         request.CreatedAt.Unix(),
		})
	}
	if carriedCost, ok := common.GetContextKeyType[platformSiteCarriedCost](
		ctx,
		constant.ContextKeyPlatformSiteUsageCost,
	); ok && carriedCost.Platform == account.Platform && carriedCost.Cost >= 0 {
		return model.UpsertPlatformSiteUsageCost(model.PlatformSiteUsageCost{
			RequestID:         request.RequestID,
			ChannelID:         request.ChannelID,
			UpstreamRequestID: request.UpstreamRequestID,
			Cost:              carriedCost.Cost,
			Source:            model.PlatformSiteUsageCostSourceUpstream,
			UpdatedAt:         request.CreatedAt.Unix(),
		})
	}

	var record *platformSiteUsageCostRecord
	if !state.queryAttempted {
		state.queryAttempted = true
		lookupCtx, cancel := context.WithTimeout(ctx, platformSiteUsageCostLookupTimeout)
		records, lookupErr := fetchPlatformSiteUsageCostRecordsOnce(lookupCtx, account, request.CreatedAt, request.UpstreamRequestID, request.ModelName)
		cancel()
		if lookupErr == nil {
			record = matchPlatformSiteUsageCostRequest(request, records)
		} else {
			common.SysError(fmt.Sprintf(
				"平台站点请求成本查询失败: channel_id=%d platform=%s reason=%s",
				request.ChannelID,
				account.Platform,
				SafePlatformSiteError(lookupErr),
			))
		}
	}

	cost := int64(0)
	source := model.PlatformSiteUsageCostSourceEstimated
	upstreamRequestID := request.UpstreamRequestID
	if record != nil {
		cost = record.cost
		source = model.PlatformSiteUsageCostSourceUpstream
		upstreamRequestID = record.requestID
	} else {
		cost, _ = estimatePlatformSiteUsageCost(model.Log{
			RequestId:         request.RequestID,
			ChannelId:         request.ChannelID,
			UpstreamRequestId: request.UpstreamRequestID,
			ModelName:         request.ModelName,
			CreatedAt:         request.CreatedAt.Unix(),
			Quota:             request.Quota,
			Other:             request.Other.JSONString(),
		})
	}
	return model.UpsertPlatformSiteUsageCost(model.PlatformSiteUsageCost{
		RequestID:         request.RequestID,
		ChannelID:         request.ChannelID,
		UpstreamRequestID: upstreamRequestID,
		Cost:              cost,
		Source:            source,
		UpdatedAt:         request.CreatedAt.Unix(),
	})
}

func ResolvePlatformSiteUsageCostForLog(
	ctx *gin.Context,
	channelID int,
	modelName string,
	quota int,
	other *model.LogOther,
	isError bool,
) {
	if ctx == nil {
		return
	}
	if err := ResolvePlatformSiteUsageCost(ctx, PlatformSiteUsageCostRequest{
		RequestID:         ctx.GetString(common.RequestIdKey),
		ChannelID:         channelID,
		UpstreamRequestID: ctx.GetString(common.UpstreamRequestIdKey),
		ModelName:         modelName,
		Quota:             quota,
		Other:             other,
		IsError:           isError,
	}); err != nil {
		common.SysError(fmt.Sprintf("平台站点请求成本记录失败: channel_id=%d", channelID))
	}
}

func PlatformSiteUsageCostWindowStart(timestamp int64) int64 {
	windowStart := timestamp / platformSiteUsageCostWindowSeconds * platformSiteUsageCostWindowSeconds
	if timestamp < 0 && timestamp%platformSiteUsageCostWindowSeconds != 0 {
		windowStart -= platformSiteUsageCostWindowSeconds
	}
	return windowStart
}

func SyncPlatformSiteUsageCosts(ctx context.Context, now time.Time) (PlatformSiteUsageCostTaskResult, error) {
	var result PlatformSiteUsageCostTaskResult
	accounts, err := model.ListPlatformSiteUsageCostAccounts()
	if err != nil {
		return result, err
	}
	if len(accounts) == 0 {
		return result, nil
	}

	targetWindowStart := PlatformSiteUsageCostWindowStart(now.UTC().Unix()) - platformSiteUsageCostWindowSeconds
	for i := range accounts {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		account := &accounts[i]
		dueWindows, err := model.ListDuePlatformSiteUsageCostWindows(
			account.ChannelID,
			now.Unix(),
			platformSiteUsageCostBatchWindows,
		)
		if err != nil {
			return result, err
		}
		for j := range dueWindows {
			processed, upstreamCount, estimateCount, failed, err := syncPlatformSiteUsageCostWindow(
				ctx,
				account,
				&dueWindows[j],
				now.Unix(),
			)
			if err != nil {
				return result, err
			}
			if processed {
				result.WindowsProcessed++
				result.UpstreamCosts += upstreamCount
				result.EstimatedCosts += estimateCount
				result.UpstreamFailures += boolInt(failed)
			}
			saved, err := model.GetPlatformSiteUsageCostWindow(
				account.ChannelID,
				dueWindows[j].WindowStart,
			)
			if err != nil {
				return result, err
			}
			if saved != nil && saved.Status == model.PlatformSiteUsageCostWindowRetry {
				result.RetryWindows++
			}
		}

		latest, err := model.GetLatestPlatformSiteUsageCostWindow(account.ChannelID)
		if err != nil {
			return result, err
		}
		nextWindowStart := targetWindowStart
		if latest != nil {
			nextWindowStart = latest.WindowStart + platformSiteUsageCostWindowSeconds
		}
		for count := 0; count < platformSiteUsageCostBatchWindows && nextWindowStart <= targetWindowStart; count++ {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			processed, upstreamCount, estimateCount, failed, err := syncPlatformSiteUsageCostWindow(
				ctx,
				account,
				&model.PlatformSiteUsageCostWindow{
					ChannelID:   account.ChannelID,
					WindowStart: nextWindowStart,
				},
				now.Unix(),
			)
			if err != nil {
				return result, err
			}
			if processed {
				result.WindowsProcessed++
				result.UpstreamCosts += upstreamCount
				result.EstimatedCosts += estimateCount
				result.UpstreamFailures += boolInt(failed)
			}
			saved, err := model.GetPlatformSiteUsageCostWindow(account.ChannelID, nextWindowStart)
			if err != nil {
				return result, err
			}
			if saved != nil && saved.Status == model.PlatformSiteUsageCostWindowRetry {
				result.RetryWindows++
			}
			nextWindowStart += platformSiteUsageCostWindowSeconds
		}
	}
	return result, nil
}

func syncPlatformSiteUsageCostWindow(
	ctx context.Context,
	account *model.PlatformSiteAccount,
	window *model.PlatformSiteUsageCostWindow,
	now int64,
) (bool, int, int, bool, error) {
	if account == nil || window == nil {
		return false, 0, 0, false, errors.New("平台站点成本窗口参数无效")
	}
	existing, err := model.GetPlatformSiteUsageCostWindow(account.ChannelID, window.WindowStart)
	if err != nil {
		return false, 0, 0, false, err
	}
	if existing != nil {
		window = existing
		if window.Status == model.PlatformSiteUsageCostWindowComplete ||
			(window.NextRetryAt > 0 && window.NextRetryAt > now) {
			return false, 0, 0, false, nil
		}
	}

	start := window.WindowStart
	end := start + platformSiteUsageCostWindowSeconds
	localLogs, err := model.ListPlatformSiteUsageCostLogs(account.ChannelID, start, end)
	if err != nil {
		return false, 0, 0, false, err
	}
	attemptNumber := window.Attempts + 1
	if len(localLogs) == 0 {
		if err := model.SavePlatformSiteUsageCostWindow(window, now, 0); err != nil {
			return false, 0, 0, false, err
		}
		return true, 0, 0, false, nil
	}

	upstreamRecords, upstreamErr := fetchPlatformSiteUsageCostRecords(ctx, account, start, end)
	matchingLogs, err := model.ListPlatformSiteUsageCostMatchingLogs(account.ChannelID, start, end)
	if err != nil {
		return false, 0, 0, false, err
	}
	matched := matchPlatformSiteUsageCostRecords(matchingLogs, upstreamRecords)
	upstreamCount := 0
	for _, log := range localLogs {
		record, ok := matched[log.RequestId]
		if !ok {
			continue
		}
		if err := model.UpsertPlatformSiteUsageCost(model.PlatformSiteUsageCost{
			RequestID:         log.RequestId,
			ChannelID:         account.ChannelID,
			UpstreamRequestID: record.requestID,
			Cost:              record.cost,
			Source:            model.PlatformSiteUsageCostSourceUpstream,
			UpdatedAt:         now,
		}); err != nil {
			return false, upstreamCount, 0, upstreamErr != nil, err
		}
		upstreamCount++
	}

	estimatedCount := 0
	missingUpstreamCost := false
	for i := range localLogs {
		log := &localLogs[i]
		if log.RequestId == "" {
			continue
		}
		if _, ok := matched[log.RequestId]; ok {
			continue
		}
		if estimate, ok := estimatePlatformSiteUsageCost(*log); ok {
			if err := model.UpsertPlatformSiteUsageCost(model.PlatformSiteUsageCost{
				RequestID: log.RequestId,
				ChannelID: account.ChannelID,
				Cost:      estimate,
				Source:    model.PlatformSiteUsageCostSourceEstimated,
				UpdatedAt: now,
			}); err != nil {
				return false, upstreamCount, estimatedCount, upstreamErr != nil, err
			}
			estimatedCount++
		}
		missingUpstreamCost = true
	}

	retryAfter := int64(0)
	if missingUpstreamCost && attemptNumber <= platformSiteUsageCostMaxRetries {
		retryAfter = platformSiteUsageCostRetrySeconds
	}
	if err := model.SavePlatformSiteUsageCostWindow(window, now, retryAfter); err != nil {
		return false, upstreamCount, estimatedCount, upstreamErr != nil, err
	}
	return true, upstreamCount, estimatedCount, upstreamErr != nil, nil
}

func fetchPlatformSiteUsageCostRecords(
	ctx context.Context,
	account *model.PlatformSiteAccount,
	start int64,
	end int64,
) ([]platformSiteUsageCostRecord, error) {
	session, adapter, err := authenticatePlatformSiteUsageCostAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	defer cleanupPlatformSiteUsageCostSession(adapter, session)

	switch account.Platform {
	case model.PlatformSub2API:
		return fetchSub2APIUsageCosts(ctx, session, start, end)
	case model.PlatformNewAPI:
		return fetchNewAPIUsageCosts(ctx, session, start, end)
	default:
		return nil, ErrUnsupportedPlatformSite
	}
}

func authenticatePlatformSiteUsageCostAccount(
	ctx context.Context,
	account *model.PlatformSiteAccount,
) (*PlatformSiteSession, PlatformSiteAdapter, error) {
	if account == nil {
		return nil, nil, ErrPlatformSiteCredential
	}
	credential, err := model.DecryptPlatformSiteCredential(account.CredentialCiphertext)
	if err != nil {
		return nil, nil, ErrPlatformSiteCredential
	}
	authType := strings.ToLower(strings.TrimSpace(account.AuthType))
	if !model.IsPlatformSiteAuthType(authType) {
		authType = model.InferPlatformSiteAuthType(credential)
	}
	if !model.IsPlatformSiteAuthType(authType) {
		return nil, nil, ErrPlatformSiteAuth
	}
	credential.AuthType = authType
	client, err := platformSiteHTTPClientForChannel(account.ChannelID)
	if err != nil {
		return nil, nil, err
	}
	adapter, err := adapterForPlatform(account.Platform, client)
	if err != nil {
		return nil, nil, err
	}
	session, authErr := adapter.Authenticate(ctx, account.BaseURL, credential)
	if session == nil {
		if authErr != nil {
			return nil, nil, authErr
		}
		return nil, nil, ErrPlatformSiteAuth
	}
	if authErr != nil {
		return nil, nil, authErr
	}
	if session.CredentialUpdate != nil {
		if err := persistPlatformSiteCredential(account, *session.CredentialUpdate); err != nil {
			return nil, nil, err
		}
	}
	return session, adapter, nil
}

func cleanupPlatformSiteUsageCostSession(adapter PlatformSiteAdapter, session *PlatformSiteSession) {
	if adapter == nil || session == nil {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), upstreamSiteRequestTimeout)
	defer cancel()
	_ = adapter.Cleanup(cleanupCtx, session)
}

func fetchPlatformSiteUsageCostRecordsOnce(
	ctx context.Context,
	account *model.PlatformSiteAccount,
	createdAt time.Time,
	upstreamRequestID string,
	modelName string,
) ([]platformSiteUsageCostRecord, error) {
	session, adapter, err := authenticatePlatformSiteUsageCostAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	defer cleanupPlatformSiteUsageCostSession(adapter, session)

	switch account.Platform {
	case model.PlatformSub2API:
		return fetchSub2APIUsageCostsOnce(ctx, session, createdAt, upstreamRequestID, modelName)
	case model.PlatformNewAPI:
		return fetchNewAPIUsageCostsOnce(ctx, session, createdAt, upstreamRequestID, modelName)
	default:
		return nil, ErrUnsupportedPlatformSite
	}
}

func fetchSub2APIUsageCosts(
	ctx context.Context,
	session *PlatformSiteSession,
	start int64,
	end int64,
) ([]platformSiteUsageCostRecord, error) {
	firstDate := time.Unix(start-platformSiteUsageCostMatchWindowSeconds, 0).UTC().Format("2006-01-02")
	lastDate := time.Unix(end+platformSiteUsageCostMatchWindowSeconds, 0).UTC().Format("2006-01-02")
	result := make([]platformSiteUsageCostRecord, 0)
	for page := 1; page <= platformSiteUsageCostMaxPages; page++ {
		query := url.Values{
			"start_date": {firstDate},
			"end_date":   {lastDate},
			"timezone":   {"UTC"},
			"page":       {strconv.Itoa(page)},
			"page_size":  {strconv.Itoa(platformSiteUsageCostPageSize)},
			"sort_by":    {"created_at"},
			"sort_order": {"desc"},
		}
		payload, err := fetchSub2APIUsagePage(ctx, session, query, "")
		if err != nil {
			return nil, err
		}
		items, pages, err := usageCostPage(payload)
		if err != nil {
			return nil, err
		}
		oldestAt := time.Time{}
		for _, item := range items {
			if object, ok := item.(map[string]any); ok {
				createdAt := parsePlatformSiteUsageTimestamp(firstMapValue(object, "created_at", "createdAt", "created_at_unix"))
				if !createdAt.IsZero() && (oldestAt.IsZero() || createdAt.Before(oldestAt)) {
					oldestAt = createdAt
				}
			}
			record, ok := parsePlatformSiteUsageCostRecord(item, model.PlatformSub2API)
			if !ok {
				continue
			}
			if !record.createdAt.Before(time.Unix(start-platformSiteUsageCostMatchWindowSeconds, 0).UTC()) &&
				!record.createdAt.After(time.Unix(end+platformSiteUsageCostMatchWindowSeconds, 0).UTC()) {
				result = append(result, record)
			}
		}
		if len(items) == 0 ||
			(page >= pages && pages > 0) ||
			(pages == 0 && len(items) < platformSiteUsageCostPageSize) ||
			(!oldestAt.IsZero() && oldestAt.Before(time.Unix(start-platformSiteUsageCostMatchWindowSeconds, 0).UTC())) {
			return result, nil
		}
	}
	return nil, errors.New("Sub2API 使用日志分页超出限制")
}

func fetchSub2APIUsageCostsOnce(
	ctx context.Context,
	session *PlatformSiteSession,
	createdAt time.Time,
	upstreamRequestID string,
	modelName string,
) ([]platformSiteUsageCostRecord, error) {
	if session == nil {
		return nil, ErrPlatformSiteSessionLimit
	}
	query := url.Values{
		"start_date": {createdAt.Add(-platformSiteUsageCostMatchWindow).UTC().Format("2006-01-02")},
		"end_date":   {createdAt.Add(platformSiteUsageCostMatchWindow).UTC().Format("2006-01-02")},
		"timezone":   {"UTC"},
		"page":       {"1"},
		"page_size":  {strconv.Itoa(platformSiteUsageCostPageSize)},
		"sort_by":    {"created_at"},
		"sort_order": {"desc"},
	}
	if modelName != "" {
		query.Set("model", modelName)
	}
	payload, err := fetchSub2APIUsagePage(ctx, session, query, upstreamRequestID)
	if err != nil {
		return nil, err
	}
	items, _, err := usageCostPage(payload)
	if err != nil {
		return nil, err
	}
	result := make([]platformSiteUsageCostRecord, 0, len(items))
	for _, item := range items {
		if record, ok := parsePlatformSiteUsageCostRecord(item, model.PlatformSub2API); ok {
			result = append(result, record)
		}
	}
	return result, nil
}

func fetchSub2APIUsagePage(
	ctx context.Context,
	session *PlatformSiteSession,
	query url.Values,
	upstreamRequestID string,
) (any, error) {
	payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/usage", query, nil)
	if err == nil || !platformSiteRouteMissing(err) {
		return payload, err
	}
	adminQuery := make(url.Values, len(query))
	maps.Copy(adminQuery, query)
	if upstreamRequestID != "" {
		adminQuery.Set("request_id", upstreamRequestID)
	}
	return platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/admin/usage", adminQuery, nil)
}

func fetchNewAPIUsageCosts(
	ctx context.Context,
	session *PlatformSiteSession,
	start int64,
	end int64,
) ([]platformSiteUsageCostRecord, error) {
	result := make([]platformSiteUsageCostRecord, 0)
	for page := 1; page <= platformSiteUsageCostMaxPages; page++ {
		query := url.Values{
			"type":            {strconv.Itoa(model.LogTypeConsume)},
			"start_timestamp": {strconv.FormatInt(start-platformSiteUsageCostMatchWindowSeconds, 10)},
			"end_timestamp":   {strconv.FormatInt(end+platformSiteUsageCostMatchWindowSeconds, 10)},
			"p":               {strconv.Itoa(page)},
			"page_size":       {strconv.Itoa(platformSiteUsageCostPageSize)},
		}
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/log/", query, nil)
		if err != nil {
			return nil, err
		}
		items, pages, err := usageCostPage(payload)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if record, ok := parsePlatformSiteUsageCostRecord(item, model.PlatformNewAPI); ok &&
				!record.createdAt.Before(time.Unix(start-platformSiteUsageCostMatchWindowSeconds, 0).UTC()) &&
				!record.createdAt.After(time.Unix(end+platformSiteUsageCostMatchWindowSeconds, 0).UTC()) {
				result = append(result, record)
			}
		}
		if len(items) == 0 ||
			(page >= pages && pages > 0) ||
			(pages == 0 && len(items) < platformSiteUsageCostPageSize) {
			return result, nil
		}
	}
	return nil, errors.New("NewAPI 使用日志分页超出限制")
}

func fetchNewAPIUsageCostsOnce(
	ctx context.Context,
	session *PlatformSiteSession,
	createdAt time.Time,
	upstreamRequestID string,
	modelName string,
) ([]platformSiteUsageCostRecord, error) {
	if session == nil {
		return nil, ErrPlatformSiteSessionLimit
	}
	start := createdAt.Add(-platformSiteUsageCostMatchWindow).Unix()
	end := createdAt.Add(platformSiteUsageCostMatchWindow).Unix()
	query := url.Values{
		"type":            {strconv.Itoa(model.LogTypeConsume)},
		"start_timestamp": {strconv.FormatInt(start, 10)},
		"end_timestamp":   {strconv.FormatInt(end, 10)},
		"p":               {"1"},
		"page_size":       {strconv.Itoa(platformSiteUsageCostPageSize)},
	}
	if upstreamRequestID != "" {
		query.Set("request_id", upstreamRequestID)
	}
	if modelName != "" {
		query.Set("model_name", modelName)
	}
	payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/log/", query, nil)
	if err != nil {
		return nil, err
	}
	items, _, err := usageCostPage(payload)
	if err != nil {
		return nil, err
	}
	result := make([]platformSiteUsageCostRecord, 0, len(items))
	for _, item := range items {
		if record, ok := parsePlatformSiteUsageCostRecord(item, model.PlatformNewAPI); ok {
			result = append(result, record)
		}
	}
	return result, nil
}

func usageCostPage(payload any) ([]any, int, error) {
	object, ok := payload.(map[string]any)
	if !ok {
		return nil, 0, ErrPlatformSiteResponse
	}
	data, ok := object["data"].(map[string]any)
	if !ok {
		return nil, 0, ErrPlatformSiteResponse
	}
	items, ok := data["items"].([]any)
	if !ok {
		return nil, 0, ErrPlatformSiteResponse
	}
	pages := numericInt(data["pages"])
	if pages == 0 {
		pages = numericInt(data["total_pages"])
	}
	if pages == 0 {
		total := numericInt(data["total"])
		pageSize := numericInt(data["page_size"])
		if total > 0 && pageSize > 0 {
			pages = total / pageSize
			if total%pageSize > 0 {
				pages++
			}
		}
	}
	return items, pages, nil
}

func parsePlatformSiteUsageCostRecord(value any, platform string) (platformSiteUsageCostRecord, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return platformSiteUsageCostRecord{}, false
	}
	modelNames := firstMapStrings(
		object,
		"model",
		"model_name",
		"modelName",
		"requested_model",
		"requestedModel",
		"upstream_model",
		"upstreamModel",
		"upstream_response_model",
		"upstreamResponseModel",
	)
	modelName := ""
	if len(modelNames) > 0 {
		modelName = modelNames[0]
	}
	createdAt := parsePlatformSiteUsageTimestamp(firstMapValue(object, "created_at", "createdAt", "created_at_unix"))
	costKeys := []string{"quota"}
	if platform == model.PlatformSub2API {
		costKeys = []string{"actual_cost", "actualCost"}
	}
	rawCost := firstMapValue(object, costKeys...)
	costValue, ok := numericFloat(rawCost)
	if !ok || math.IsNaN(costValue) || math.IsInf(costValue, 0) || costValue < 0 {
		return platformSiteUsageCostRecord{}, false
	}
	var cost int
	var err error
	if platform == model.PlatformSub2API {
		cost, err = common.QuotaRoundStrict(costValue * common.QuotaPerUnit)
	} else {
		cost, err = common.QuotaRoundStrict(costValue)
	}
	if err != nil {
		return platformSiteUsageCostRecord{}, false
	}
	requestID := firstMapString(
		object,
		"upstream_request_id",
		"upstreamRequestId",
		"request_id",
		"requestId",
	)
	if cost < 0 || (requestID == "" && (modelName == "" || createdAt.IsZero())) {
		return platformSiteUsageCostRecord{}, false
	}
	return platformSiteUsageCostRecord{
		requestID:  requestID,
		model:      modelName,
		modelNames: modelNames,
		createdAt:  createdAt,
		cost:       int64(cost),
	}, true
}

func firstMapValue(object map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := object[key]; ok && value != nil {
			return value
		}
	}
	return nil
}

func firstMapString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstMapStrings(object map[string]any, keys ...string) []string {
	values := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		value, ok := object[key].(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func numericFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func numericInt(value any) int {
	number, ok := numericFloat(value)
	if !ok || number <= 0 || number >= float64(math.MaxInt) {
		return 0
	}
	return int(number)
}

func parsePlatformSiteUsageTimestamp(value any) time.Time {
	switch typed := value.(type) {
	case int:
		return parsePlatformSiteUsageUnixTimestamp(int64(typed))
	case int64:
		return parsePlatformSiteUsageUnixTimestamp(typed)
	case int32:
		return parsePlatformSiteUsageUnixTimestamp(int64(typed))
	case json.Number:
		integer, err := typed.Int64()
		if err == nil {
			return parsePlatformSiteUsageUnixTimestamp(integer)
		}
		number, err := typed.Float64()
		if err == nil {
			return parsePlatformSiteUsageFloatTimestamp(number)
		}
	case float64:
		return parsePlatformSiteUsageFloatTimestamp(typed)
	case float32:
		return parsePlatformSiteUsageFloatTimestamp(float64(typed))
	}
	text, ok := value.(string)
	if !ok {
		return time.Time{}
	}
	text = strings.TrimSpace(text)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00"} {
		parsed, err := time.Parse(layout, text)
		if err == nil {
			return parsed.UTC()
		}
	}
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05.999999999", text, time.UTC); err == nil {
		return parsed
	}
	if unix, err := strconv.ParseInt(text, 10, 64); err == nil {
		return parsePlatformSiteUsageUnixTimestamp(unix)
	}
	if number, err := strconv.ParseFloat(text, 64); err == nil {
		return parsePlatformSiteUsageFloatTimestamp(number)
	}
	return time.Time{}
}

func parsePlatformSiteUsageUnixTimestamp(value int64) time.Time {
	switch {
	case value > 1e17:
		return time.Unix(0, value).UTC()
	case value > 1e12:
		return time.UnixMilli(value).UTC()
	case value > 0:
		return time.Unix(value, 0).UTC()
	default:
		return time.Time{}
	}
}

func parsePlatformSiteUsageFloatTimestamp(value float64) time.Time {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return time.Time{}
	}
	if value > 1e17 {
		if value >= float64(math.MaxInt64) {
			return time.Time{}
		}
		return time.Unix(0, int64(math.Round(value))).UTC()
	}
	if value > 1e12 {
		if value >= float64(math.MaxInt64) {
			return time.Time{}
		}
		return time.UnixMilli(int64(math.Round(value))).UTC()
	}
	if value >= float64(math.MaxInt64) {
		return time.Time{}
	}
	seconds, fraction := math.Modf(value)
	nanoseconds := int64(math.Round(fraction * float64(time.Second)))
	return time.Unix(int64(seconds), nanoseconds).UTC()
}

func matchPlatformSiteUsageCostRecords(
	logs []model.Log,
	records []platformSiteUsageCostRecord,
) map[string]platformSiteUsageCostRecord {
	matched := make(map[string]platformSiteUsageCostRecord)
	usedLogs := make(map[int]bool)
	usedRecords := make(map[int]bool)
	logModels := make([][]string, len(logs))
	for logIndex := range logs {
		logModels[logIndex] = platformSiteUsageCostLogModels(logs[logIndex])
	}
	recordIDs := make(map[string]int)
	for _, record := range records {
		if record.requestID != "" {
			recordIDs[record.requestID]++
		}
	}

	for recordIndex, record := range records {
		if record.requestID == "" {
			continue
		}
		if recordIDs[record.requestID] != 1 {
			usedRecords[recordIndex] = true
			continue
		}
		candidates := make([]int, 0, 1)
		for logIndex := range logs {
			if logs[logIndex].RequestId != "" && logs[logIndex].UpstreamRequestId == record.requestID {
				candidates = append(candidates, logIndex)
			}
		}
		if len(candidates) == 1 && !usedLogs[candidates[0]] {
			logIndex := candidates[0]
			matched[logs[logIndex].RequestId] = record
			usedLogs[logIndex] = true
			usedRecords[recordIndex] = true
		} else if len(candidates) > 1 {
			usedRecords[recordIndex] = true
		}
	}

	type candidatePair struct {
		recordIndex int
		logIndex    int
	}
	pairs := make([]candidatePair, 0)
	for recordIndex, record := range records {
		if usedRecords[recordIndex] ||
			record.createdAt.IsZero() ||
			(record.model == "" && len(record.modelNames) == 0) {
			continue
		}
		minDistance := time.Duration(math.MaxInt64)
		candidates := make([]int, 0, 2)
		for logIndex := range logs {
			log := logs[logIndex]
			if usedLogs[logIndex] ||
				log.RequestId == "" ||
				!platformSiteUsageCostModelsMatch(logModels[logIndex], record) ||
				(record.requestID != "" &&
					log.UpstreamRequestId != "" &&
					log.UpstreamRequestId != record.requestID) {
				continue
			}
			distance := record.createdAt.Sub(time.Unix(log.CreatedAt, 0).UTC())
			if distance < -platformSiteUsageCostMatchWindow || distance > platformSiteUsageCostMatchWindow {
				continue
			}
			if distance < 0 {
				distance = -distance
			}
			if distance < minDistance {
				minDistance = distance
				candidates = candidates[:0]
			}
			if distance == minDistance {
				candidates = append(candidates, logIndex)
			}
		}
		if len(candidates) == 1 {
			pairs = append(pairs, candidatePair{
				recordIndex: recordIndex,
				logIndex:    candidates[0],
			})
		}
	}
	recordPairs := make(map[int][]candidatePair)
	logPairs := make(map[int][]candidatePair)
	for _, pair := range pairs {
		recordPairs[pair.recordIndex] = append(recordPairs[pair.recordIndex], pair)
		logPairs[pair.logIndex] = append(logPairs[pair.logIndex], pair)
	}
	for _, pair := range pairs {
		if len(recordPairs[pair.recordIndex]) != 1 || len(logPairs[pair.logIndex]) != 1 {
			continue
		}
		log := logs[pair.logIndex]
		matched[log.RequestId] = records[pair.recordIndex]
	}
	return matched
}

func matchPlatformSiteUsageCostRequest(
	request PlatformSiteUsageCostRequest,
	records []platformSiteUsageCostRecord,
) *platformSiteUsageCostRecord {
	if request.RequestID == "" {
		return nil
	}
	if request.UpstreamRequestID != "" {
		var matched *platformSiteUsageCostRecord
		for i := range records {
			if records[i].requestID != request.UpstreamRequestID {
				continue
			}
			if matched != nil {
				return nil
			}
			matched = &records[i]
		}
		if matched != nil {
			return matched
		}
	}

	log := model.Log{
		RequestId:         request.RequestID,
		ChannelId:         request.ChannelID,
		UpstreamRequestId: request.UpstreamRequestID,
		ModelName:         request.ModelName,
		CreatedAt:         request.CreatedAt.Unix(),
	}
	matched := matchPlatformSiteUsageCostRecords([]model.Log{log}, records)
	record, ok := matched[request.RequestID]
	if !ok {
		return nil
	}
	return &record
}

func platformSiteUsageCostLogModels(log model.Log) []string {
	models := firstNonEmptyStrings(log.ModelName)
	if log.Other == "" {
		return models
	}
	var other map[string]any
	if err := common.Unmarshal([]byte(log.Other), &other); err != nil {
		return models
	}
	for _, key := range []string{"upstream_model_name", "billing_model"} {
		if value, ok := other[key].(string); ok {
			models = appendUniqueString(models, value)
		}
	}
	if adminInfo, ok := other["admin_info"].(map[string]any); ok {
		if value, ok := adminInfo["billing_model"].(string); ok {
			models = appendUniqueString(models, value)
		}
	}
	return models
}

func firstNonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = appendUniqueString(result, value)
		}
	}
	return result
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func platformSiteUsageCostModelsMatch(logModels []string, record platformSiteUsageCostRecord) bool {
	if len(logModels) == 0 {
		return false
	}
	recordModels := record.modelNames
	if len(recordModels) == 0 && record.model != "" {
		recordModels = []string{record.model}
	}
	for _, logModel := range logModels {
		for _, recordModel := range recordModels {
			if logModel == recordModel {
				return true
			}
		}
	}
	return false
}

func estimatePlatformSiteUsageCost(log model.Log) (int64, bool) {
	if log.Quota < 0 || log.Other == "" {
		return 0, false
	}
	var other map[string]any
	if err := common.Unmarshal([]byte(log.Other), &other); err != nil {
		return 0, false
	}
	adminInfo, ok := other["admin_info"].(map[string]any)
	if !ok {
		return 0, false
	}
	ratio, ok := numericFloat(adminInfo["upstream_key_ratio"])
	if !ok || ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return 0, false
	}
	cost, err := common.QuotaRoundStrict(float64(log.Quota) * ratio)
	if err != nil {
		return 0, false
	}
	if cost < 0 {
		return 0, false
	}
	return int64(cost), true
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
