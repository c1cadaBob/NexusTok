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
	"sync"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/constant"
	"github.com/c1cadaBob/NexusTok/model"

	"github.com/gin-gonic/gin"
)

const (
	platformSiteUsageCostWindowSeconds          = int64(5 * 60)
	platformSiteUsageCostRetrySeconds           = int64(5 * 60)
	platformSiteUsageCostMaxRetries             = 3
	platformSiteUsageCostPageSize               = 100
	platformSiteUsageCostMaxPages               = 1000
	platformSiteUsageCostBatchWindows           = 24
	platformSiteUsageCostLookupTimeout          = 8 * time.Second
	platformSiteUsageCostMatchWindowSeconds     = int64(3)
	platformSiteUsageCostMatchWindow            = 3 * time.Second
	platformSiteUsageCostSessionTTL             = 10 * time.Minute
	platformSiteUsageCostSessionExpirySkew      = 30 * time.Second
	platformSiteUsageCostSessionFailureCooldown = time.Minute
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
	RequestID             string
	ChannelID             int
	UpstreamRequestID     string
	PlatformSiteRequestID string
	ModelName             string
	CreatedAt             time.Time
	Quota                 int
	Other                 *model.LogOther
	IsError               bool
	CarriedCost           *int64
	CarriedCostPlatform   string
}

type platformSiteUsageCostState struct {
	errorHandled   bool
	queryAttempted bool
}

type platformSiteCarriedCost struct {
	Platform string
	Cost     int64
}

type platformSiteUsageCostSessionCacheEntry struct {
	mu                    sync.Mutex
	session               *PlatformSiteSession
	adapter               PlatformSiteAdapter
	credentialFingerprint string
	expiresAt             time.Time
	authFailure           error
	authFailureUntil      time.Time
}

type platformSiteUsageCostSessionLease struct {
	entry   *platformSiteUsageCostSessionCacheEntry
	session *PlatformSiteSession
	adapter PlatformSiteAdapter
}

var platformSiteUsageCostSessionCache = struct {
	sync.Mutex
	entries map[int]*platformSiteUsageCostSessionCacheEntry
}{
	entries: make(map[int]*platformSiteUsageCostSessionCacheEntry),
}

type platformSiteUsageCostDiagnostic struct {
	reason     string
	statusCode int
	errorCode  string
}

