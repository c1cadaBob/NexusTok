package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/model"
	"github.com/c1cadaBob/NexusTok/pkg/cachex"

	"github.com/samber/hot"
)

const (
	platformSiteChallengeTTL        = 5 * time.Minute
	platformSiteChallengeMaxTries   = 3
	platformSiteChallengeLockTTL    = 2 * time.Minute
	platformSiteChallengeManual     = "manual"
	platformSiteChallengeBackground = "background"

	platformSiteChallengeStatusWaiting = "waiting_verification"
	platformSiteChallengeStatusSuccess = "success"
)

const (
	PlatformSiteChallengeCodeNotFound       = "UPSTREAM_CHALLENGE_NOT_FOUND"
	PlatformSiteChallengeCodeExpired        = "UPSTREAM_CHALLENGE_EXPIRED"
	PlatformSiteChallengeCodeConsumed       = "UPSTREAM_CHALLENGE_CONSUMED"
	PlatformSiteChallengeCodeMismatch       = "UPSTREAM_CHALLENGE_MISMATCH"
	PlatformSiteChallengeCodePermission     = "UPSTREAM_CHALLENGE_PERMISSION_DENIED"
	PlatformSiteChallengeCodeAttempts       = "UPSTREAM_CHALLENGE_ATTEMPTS_EXCEEDED"
	PlatformSiteChallengeCodeInvalidCode    = "UPSTREAM_CHALLENGE_CODE_INVALID"
	PlatformSiteChallengeCodeBusy           = "UPSTREAM_CHALLENGE_BUSY"
	PlatformSiteChallengeCodeInfrastructure = "UPSTREAM_CHALLENGE_INTERNAL_ERROR"
)

var platformSiteChallengeCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

var (
	ErrPlatformSiteChallengeNotFound    = errors.New("platform site challenge not found")
	ErrPlatformSiteChallengeExpired     = errors.New("platform site challenge expired")
	ErrPlatformSiteChallengeConsumed    = errors.New("platform site challenge consumed")
	ErrPlatformSiteChallengeMismatch    = errors.New("platform site challenge mismatch")
	ErrPlatformSiteChallengePermission  = errors.New("platform site challenge permission denied")
	ErrPlatformSiteChallengeAttempts    = errors.New("platform site challenge attempts exceeded")
	ErrPlatformSiteChallengeInvalidCode = errors.New("platform site challenge code invalid")
	ErrPlatformSiteChallengeBusy        = errors.New("platform site challenge busy")
)

type PlatformSiteChallengeError struct {
	Code    string
	Message string
	Cause   error
}

func (err *PlatformSiteChallengeError) Error() string {
	if err == nil {
		return ""
	}
	return err.Message
}