var errPlatformSiteUsageCostInvalidRecord = errors.New("平台站点使用日志成本字段无效")

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
	if request.PlatformSiteRequestID == "" {
		request.PlatformSiteRequestID = common.GetContextKeyString(
			ctx,
			constant.ContextKeyPlatformSiteClientRequestID,
		)
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
		setPlatformSiteUsageCostFallbackDiagnostic(
			request.Other,
			platformSiteUsageCostDiagnostic{reason: model.PlatformSiteUsageCostFallbackRequestError},
		)
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
		clearPlatformSiteUsageCostFallbackDiagnostic(request.Other)
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
		clearPlatformSiteUsageCostFallbackDiagnostic(request.Other)
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
		records, lookupErr := fetchPlatformSiteUsageCostRecordsOnce(
			lookupCtx,
			account,
			request.CreatedAt,
			request.PlatformSiteRequestID,
			request.UpstreamRequestID,
			request.ModelName,
		)
		cancel()
		if lookupErr == nil {
			var diagnostic platformSiteUsageCostDiagnostic
			matchRequest := platformSiteUsageCostMatchRequestForPlatform(request, account.Platform)
			record, diagnostic = matchPlatformSiteUsageCostRequestWithDiagnostic(matchRequest, records)
			if len(records) == 0 {
				diagnostic = platformSiteUsageCostDiagnostic{reason: model.PlatformSiteUsageCostFallbackEmptyResult}
			} else if record == nil {
				if diagnostic.reason == "" {
					diagnostic.reason = model.PlatformSiteUsageCostFallbackModelTimeNoMatch
				}
			}
			if record == nil {
				setPlatformSiteUsageCostFallbackDiagnostic(request.Other, diagnostic)
			}
			if record != nil {
				clearPlatformSiteUsageCostFallbackDiagnostic(request.Other)
			}
			if diagnostic.reason != "" {
				logPlatformSiteUsageCostDiagnostic(request.ChannelID, account.Platform, diagnostic.reason, nil)
			}
		} else {
			diagnostic := platformSiteUsageCostDiagnosticFromError(lookupErr)
			setPlatformSiteUsageCostFallbackDiagnostic(request.Other, diagnostic)
			logPlatformSiteUsageCostDiagnostic(request.ChannelID, account.Platform, diagnostic.reason, lookupErr)
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
	if source == model.PlatformSiteUsageCostSourceUpstream {
		clearPlatformSiteUsageCostFallbackDiagnostic(request.Other)
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

func platformSiteUsageCostMatchRequestForPlatform(
	request PlatformSiteUsageCostRequest,
	platform string,
) PlatformSiteUsageCostRequest {
	switch platform {
	case model.PlatformSub2API:
		request.UpstreamRequestID = ""
	case model.PlatformNewAPI:
		request.PlatformSiteRequestID = ""
	}
	return request
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
		PlatformSiteRequestID: common.GetContextKeyString(
			ctx,
			constant.ContextKeyPlatformSiteClientRequestID,
		),
		ModelName: modelName,
		Quota:     quota,
		Other:     other,
		IsError:   isError,
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
	matched := matchPlatformSiteUsageCostRecordsForPlatform(account.Platform, matchingLogs, upstreamRecords)
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
	for attempt := 0; attempt < 2; attempt++ {
		lease, err := acquirePlatformSiteUsageCostSession(ctx, account)
		if err != nil {
			return nil, err
		}
		var records []platformSiteUsageCostRecord
		switch account.Platform {
		case model.PlatformSub2API:
			records, err = fetchSub2APIUsageCosts(ctx, lease.session, start, end)
		case model.PlatformNewAPI:
			records, err = fetchNewAPIUsageCosts(ctx, lease.session, start, end)
		default:
			err = ErrUnsupportedPlatformSite
		}
		if err != nil && attempt == 0 && platformSiteUsageCostSessionInvalid(err) {
			lease.invalidate()
			lease.release()
			continue
		}
		lease.release()
		return records, err
	}
	return nil, ErrPlatformSiteAuth
}

func acquirePlatformSiteUsageCostSession(
	ctx context.Context,
	account *model.PlatformSiteAccount,
) (*platformSiteUsageCostSessionLease, error) {
	if account == nil {
		return nil, ErrPlatformSiteCredential
	}
	credential, err := model.DecryptPlatformSiteCredential(account.CredentialCiphertext)
	if err != nil {
		return nil, ErrPlatformSiteCredential
	}
	authType := strings.ToLower(strings.TrimSpace(account.AuthType))
	if !model.IsPlatformSiteAuthType(authType) {
		authType = model.InferPlatformSiteAuthType(credential)
	}
	if !model.IsPlatformSiteAuthType(authType) {
		return nil, ErrPlatformSiteAuth
	}
	credential.AuthType = authType
	fingerprint := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(account.Platform)),
		normalizePlatformSiteURLForCache(account.BaseURL),
		credential.Fingerprint(),
	}, "\x00")

	platformSiteUsageCostSessionCache.Lock()
	entry := platformSiteUsageCostSessionCache.entries[account.ChannelID]
	if entry == nil {
		entry = &platformSiteUsageCostSessionCacheEntry{}
		platformSiteUsageCostSessionCache.entries[account.ChannelID] = entry
	}
	platformSiteUsageCostSessionCache.Unlock()

	entry.mu.Lock()
	now := time.Now()
	if entry.authFailure != nil &&
		entry.credentialFingerprint == fingerprint &&
		now.Before(entry.authFailureUntil) {
		authFailure := entry.authFailure
		entry.mu.Unlock()
		return nil, authFailure
	}
	if entry.authFailure != nil {
		entry.authFailure = nil
		entry.authFailureUntil = time.Time{}
	}
	if entry.session != nil && entry.adapter != nil &&
		entry.credentialFingerprint == fingerprint && now.Before(entry.expiresAt) {
		return &platformSiteUsageCostSessionLease{
			entry:   entry,
			session: entry.session,
			adapter: entry.adapter,
		}, nil
	}
	if entry.session != nil && entry.adapter != nil {
		cleanupPlatformSiteUsageCostSession(entry.adapter, entry.session)
		entry.session = nil
		entry.adapter = nil
		entry.credentialFingerprint = ""
		entry.expiresAt = time.Time{}
	}

	client, err := platformSiteHTTPClientForChannel(account.ChannelID)
	if err != nil {
		entry.mu.Unlock()
		return nil, err
	}
	adapter, err := adapterForPlatform(account.Platform, client)
	if err != nil {
		entry.mu.Unlock()
		return nil, err
	}
	session, authErr := adapter.Authenticate(ctx, account.BaseURL, credential)
	if session == nil {
		entry.mu.Unlock()
		if authErr != nil {
			return nil, authErr
		}
		return nil, ErrPlatformSiteAuth
	}
	if authErr != nil {
		cleanupPlatformSiteUsageCostSession(adapter, session)
		if platformSiteSessionLimit(authErr) {
			entry.credentialFingerprint = fingerprint
			entry.authFailure = authErr
			entry.authFailureUntil = time.Now().Add(platformSiteUsageCostSessionFailureCooldown)
		}
		entry.mu.Unlock()
		return nil, authErr
	}
	if session.CredentialUpdate != nil {
		if err := persistPlatformSiteCredential(account, *session.CredentialUpdate); err != nil {
			cleanupPlatformSiteUsageCostSession(adapter, session)
			entry.mu.Unlock()
			return nil, err
		}
		fingerprint = strings.Join([]string{
			strings.ToLower(strings.TrimSpace(account.Platform)),
			normalizePlatformSiteURLForCache(account.BaseURL),
			account.CredentialFingerprint,
		}, "\x00")
	}
	entry.session = session
	entry.adapter = adapter
	entry.credentialFingerprint = fingerprint
	entry.expiresAt = platformSiteUsageCostSessionExpiresAt(session, credential)
	entry.authFailure = nil
	entry.authFailureUntil = time.Time{}
	return &platformSiteUsageCostSessionLease{
		entry:   entry,
		session: session,
		adapter: adapter,
	}, nil
}

func (lease *platformSiteUsageCostSessionLease) release() {
	if lease == nil || lease.entry == nil {
		return
	}
	lease.entry.mu.Unlock()
	lease.entry = nil
}

func (lease *platformSiteUsageCostSessionLease) invalidate() {
	if lease == nil || lease.entry == nil || lease.entry.session != lease.session {
		return
	}
	cleanupPlatformSiteUsageCostSession(lease.entry.adapter, lease.entry.session)
	lease.entry.session = nil
	lease.entry.adapter = nil
	lease.entry.credentialFingerprint = ""
	lease.entry.expiresAt = time.Time{}
	lease.entry.authFailure = nil
	lease.entry.authFailureUntil = time.Time{}
}

func platformSiteUsageCostSessionInvalid(err error) bool {
	if err == nil || platformSiteSessionLimit(err) {
		return false
	}
	if errors.Is(err, ErrPlatformSiteCredentials) {
		return true
	}
	status, ok := platformSiteHTTPStatusCode(err)
	return ok && status == http.StatusUnauthorized
}

func platformSiteUsageCostSessionExpiresAt(
	session *PlatformSiteSession,
	credential model.PlatformSiteCredential,
) time.Time {
	now := time.Now()
	expiresAt := now.Add(platformSiteUsageCostSessionTTL)
	tokenExpiresAt := credential.TokenExpiresAt
	if session != nil && session.TemporaryExpiresAt > 0 {
		tokenExpiresAt = session.TemporaryExpiresAt
	}
	if session != nil && session.CredentialUpdate != nil &&
		session.CredentialUpdate.TokenExpiresAt > 0 {
		tokenExpiresAt = session.CredentialUpdate.TokenExpiresAt
	}
	if tokenExpiresAt > 0 {
		tokenExpiry := time.Unix(tokenExpiresAt, 0)
		if tokenExpiry.Before(expiresAt) {
			expiresAt = tokenExpiry.Add(-platformSiteUsageCostSessionExpirySkew)
		}
	}
	return expiresAt
}