func (err *PlatformSiteChallengeError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

type PlatformSiteChallenge struct {
	ChallengeID              string `json:"challenge_id"`
	ChannelID                int    `json:"channel_id"`
	Platform                 string `json:"platform"`
	BaseURL                  string `json:"base_url"`
	Origin                   string `json:"origin"`
	CreatedBy                int    `json:"created_by"`
	Source                   string `json:"source"`
	CreatedAt                int64  `json:"created_at"`
	ExpiresAt                int64  `json:"expires_at"`
	Attempts                 int    `json:"attempts"`
	MaxAttempts              int    `json:"max_attempts"`
	Version                  int    `json:"version"`
	PendingContextCiphertext string `json:"pending_context_ciphertext"`
}

type PlatformSiteChallengeResult struct {
	ChallengeID       string                       `json:"challenge_id"`
	Status            string                       `json:"status"`
	ExpiresAt         int64                        `json:"expires_at"`
	AttemptsRemaining int                          `json:"attempts_remaining"`
	SyncStatus        string                       `json:"sync_status,omitempty"`
	SyncStages        model.PlatformSiteSyncStages `json:"sync_stages,omitempty"`
}

type PlatformSiteWaitingVerificationError struct {
	Result *PlatformSiteChallengeResult
}

func (err *PlatformSiteWaitingVerificationError) Error() string {
	return "平台站点同步等待上游交互验证"
}

func (err *PlatformSiteWaitingVerificationError) Unwrap() error {
	return ErrPlatformSiteVerification
}

type platformSiteChallengeLock struct {
	mutex sync.Mutex
}

var (
	platformSiteChallengeCache = cachex.NewHybridCache[PlatformSiteChallenge](
		cachex.HybridCacheConfig[PlatformSiteChallenge]{
			Namespace:  cachex.Namespace("platform-site-challenge"),
			Redis:      common.RDB,
			RedisCodec: cachex.JSONCodec[PlatformSiteChallenge]{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, PlatformSiteChallenge] {
				return hot.NewHotCache[string, PlatformSiteChallenge](hot.LRU, 256).
					WithTTL(platformSiteChallengeTTL).
					WithJanitor().
					Build()
			},
		},
	)
	platformSiteChallengeIndexCache = cachex.NewHybridCache[string](
		cachex.HybridCacheConfig[string]{
			Namespace:  cachex.Namespace("platform-site-challenge-index"),
			Redis:      common.RDB,
			RedisCodec: cachex.StringCodec{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, string] {
				return hot.NewHotCache[string, string](hot.LRU, 256).
					WithTTL(platformSiteChallengeTTL).
					WithJanitor().
					Build()
			},
		},
	)
	platformSiteChallengeConsumedCache = cachex.NewHybridCache[bool](
		cachex.HybridCacheConfig[bool]{
			Namespace:  cachex.Namespace("platform-site-challenge-consumed"),
			Redis:      common.RDB,
			RedisCodec: cachex.JSONCodec[bool]{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, bool] {
				return hot.NewHotCache[string, bool](hot.LRU, 256).
					WithTTL(platformSiteChallengeTTL).
					WithJanitor().
					Build()
			},
		},
	)
	platformSiteChallengeExpiredCache = cachex.NewHybridCache[bool](
		cachex.HybridCacheConfig[bool]{
			Namespace:  cachex.Namespace("platform-site-challenge-expired"),
			Redis:      common.RDB,
			RedisCodec: cachex.JSONCodec[bool]{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, bool] {
				return hot.NewHotCache[string, bool](hot.LRU, 256).
					WithTTL(platformSiteChallengeTTL).
					WithJanitor().
					Build()
			},
		},
	)
	platformSiteChallengeLocks sync.Map
)

func CreatePlatformSiteChallenge(
	channelID int,
	platform string,
	baseURL string,
	origin string,
	createdBy int,
	source string,
	pending PlatformSitePendingContext,
) (*PlatformSiteChallengeResult, error) {
	if channelID <= 0 {
		return nil, challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 渠道无效",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	if source != platformSiteChallengeManual && source != platformSiteChallengeBackground {
		return nil, challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 来源无效",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	normalizedPlatform := strings.ToLower(strings.TrimSpace(platform))
	if normalizedPlatform != model.PlatformNewAPI && normalizedPlatform != model.PlatformSub2API {
		return nil, challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 平台无效",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	normalizedBaseURL, err := normalizePlatformSiteURL(baseURL)
	if err != nil {
		return nil, challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 地址无效",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	if err := validatePlatformSiteURL(normalizedBaseURL); err != nil {
		return nil, challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 地址不受信任",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	normalizedOrigin := strings.TrimRight(strings.TrimSpace(origin), "/")
	expectedOrigin, originErr := platformSiteOrigin(normalizedBaseURL)
	if originErr != nil {
		return nil, challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 来源无效",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	if normalizedOrigin == "" {
		normalizedOrigin = expectedOrigin
	}
	if !strings.EqualFold(normalizedOrigin, expectedOrigin) {
		return nil, challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 来源与站点不匹配",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	switch pending.Kind {
	case "newapi_cookie":
		if len(pending.Cookies) == 0 {
			return nil, challengeError(
				PlatformSiteChallengeCodeInfrastructure,
				"上游交互验证 Cookie 不可用，请重新使用浏览器采集登录态",
				ErrPlatformSiteVerification,
			)
		}
	case "sub2api_temp_token":
		if strings.TrimSpace(pending.TempToken) == "" {
			return nil, challengeError(
				PlatformSiteChallengeCodeInfrastructure,
				"上游交互验证临时令牌不可用，请重新同步",
				ErrPlatformSiteVerification,
			)
		}
	default:
		return nil, challengeError(
			PlatformSiteChallengeCodeInfrastructure,
			"上游交互验证上下文不可用，请重新使用浏览器采集登录态",
			ErrPlatformSiteVerification,
		)
	}
	plaintext, err := common.Marshal(pending)
	if err != nil {
		return nil, fmt.Errorf("编码平台同步 Challenge 上下文失败")
	}
	ciphertext, err := common.EncryptUpstreamCredential(string(plaintext))
	if err != nil {
		return nil, fmt.Errorf("加密平台同步 Challenge 上下文失败")
	}

	_, release, lockErr := acquirePlatformSiteChallengeLock(
		context.Background(),
		fmt.Sprintf("channel:%d", channelID),
	)
	if lockErr != nil {
		return nil, lockErr
	}
	defer release()

	existing, found, getErr := getPlatformSiteChallengeByChannel(channelID)
	if getErr != nil {
		return nil, fmt.Errorf("读取平台同步 Challenge 失败")
	}
	if found {
		if existing.Platform == normalizedPlatform &&
			existing.BaseURL == normalizedBaseURL &&
			strings.EqualFold(existing.Origin, normalizedOrigin) {
			result := platformSiteChallengeResult(existing)
			return &result, nil
		}
		_ = deletePlatformSiteChallenge(existing)
	}
	now := common.GetTimestamp()
	record := PlatformSiteChallenge{
		ChallengeID:              common.GetUUID(),
		ChannelID:                channelID,
		Platform:                 normalizedPlatform,
		BaseURL:                  normalizedBaseURL,
		Origin:                   normalizedOrigin,
		CreatedBy:                createdBy,
		Source:                   source,
		CreatedAt:                now,
		ExpiresAt:                time.Now().Add(platformSiteChallengeTTL).Unix(),
		MaxAttempts:              platformSiteChallengeMaxTries,
		Version:                  1,
		PendingContextCiphertext: ciphertext,
	}
	if err := platformSiteChallengeCache.SetWithTTL(
		record.ChallengeID,
		record,
		platformSiteChallengeTTL,
	); err != nil {
		return nil, fmt.Errorf("保存平台同步 Challenge 失败")
	}
	if err := platformSiteChallengeIndexCache.SetWithTTL(
		challengeChannelIndexKey(channelID),
		record.ChallengeID,
		platformSiteChallengeTTL,
	); err != nil {
		_ = deletePlatformSiteChallenge(record)
		return nil, fmt.Errorf("保存平台同步 Challenge 索引失败")
	}
	result := platformSiteChallengeResult(record)
	return &result, nil
}

func GetPlatformSiteChallengeStatus(channelID int) (*PlatformSiteChallengeResult, bool) {
	record, found, err := getPlatformSiteChallengeByChannel(channelID)
	if err != nil || !found {
		return nil, false
	}
	result := platformSiteChallengeResult(record)
	return &result, true
}

func SubmitPlatformSiteChallenge(
	ctx context.Context,
	userID int,
	channelID int,
	challengeID string,
	code string,
) (*PlatformSiteChallengeResult, error) {
	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return nil, challengeError(
			PlatformSiteChallengeCodeNotFound,
			"平台同步 Challenge ID 不能为空",
			ErrPlatformSiteChallengeNotFound,
		)
	}
	code = strings.TrimSpace(code)
	if !platformSiteChallengeCodePattern.MatchString(code) {
		return nil, challengeError(
			PlatformSiteChallengeCodeInvalidCode,
			"一次性验证码格式无效",
			ErrPlatformSiteChallengeInvalidCode,
		)
	}
	record, found, err := getPlatformSiteChallengeByID(challengeID)
	if err != nil {
		return nil, err
	}
	if !found {
		if _, consumed, consumedErr := platformSiteChallengeConsumedCache.Get(challengeID); consumedErr == nil && consumed {
			return nil, challengeError(
				PlatformSiteChallengeCodeConsumed,
				"平台同步 Challenge 已消费，请重新同步",
				ErrPlatformSiteChallengeConsumed,
			)
		}
		if _, expired, expiredErr := platformSiteChallengeExpiredCache.Get(challengeID); expiredErr == nil && expired {
			return nil, challengeError(
				PlatformSiteChallengeCodeExpired,
				"平台同步 Challenge 已过期，请重新同步",
				ErrPlatformSiteChallengeExpired,
			)
		}
		return nil, challengeError(
			PlatformSiteChallengeCodeNotFound,
			"平台同步 Challenge 不存在或已过期",
			ErrPlatformSiteChallengeNotFound,
		)
	}
	_, release, err := acquirePlatformSiteChallengeLock(
		ctx,
		fmt.Sprintf("challenge:%s", challengeID),
	)
	if err != nil {
		return nil, err
	}
	defer release()

	record, found, err = getPlatformSiteChallengeByID(challengeID)
	if err != nil {
		return nil, err
	}
	if !found {
		if _, consumed, consumedErr := platformSiteChallengeConsumedCache.Get(challengeID); consumedErr == nil && consumed {
			return nil, challengeError(
				PlatformSiteChallengeCodeConsumed,
				"平台同步 Challenge 已消费，请重新同步",
				ErrPlatformSiteChallengeConsumed,
			)
		}
		if _, expired, expiredErr := platformSiteChallengeExpiredCache.Get(challengeID); expiredErr == nil && expired {
			return nil, challengeError(
				PlatformSiteChallengeCodeExpired,
				"平台同步 Challenge 已过期，请重新同步",
				ErrPlatformSiteChallengeExpired,
			)
		}
		return nil, challengeError(
			PlatformSiteChallengeCodeNotFound,
			"平台同步 Challenge 不存在或已过期",
			ErrPlatformSiteChallengeNotFound,
		)
	}
	if record.ChannelID != channelID {
		return nil, challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 与当前请求不匹配",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	if record.ExpiresAt <= time.Now().Unix() {
		_ = platformSiteChallengeExpiredCache.SetWithTTL(
			record.ChallengeID,
			true,
			platformSiteChallengeTTL,
		)
		_ = deletePlatformSiteChallenge(record)
		return nil, challengeError(
			PlatformSiteChallengeCodeExpired,
			"平台同步 Challenge 已过期，请重新同步",
			ErrPlatformSiteChallengeExpired,
		)
	}
	if record.Source == platformSiteChallengeManual && record.CreatedBy != userID {
		return nil, challengeError(
			PlatformSiteChallengeCodePermission,
			"无权提交该平台同步 Challenge",
			ErrPlatformSiteChallengePermission,
		)
	}
	if record.Attempts >= record.MaxAttempts {
		_ = deletePlatformSiteChallenge(record)
		return nil, challengeError(
			PlatformSiteChallengeCodeAttempts,
			"平台同步 Challenge 尝试次数已耗尽，请重新同步",
			ErrPlatformSiteChallengeAttempts,
		)
	}

	var account model.PlatformSiteAccount
	if err := model.DB.Where("channel_id = ?", channelID).First(&account).Error; err != nil {
		return nil, err
	}
	if err := validatePlatformSiteChallengeTarget(record, &account); err != nil {
		return nil, err
	}
	var pending PlatformSitePendingContext
	plaintext, err := common.DecryptUpstreamCredential(record.PendingContextCiphertext)
	if err != nil || common.Unmarshal([]byte(plaintext), &pending) != nil {
		return nil, challengeError(
			PlatformSiteChallengeCodeInfrastructure,
			"平台同步 Challenge 上下文不可用，请重新同步",
			ErrPlatformSiteVerification,
		)
	}
	credential, err := model.DecryptPlatformSiteCredential(account.CredentialCiphertext)
	if err != nil {
		return nil, err
	}
	credential.AuthType = account.AuthType
	adapter, err := adapterForPlatform(account.Platform)
	if err != nil {
		return nil, err
	}
	session, verificationErr := adapter.CompleteVerification(
		ctx,
		account.BaseURL,
		credential,
		pending,
		code,
	)
	if verificationErr != nil {
		if errors.Is(verificationErr, ErrPlatformSiteVerificationCode) {
			record.Attempts++
			if record.Attempts >= record.MaxAttempts {
				_ = deletePlatformSiteChallenge(record)
				return nil, challengeError(
					PlatformSiteChallengeCodeAttempts,
					"验证码错误次数已耗尽，请重新同步",
					ErrPlatformSiteChallengeAttempts,
				)
			}
			if err := savePlatformSiteChallenge(record); err != nil {
				return nil, err
			}
			result := platformSiteChallengeResult(record)
			return &result, challengeError(
				PlatformSiteChallengeCodeInvalidCode,
				"验证码错误，请重试",
				ErrPlatformSiteChallengeInvalidCode,
			)
		}
		return nil, verificationErr
	}
	if err := consumePlatformSiteChallenge(record); err != nil {
		return nil, challengeError(
			PlatformSiteChallengeCodeInfrastructure,
			"平台同步 Challenge 消费失败，请重新同步",
			err,
		)
	}
	release()
	release = func() {}
	if session != nil && session.CredentialUpdate != nil {
		if err := persistPlatformSiteCredential(&account, *session.CredentialUpdate); err != nil {
			return nil, err
		}
	}
	if err := model.DB.Model(&account).Updates(map[string]any{
		"sync_status":     model.UpstreamSiteSyncIdle,
		"last_sync_error": "",
	}).Error; err != nil {
		return nil, err
	}
	if err := syncPlatformSite(ctx, channelID); err != nil {
		return nil, err
	}
	var updated model.PlatformSiteAccount
	if err := model.DB.Where("channel_id = ?", channelID).First(&updated).Error; err != nil {
		return nil, err
	}
	stages, _ := model.DecodePlatformSiteSyncStages(updated.SyncStages)
	return &PlatformSiteChallengeResult{
		ChallengeID:       record.ChallengeID,
		Status:            platformSiteChallengeStatusSuccess,
		ExpiresAt:         record.ExpiresAt,
		AttemptsRemaining: max(0, record.MaxAttempts-record.Attempts),
		SyncStatus:        updated.SyncStatus,
		SyncStages:        stages,
	}, nil
}

func HandlePlatformSiteVerification(
	ctx context.Context,
	account *model.PlatformSiteAccount,
	verificationErr error,
	source string,
	createdBy int,
) (*PlatformSiteChallengeResult, error) {
	if account == nil {
		return nil, verificationErr
	}
	var required *PlatformSiteVerificationRequired
	if !errors.As(verificationErr, &required) {
		return nil, verificationErr
	}
	origin, err := platformSiteOrigin(account.BaseURL)
	if err != nil {
		return nil, verificationErr
	}
	result, err := CreatePlatformSiteChallenge(
		account.ChannelID,
		account.Platform,
		account.BaseURL,
		origin,
		createdBy,
		source,
		required.Pending,
	)
	if err != nil {
		if errors.Is(err, ErrPlatformSiteVerification) {
			return &PlatformSiteChallengeResult{
				Status:    platformSiteChallengeStatusWaiting,
				ExpiresAt: time.Now().Add(platformSiteChallengeTTL).Unix(),
			}, nil
		}
		return nil, err
	}
	_ = ctx
	return result, nil
}

func validatePlatformSiteChallengeTarget(
	challenge PlatformSiteChallenge,
	account *model.PlatformSiteAccount,
) error {
	if account == nil ||
		challenge.ChannelID != account.ChannelID ||
		!strings.EqualFold(challenge.Platform, account.Platform) {
		return challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 与当前渠道不匹配",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	baseURL, err := normalizePlatformSiteURL(account.BaseURL)
	if err != nil || baseURL != challenge.BaseURL {
		return challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 与当前站点地址不匹配",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	origin, err := platformSiteOrigin(baseURL)
	if err != nil || !strings.EqualFold(origin, challenge.Origin) {
		return challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 与当前站点来源不匹配",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	return nil
}

func getPlatformSiteChallengeByChannel(channelID int) (PlatformSiteChallenge, bool, error) {
	index, found, err := platformSiteChallengeIndexCache.Get(challengeChannelIndexKey(channelID))
	if err != nil {
		return PlatformSiteChallenge{}, false, err
	}
	if !found || strings.TrimSpace(index) == "" {
		return PlatformSiteChallenge{}, false, nil
	}
	return getPlatformSiteChallengeByID(index)
}

func getPlatformSiteChallengeByID(challengeID string) (PlatformSiteChallenge, bool, error) {
	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return PlatformSiteChallenge{}, false, nil
	}
	record, found, err := platformSiteChallengeCache.Get(challengeID)
	if err != nil {
		return PlatformSiteChallenge{}, false, err
	}
	if !found || record.ChallengeID == "" {
		return PlatformSiteChallenge{}, false, nil
	}
	if record.ExpiresAt <= time.Now().Unix() {
		_ = platformSiteChallengeExpiredCache.SetWithTTL(
			record.ChallengeID,
			true,
			platformSiteChallengeTTL,
		)
		_ = deletePlatformSiteChallenge(record)
		return PlatformSiteChallenge{}, false, nil
	}
	return record, true, nil
}

func savePlatformSiteChallenge(record PlatformSiteChallenge) error {
	remaining := time.Until(time.Unix(record.ExpiresAt, 0))
	if remaining <= 0 {
		return challengeError(
			PlatformSiteChallengeCodeExpired,
			"平台同步 Challenge 已过期，请重新同步",
			ErrPlatformSiteChallengeExpired,
		)
	}
	return platformSiteChallengeCache.SetWithTTL(record.ChallengeID, record, remaining)
}

func deletePlatformSiteChallenge(record PlatformSiteChallenge) error {
	_, firstErr := platformSiteChallengeCache.DeleteMany([]string{record.ChallengeID})
	_, secondErr := platformSiteChallengeIndexCache.DeleteMany([]string{challengeChannelIndexKey(record.ChannelID)})
	if firstErr != nil {
		return firstErr
	}
	return secondErr
}

func consumePlatformSiteChallenge(record PlatformSiteChallenge) error {
	if err := platformSiteChallengeConsumedCache.SetWithTTL(
		record.ChallengeID,
		true,
		platformSiteChallengeTTL,
	); err != nil {
		return err
	}
	return deletePlatformSiteChallenge(record)
}

func challengeChannelIndexKey(channelID int) string {
	return fmt.Sprintf("channel:%d", channelID)
}

func platformSiteChallengeResult(record PlatformSiteChallenge) PlatformSiteChallengeResult {
	remaining := record.MaxAttempts - record.Attempts
	if remaining < 0 {
		remaining = 0
	}
	return PlatformSiteChallengeResult{
		ChallengeID:       record.ChallengeID,
		Status:            platformSiteChallengeStatusWaiting,
		ExpiresAt:         record.ExpiresAt,
		AttemptsRemaining: remaining,
	}
}

func challengeError(code, message string, cause error) error {
	return &PlatformSiteChallengeError{Code: code, Message: message, Cause: cause}
}

func acquirePlatformSiteChallengeLock(
	ctx context.Context,
	lockIdentity string,
) (PlatformSiteChallenge, func(), error) {
	lockIdentity = strings.TrimSpace(lockIdentity)
	if lockIdentity == "" {
		return PlatformSiteChallenge{}, func() {}, challengeError(
			PlatformSiteChallengeCodeMismatch,
			"平台同步 Challenge 锁标识无效",
			ErrPlatformSiteChallengeMismatch,
		)
	}
	lockKey := "platform-site-challenge-lock:" + lockIdentity
	lockValue, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return PlatformSiteChallenge{}, func() {}, err
	}
	if common.RedisEnabled && common.RDB != nil {
		lockContext, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		acquired, setErr := common.RDB.SetNX(
			lockContext,
			lockKey,
			lockValue,
			platformSiteChallengeLockTTL,
		).Result()
		if setErr == nil && !acquired {
			return PlatformSiteChallenge{}, func() {}, challengeError(
				PlatformSiteChallengeCodeBusy,
				"平台同步 Challenge 正在处理中，请稍后重试",
				ErrPlatformSiteChallengeBusy,
			)
		}
		if setErr == nil && acquired {
			return PlatformSiteChallenge{}, func() {
				releaseContext, releaseCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer releaseCancel()
				_, _ = common.RDB.Eval(releaseContext, `
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
end
return 0
`, []string{lockKey}, lockValue).Result()
			}, nil
		}
	}
	value, _ := platformSiteChallengeLocks.LoadOrStore(
		lockIdentity,
		&platformSiteChallengeLock{},
	)
	mutex := value.(*platformSiteChallengeLock)
	mutex.mutex.Lock()
	return PlatformSiteChallenge{}, mutex.mutex.Unlock, nil
}

func clearPlatformSiteChallengeForTests() {
	_ = platformSiteChallengeCache.Purge()
	_ = platformSiteChallengeIndexCache.Purge()
	_ = platformSiteChallengeConsumedCache.Purge()
	_ = platformSiteChallengeExpiredCache.Purge()
}