func normalizePlatformSiteURLForCache(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func clearPlatformSiteUsageCostSessionCache(channelID int) {
	platformSiteUsageCostSessionCache.Lock()
	entry := platformSiteUsageCostSessionCache.entries[channelID]
	delete(platformSiteUsageCostSessionCache.entries, channelID)
	platformSiteUsageCostSessionCache.Unlock()
	if entry == nil {
		return
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	cleanupPlatformSiteUsageCostSession(entry.adapter, entry.session)
	entry.session = nil
	entry.adapter = nil
	entry.credentialFingerprint = ""
	entry.expiresAt = time.Time{}
	entry.authFailure = nil
	entry.authFailureUntil = time.Time{}
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
	platformSiteRequestID string,
	upstreamRequestID string,
	modelName string,
) ([]platformSiteUsageCostRecord, error) {
	for attempt := 0; attempt < 2; attempt++ {
		lease, err := acquirePlatformSiteUsageCostSession(ctx, account)
		if err != nil {
			return nil, err
		}
		var records []platformSiteUsageCostRecord
		switch account.Platform {
		case model.PlatformSub2API:
			records, err = fetchSub2APIUsageCostsOnce(ctx, lease.session, createdAt, platformSiteRequestID, modelName)
		case model.PlatformNewAPI:
			records, err = fetchNewAPIUsageCostsOnce(ctx, lease.session, createdAt, upstreamRequestID, modelName)
		default:
			err = ErrUnsupportedPlatformSite
		}
		if err != nil && attempt == 0 && platformSiteUsageCostSessionInvalid(err) {
			lease.invalidate()
			lease.release()
			continue
		}
		lease.release()
		return records, err
	}
	return nil, ErrPlatformSiteAuth
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
	platformSiteRequestID string,
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
	payload, err := fetchSub2APIUsagePage(ctx, session, query, platformSiteRequestID)
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
	if len(items) > 0 && len(result) == 0 {
		return nil, errPlatformSiteUsageCostInvalidRecord
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
	usagePath := "/api/log/self"
	for page := 1; page <= platformSiteUsageCostMaxPages; page++ {
		query := url.Values{
			"type":            {strconv.Itoa(model.LogTypeConsume)},
			"start_timestamp": {strconv.FormatInt(start-platformSiteUsageCostMatchWindowSeconds, 10)},
			"end_timestamp":   {strconv.FormatInt(end+platformSiteUsageCostMatchWindowSeconds, 10)},
			"p":               {strconv.Itoa(page)},
			"page_size":       {strconv.Itoa(platformSiteUsageCostPageSize)},
		}
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, usagePath, query, nil)
		if err != nil && usagePath == "/api/log/self" && platformSiteUsageCostRouteFallback(err) {
			usagePath = "/api/log/"
			payload, err = platformSiteRequest(ctx, session, http.MethodGet, usagePath, query, nil)
		}
		if err != nil {
			return nil, wrapPlatformSiteStage("NewAPI 成本查询 "+usagePath, err)
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
	usagePath := "/api/log/self"
	payload, err := platformSiteRequest(ctx, session, http.MethodGet, usagePath, query, nil)
	if err != nil && platformSiteUsageCostRouteFallback(err) {
		usagePath = "/api/log/"
		payload, err = platformSiteRequest(ctx, session, http.MethodGet, usagePath, query, nil)
	}
	if err != nil {
		return nil, wrapPlatformSiteStage("NewAPI 成本查询 "+usagePath, err)
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
	if len(items) > 0 && len(result) == 0 {
		return nil, errPlatformSiteUsageCostInvalidRecord
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

func platformSiteUsageCostRouteFallback(err error) bool {
	statusCode, ok := platformSiteHTTPStatusCode(err)
	return ok && (statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed)
}

func logPlatformSiteUsageCostDiagnostic(channelID int, platform string, stage string, err error) {
	statusCode, hasStatus := platformSiteHTTPStatusCode(err)
	errorCode := platformSiteErrorCodeOf(err)
	parts := []string{
		"平台站点成本查询诊断",
		fmt.Sprintf("channel_id=%d", channelID),
		"platform=" + strings.TrimSpace(platform),
		"stage=" + stage,
	}
	if hasStatus {
		parts = append(parts, fmt.Sprintf("status=%d", statusCode))
	}
	if errorCode != "" {
		parts = append(parts, "code="+errorCode)
	}
	if err != nil && stage == "query_failed" {
		parts = append(parts, "reason="+SafePlatformSiteError(err))
	}
	common.SysError(strings.Join(parts, " "))
}

func platformSiteUsageCostDiagnosticFromError(err error) platformSiteUsageCostDiagnostic {
	diagnostic := platformSiteUsageCostDiagnostic{
		reason: model.PlatformSiteUsageCostFallbackHTTPFailed,
	}
	if err == nil {
		return diagnostic
	}
	diagnostic.statusCode, _ = platformSiteHTTPStatusCode(err)
	diagnostic.errorCode = platformSiteErrorCodeOf(err)
	switch {
	case platformSiteSessionLimit(err), errors.Is(err, ErrPlatformSiteAuth), errors.Is(err, ErrPlatformSiteCredentials), diagnostic.statusCode == http.StatusUnauthorized:
		diagnostic.reason = model.PlatformSiteUsageCostFallbackAuthenticationFailed
	case errors.Is(err, ErrPlatformSitePermission), diagnostic.statusCode == http.StatusForbidden:
		diagnostic.reason = model.PlatformSiteUsageCostFallbackPermissionDenied
	case errors.Is(err, errPlatformSiteUsageCostInvalidRecord), errors.Is(err, ErrPlatformSiteResponse):
		diagnostic.reason = model.PlatformSiteUsageCostFallbackInvalidRecord
	}
	return diagnostic
}

func setPlatformSiteUsageCostFallbackDiagnostic(other *model.LogOther, diagnostic platformSiteUsageCostDiagnostic) {
	if other == nil || diagnostic.reason == "" {
		return
	}
	other.SetAdmin(model.PlatformSiteUsageCostFallbackReasonKey, diagnostic.reason)
	if diagnostic.statusCode > 0 {
		other.SetAdmin(model.PlatformSiteUsageCostFallbackStatusKey, diagnostic.statusCode)
	} else {
		other.DeleteAdmin(model.PlatformSiteUsageCostFallbackStatusKey)
	}
	if diagnostic.errorCode != "" {
		other.SetAdmin(model.PlatformSiteUsageCostFallbackCodeKey, diagnostic.errorCode)
	} else {
		other.DeleteAdmin(model.PlatformSiteUsageCostFallbackCodeKey)
	}
}

func clearPlatformSiteUsageCostFallbackDiagnostic(other *model.LogOther) {
	if other == nil {
		return
	}
	other.DeleteAdmin(model.PlatformSiteUsageCostFallbackReasonKey)
	other.DeleteAdmin(model.PlatformSiteUsageCostFallbackStatusKey)
	other.DeleteAdmin(model.PlatformSiteUsageCostFallbackCodeKey)
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
	requestIDKeys := []string{"upstream_request_id", "upstreamRequestId", "request_id", "requestId"}
	if platform == model.PlatformSub2API {
		requestIDKeys = []string{"request_id", "requestId", "upstream_request_id", "upstreamRequestId"}
	}
	requestID := firstMapString(object, requestIDKeys...)
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
	return matchPlatformSiteUsageCostRecordsForPlatform("", logs, records)
}

func matchPlatformSiteUsageCostRecordsForPlatform(
	platform string,
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
			if logs[logIndex].RequestId != "" &&
				platformSiteUsageCostLogHasPreciseRequestID(platform, logs[logIndex]) &&
				logs[logIndex].UpstreamRequestId == record.requestID {
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
					platformSiteUsageCostLogHasPreciseRequestID(platform, log) &&
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

func platformSiteUsageCostLogHasPreciseRequestID(platform string, log model.Log) bool {
	if log.UpstreamRequestId == "" {
		return false
	}
	if platform == model.PlatformSub2API {
		return strings.HasPrefix(strings.ToLower(log.UpstreamRequestId), "client:")
	}
	return true
}

func matchPlatformSiteUsageCostRequest(
	request PlatformSiteUsageCostRequest,
	records []platformSiteUsageCostRecord,
) *platformSiteUsageCostRecord {
	record, _ := matchPlatformSiteUsageCostRequestWithDiagnostic(request, records)
	return record
}

func matchPlatformSiteUsageCostRequestWithDiagnostic(
	request PlatformSiteUsageCostRequest,
	records []platformSiteUsageCostRecord,
) (*platformSiteUsageCostRecord, platformSiteUsageCostDiagnostic) {
	if request.RequestID == "" {
		return nil, platformSiteUsageCostDiagnostic{
			reason: model.PlatformSiteUsageCostFallbackRequestIDNoMatch,
		}
	}
	exactRequestID := request.PlatformSiteRequestID
	if exactRequestID == "" {
		exactRequestID = request.UpstreamRequestID
	}
	if exactRequestID != "" {
		var matched *platformSiteUsageCostRecord
		for i := range records {
			if records[i].requestID != exactRequestID {
				continue
			}
			if matched != nil {
				return nil, platformSiteUsageCostDiagnostic{
					reason: model.PlatformSiteUsageCostFallbackAmbiguousMatch,
				}
			}
			matched = &records[i]
		}
		if matched != nil {
			return matched, platformSiteUsageCostDiagnostic{}
		}
		return nil, platformSiteUsageCostDiagnostic{
			reason: model.PlatformSiteUsageCostFallbackRequestIDNoMatch,
		}
	}

	logModels := firstNonEmptyStrings(request.ModelName)
	if len(logModels) == 0 || request.CreatedAt.IsZero() {
		return nil, platformSiteUsageCostDiagnostic{
			reason: model.PlatformSiteUsageCostFallbackModelTimeNoMatch,
		}
	}
	var matched *platformSiteUsageCostRecord
	minDistance := time.Duration(math.MaxInt64)
	ambiguous := false
	for i := range records {
		record := records[i]
		if record.createdAt.IsZero() || !platformSiteUsageCostModelsMatch(logModels, record) {
			continue
		}
		distance := record.createdAt.Sub(request.CreatedAt)
		if distance < -platformSiteUsageCostMatchWindow || distance > platformSiteUsageCostMatchWindow {
			continue
		}
		if distance < 0 {
			distance = -distance
		}
		if distance < minDistance {
			minDistance = distance
			matched = &records[i]
			ambiguous = false
			continue
		}
		if distance == minDistance {
			ambiguous = true
		}
	}
	if matched == nil {
		return nil, platformSiteUsageCostDiagnostic{
			reason: model.PlatformSiteUsageCostFallbackModelTimeNoMatch,
		}
	}
	if ambiguous {
		return nil, platformSiteUsageCostDiagnostic{
			reason: model.PlatformSiteUsageCostFallbackAmbiguousMatch,
		}
	}
	return matched, platformSiteUsageCostDiagnostic{}
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
