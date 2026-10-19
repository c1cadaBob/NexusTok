package model

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"

	"gorm.io/gorm"
)

const routingKeyHealthErrorSummaryMaxLength = 240

var routingKeyHealthSensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization["']?\s*[:=]\s*(?:bearer\s+)?)[^\s,;"}]+`),
	regexp.MustCompile(`(?i)(\b(?:bearer|token)\s+)[^\s,;]+`),
	regexp.MustCompile(`(?i)((?:cookie|set-cookie)["']?\s*[:=]\s*)[^\r\n}]+`),
	regexp.MustCompile(`(?i)((?:access[_-]?token|refresh[_-]?token|id[_-]?token|api[_-]?key|token|secret|password|passwd|credential)["']?\s*[:=]\s*["']?)[^"',;\s}]+`),
	regexp.MustCompile(`(?i)(\b)sk-[a-z0-9_-]{8,}\b`),
	regexp.MustCompile(`(\b)eyJ[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\b`),
	regexp.MustCompile(`(?i)(\b)AIza[a-z0-9_-]{20,}\b`),
	regexp.MustCompile(`(?i)(\b)AKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)(\b)(?:ghp|github_pat)_[a-z0-9_]{12,}\b`),
}

const (
	RoutingKeyHealthDisabled = "disabled"
	RoutingKeyHealthEnabled  = "enabled"
	RoutingKeyHealthNormal   = "normal"
	RoutingKeyHealthDegraded = "degraded"
	RoutingKeyHealthInvalid  = "invalid"

	routingKeyHealthWindowSize     = 5
	routingKeyHealthFreshDuration  = 24 * time.Hour
	routingKeyHealthNormalLatency  = int64(10_000)
	routingKeyHealthInvalidSuccess = 1

	RoutingKeyRecoveryPhaseProbing    = "probing"
	RoutingKeyRecoveryPhaseConfirming = "confirming"
	RoutingKeyRecoveryConfirmations   = 5
	RoutingKeyRecoveryRetryInterval   = 30 * time.Minute
	RoutingKeyRecoveryConfirmInterval = 5 * time.Minute
)

type RoutingKeyHealth struct {
	RoutingKeyID               uint   `json:"routing_key_id" gorm:"primaryKey;autoIncrement:false"`
	ChannelID                  int    `json:"channel_id" gorm:"not null;index"`
	Source                     string `json:"source" gorm:"type:varchar(32);not null;index"`
	SamplesJSON                string `json:"-" gorm:"type:text;not null"`
	SampleCount                int    `json:"sample_count" gorm:"index"`
	SuccessCount               int    `json:"success_count" gorm:"index"`
	MaxFirstLatencyMs          int64  `json:"max_first_latency_ms" gorm:"bigint"`
	LastFirstLatencyMs         int64  `json:"last_first_latency_ms" gorm:"bigint"`
	LastSampleAt               int64  `json:"last_sample_at" gorm:"bigint;index"`
	UpdatedAt                  int64  `json:"updated_at" gorm:"bigint;index"`
	RecoveryPhase              string `json:"recovery_phase,omitempty" gorm:"type:varchar(32);index"`
	RecoveryNextTestAt         int64  `json:"recovery_next_test_at,omitempty" gorm:"bigint;index"`
	RecoveryLastTestAt         int64  `json:"recovery_last_test_at,omitempty" gorm:"bigint;index"`
	RecoveryConfirmationsDone  int    `json:"recovery_confirmations_done,omitempty"`
	RecoveryConfirmationsTotal int    `json:"recovery_confirmations_total,omitempty"`
}

type RoutingKeyHealthSample struct {
	Success        bool   `json:"success"`
	FirstLatencyMs int64  `json:"first_latency_ms"`
	CreatedAt      int64  `json:"created_at"`
	ErrorSummary   string `json:"error_summary,omitempty"`
}

type RoutingKeyHealthSummary struct {
	Status         string                   `json:"status"`
	Reason         string                   `json:"reason,omitempty"`
	SampleCount    int                      `json:"sample_count"`
	SuccessCount   int                      `json:"success_count"`
	FirstLatencyMs int64                    `json:"first_latency_ms,omitempty"`
	LastSampleAt   int64                    `json:"last_sample_at,omitempty"`
	HealthSamples  []RoutingKeyHealthSample `json:"health_samples,omitempty"`
}

type RoutingKeyRecoveryState struct {
	Phase              string `json:"recovery_phase,omitempty"`
	NextTestAt         int64  `json:"recovery_next_test_at,omitempty"`
	LastTestAt         int64  `json:"recovery_last_test_at,omitempty"`
	ConfirmationsDone  int    `json:"recovery_confirmations_done,omitempty"`
	ConfirmationsTotal int    `json:"recovery_confirmations_total,omitempty"`
}

type RoutingKeyRecoveryCandidate struct {
	RoutingKeyID       uint
	ChannelID          int
	Source             string
	Status             int
	AvailabilityReason string
	Channel            *Channel
}

func routingKeyHealthTableReady(tx *gorm.DB) bool {
	return tx != nil && tx.Migrator().HasTable(&RoutingKeyHealth{})
}

func healthSummaryFromBase(status int, availabilityReason string) (RoutingKeyHealthSummary, bool) {
	switch status {
	case common.ChannelStatusManuallyDisabled:
		return RoutingKeyHealthSummary{Status: RoutingKeyHealthDisabled, Reason: UpstreamAvailabilityManualDisabled}, true
	case common.ChannelStatusEnabled, 0:
	default:
		reason := strings.TrimSpace(availabilityReason)
		if reason == "" {
			reason = UpstreamAvailabilityUpstreamDisabled
		}
		return RoutingKeyHealthSummary{Status: RoutingKeyHealthInvalid, Reason: reason}, true
	}

	reason := strings.TrimSpace(availabilityReason)
	if reason != "" && reason != UpstreamAvailabilityRoutable {
		return RoutingKeyHealthSummary{Status: RoutingKeyHealthInvalid, Reason: reason}, true
	}
	return RoutingKeyHealthSummary{}, false
}

func routingKeyHealthSummaryFromRecord(
	health *RoutingKeyHealth,
	now time.Time,
	includeSamples bool,
) RoutingKeyHealthSummary {
	if health == nil {
		return RoutingKeyHealthSummary{Status: RoutingKeyHealthEnabled, Reason: "not_enough_samples"}
	}
	var samples []RoutingKeyHealthSample
	if includeSamples {
		samples, _ = healthSamplesFromJSON(health.SamplesJSON)
		if len(samples) > routingKeyHealthWindowSize {
			samples = samples[len(samples)-routingKeyHealthWindowSize:]
		}
	}
	if health.SampleCount < routingKeyHealthWindowSize {
		return RoutingKeyHealthSummary{
			Status:        RoutingKeyHealthEnabled,
			Reason:        "not_enough_samples",
			HealthSamples: samples,
		}
	}
	if health.LastSampleAt <= 0 ||
		now.Sub(time.Unix(health.LastSampleAt, 0)) > routingKeyHealthFreshDuration {
		return RoutingKeyHealthSummary{
			Status:         RoutingKeyHealthEnabled,
			Reason:         "stale_samples",
			SampleCount:    health.SampleCount,
			SuccessCount:   health.SuccessCount,
			FirstLatencyMs: health.LastFirstLatencyMs,
			LastSampleAt:   health.LastSampleAt,
			HealthSamples:  samples,
		}
	}

	status := RoutingKeyHealthDegraded
	reason := "recent_degraded"
	if health.SuccessCount <= routingKeyHealthInvalidSuccess {
		status = RoutingKeyHealthInvalid
		reason = "recent_failures"
	} else if health.SuccessCount == routingKeyHealthWindowSize &&
		health.MaxFirstLatencyMs < routingKeyHealthNormalLatency {
		status = RoutingKeyHealthNormal
		reason = ""
	} else if health.SuccessCount == routingKeyHealthWindowSize {
		reason = "high_first_latency"
	}
	return RoutingKeyHealthSummary{
		Status:         status,
		Reason:         reason,
		SampleCount:    health.SampleCount,
		SuccessCount:   health.SuccessCount,
		FirstLatencyMs: health.LastFirstLatencyMs,
		LastSampleAt:   health.LastSampleAt,
		HealthSamples:  samples,
	}
}

func GetRoutingKeyHealthSummary(
	routingKeyID uint,
	status int,
	availabilityReason string,
	now time.Time,
) RoutingKeyHealthSummary {
	return getRoutingKeyHealthSummary(routingKeyID, status, availabilityReason, now, true)
}

func getRoutingKeyHealthSummary(
	routingKeyID uint,
	status int,
	availabilityReason string,
	now time.Time,
	includeSamples bool,
) RoutingKeyHealthSummary {
	baseSummary, hasBaseSummary := healthSummaryFromBase(status, availabilityReason)
	if routingKeyID == 0 || DB == nil || !routingKeyHealthTableReady(DB) {
		if hasBaseSummary {
			return baseSummary
		}
		return RoutingKeyHealthSummary{Status: RoutingKeyHealthEnabled, Reason: "no_health_record"}
	}
	var health RoutingKeyHealth
	query := DB
	if !includeSamples {
		query = query.Omit("SamplesJSON")
	}
	if err := query.Where("routing_key_id = ?", routingKeyID).First(&health).Error; err != nil {
		if hasBaseSummary {
			return baseSummary
		}
		return RoutingKeyHealthSummary{Status: RoutingKeyHealthEnabled, Reason: "no_health_record"}
	}
	if hasBaseSummary {
		if includeSamples {
			baseSummary.HealthSamples, _ = healthSamplesFromJSON(health.SamplesJSON)
			if len(baseSummary.HealthSamples) > routingKeyHealthWindowSize {
				baseSummary.HealthSamples = baseSummary.HealthSamples[len(baseSummary.HealthSamples)-routingKeyHealthWindowSize:]
			}
		}
		return baseSummary
	}
	return routingKeyHealthSummaryFromRecord(&health, now, includeSamples)
}

func RoutingKeyHealthAllowsRouting(
	routingKeyID uint,
	status int,
	availabilityReason string,
	now time.Time,
) bool {
	summary := getRoutingKeyHealthSummary(routingKeyID, status, availabilityReason, now, false)
	return summary.Status != RoutingKeyHealthDisabled && summary.Status != RoutingKeyHealthInvalid
}

func GetRoutingKeyRecoveryState(routingKeyID uint) RoutingKeyRecoveryState {
	if routingKeyID == 0 || DB == nil || !routingKeyHealthTableReady(DB) {
		return RoutingKeyRecoveryState{}
	}
	var health RoutingKeyHealth
	if err := DB.Where("routing_key_id = ?", routingKeyID).First(&health).Error; err != nil {
		return RoutingKeyRecoveryState{}
	}
	return RoutingKeyRecoveryState{
		Phase:              health.RecoveryPhase,
		NextTestAt:         health.RecoveryNextTestAt,
		LastTestAt:         health.RecoveryLastTestAt,
		ConfirmationsDone:  health.RecoveryConfirmationsDone,
		ConfirmationsTotal: health.RecoveryConfirmationsTotal,
	}
}

func healthRecoveryStateDue(health *RoutingKeyHealth, now int64) bool {
	return health == nil || health.RecoveryNextTestAt <= 0 || health.RecoveryNextTestAt <= now
}

// ListDueRoutingKeyRecoveryCandidates 只返回已到期的恢复测试候选。手动禁用的密钥和
// 渠道始终排除；静态可用性由调用方继续判断，以便不可测试的密钥也能记录失败样本。
func ListDueRoutingKeyRecoveryCandidates(now time.Time) ([]RoutingKeyRecoveryCandidate, error) {
	if DB == nil || !routingKeyHealthTableReady(DB) {
		return nil, nil
	}
	if now.IsZero() {
		now = time.Now()
	}

	var routingKeys []RoutingKey
	if err := DB.Order("id ASC").Find(&routingKeys).Error; err != nil {
		return nil, err
	}
	candidates := make([]RoutingKeyRecoveryCandidate, 0)
	nowUnix := now.Unix()
	for index := range routingKeys {
		routingKey := &routingKeys[index]
		var channel Channel
		if err := DB.First(&channel, routingKey.ChannelID).Error; err != nil {
			continue
		}
		if channel.Status == common.ChannelStatusManuallyDisabled {
			continue
		}

		status := common.ChannelStatusEnabled
		availabilityReason := UpstreamAvailabilityRoutable
		switch routingKey.Source {
		case RoutingKeySourceKeyChannel:
			var key ChannelKey
			if err := DB.Where(
				"routing_key_id = ? AND channel_id = ?",
				routingKey.ID,
				routingKey.ChannelID,
			).First(&key).Error; err != nil {
				continue
			}
			status = key.Status
			if status == 0 {
				status = common.ChannelStatusEnabled
			}
			if status == common.ChannelStatusManuallyDisabled {
				continue
			}
			if status != common.ChannelStatusEnabled &&
				status != common.ChannelStatusAutoDisabled {
				continue
			}
		case RoutingKeySourcePlatformSite:
			var key UpstreamKey
			if err := DB.Where(
				"routing_key_id = ? AND channel_id = ?",
				routingKey.ID,
				routingKey.ChannelID,
			).First(&key).Error; err != nil {
				continue
			}
			status = key.Status
			if status == 0 {
				status = common.ChannelStatusEnabled
			}
			if status == UpstreamKeyStatusManualDisabled ||
				status == UpstreamKeyStatusMissing ||
				(status != UpstreamKeyStatusEnabled &&
					status != UpstreamKeyStatusAutoDisabled) {
				continue
			}
			availabilityKey := key
			if status == UpstreamKeyStatusAutoDisabled {
				availabilityKey.Status = UpstreamKeyStatusEnabled
			}
			availabilityReason = availabilityKey.AvailabilityReason(now)
		default:
			continue
		}

		var health *RoutingKeyHealth
		var savedHealth RoutingKeyHealth
		if err := DB.Where("routing_key_id = ?", routingKey.ID).First(&savedHealth).Error; err == nil {
			health = &savedHealth
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		summaryStatus := status
		if status == common.ChannelStatusAutoDisabled {
			summaryStatus = common.ChannelStatusEnabled
		}
		summary := GetRoutingKeyHealthSummary(
			routingKey.ID,
			summaryStatus,
			availabilityReason,
			now,
		)
		if summary.Status != RoutingKeyHealthInvalid &&
			status != common.ChannelStatusAutoDisabled &&
			(health == nil || health.RecoveryPhase != RoutingKeyRecoveryPhaseConfirming) {
			continue
		}
		if !healthRecoveryStateDue(health, nowUnix) {
			continue
		}
		candidates = append(candidates, RoutingKeyRecoveryCandidate{
			RoutingKeyID:       routingKey.ID,
			ChannelID:          routingKey.ChannelID,
			Source:             routingKey.Source,
			Status:             status,
			AvailabilityReason: availabilityReason,
			Channel:            &channel,
		})
	}
	return candidates, nil
}

func healthSamplesFromJSON(raw string) ([]RoutingKeyHealthSample, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var samples []RoutingKeyHealthSample
	if err := common.Unmarshal([]byte(raw), &samples); err != nil {
		return nil, err
	}
	return samples, nil
}

func summarizeHealthSamples(samples []RoutingKeyHealthSample) (int, int, int64, int64, int64) {
	successCount := 0
	maxFirstLatencyMs := int64(0)
	lastFirstLatencyMs := int64(0)
	lastSampleAt := int64(0)
	for _, sample := range samples {
		if sample.Success {
			successCount++
		}
		if sample.FirstLatencyMs > maxFirstLatencyMs {
			maxFirstLatencyMs = sample.FirstLatencyMs
		}
		lastFirstLatencyMs = sample.FirstLatencyMs
		lastSampleAt = sample.CreatedAt
	}
	return len(samples), successCount, maxFirstLatencyMs, lastFirstLatencyMs, lastSampleAt
}

func sanitizeRoutingKeyHealthErrorSummary(summary string) string {
	text := strings.Join(strings.Fields(summary), " ")
	text = common.MaskSensitiveInfo(text)
	for _, pattern := range routingKeyHealthSensitivePatterns {
		text = pattern.ReplaceAllString(text, "$1***")
	}
	text = strings.TrimSpace(text)
	if len([]rune(text)) <= routingKeyHealthErrorSummaryMaxLength {
		return text
	}
	return string([]rune(text)[:routingKeyHealthErrorSummaryMaxLength-1]) + "…"
}

func RecordRoutingKeyHealthSample(
	routingKeyID uint,
	channelID int,
	source string,
	success bool,
	firstLatencyMs int64,
	unixTime int64,
	errorSummary ...string,
) error {
	if routingKeyID == 0 || DB == nil {
		return nil
	}
	if unixTime <= 0 {
		unixTime = common.GetTimestamp()
	}
	if firstLatencyMs < 0 {
		firstLatencyMs = 0
	}
	sanitizedErrorSummary := ""
	if len(errorSummary) > 0 {
		sanitizedErrorSummary = sanitizeRoutingKeyHealthErrorSummary(errorSummary[0])
	}
	source = strings.TrimSpace(source)

	var lastErr error
	for range 2 {
		lastErr = DB.Transaction(func(tx *gorm.DB) error {
			if !routingKeyHealthTableReady(tx) {
				return nil
			}
			var health RoutingKeyHealth
			err := lockForUpdate(tx).Where("routing_key_id = ?", routingKeyID).First(&health).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			samples := make([]RoutingKeyHealthSample, 0, routingKeyHealthWindowSize)
			if err == nil {
				parsed, parseErr := healthSamplesFromJSON(health.SamplesJSON)
				if parseErr != nil {
					return parseErr
				}
				samples = append(samples, parsed...)
			}
			samples = append(samples, RoutingKeyHealthSample{
				Success:        success,
				FirstLatencyMs: firstLatencyMs,
				CreatedAt:      unixTime,
				ErrorSummary:   sanitizedErrorSummary,
			})
			if len(samples) > routingKeyHealthWindowSize {
				samples = samples[len(samples)-routingKeyHealthWindowSize:]
			}
			encoded, marshalErr := common.Marshal(samples)
			if marshalErr != nil {
				return marshalErr
			}
			sampleCount, successCount, maxFirstLatencyMs, lastFirstLatencyMs, lastSampleAt :=
				summarizeHealthSamples(samples)
			updates := map[string]any{
				"channel_id":            channelID,
				"source":                source,
				"samples_json":          string(encoded),
				"sample_count":          sampleCount,
				"success_count":         successCount,
				"max_first_latency_ms":  maxFirstLatencyMs,
				"last_first_latency_ms": lastFirstLatencyMs,
				"last_sample_at":        lastSampleAt,
				"updated_at":            unixTime,
			}
			if err == nil {
				return tx.Model(&RoutingKeyHealth{}).
					Where("routing_key_id = ?", routingKeyID).
					Updates(updates).Error
			}
			health = RoutingKeyHealth{
				RoutingKeyID:       routingKeyID,
				ChannelID:          channelID,
				Source:             source,
				SamplesJSON:        string(encoded),
				SampleCount:        sampleCount,
				SuccessCount:       successCount,
				MaxFirstLatencyMs:  maxFirstLatencyMs,
				LastFirstLatencyMs: lastFirstLatencyMs,
				LastSampleAt:       lastSampleAt,
				UpdatedAt:          unixTime,
			}
			return tx.Create(&health).Error
		})
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

// RecordRoutingKeyRecoverySample 将恢复请求作为普通健康样本记录，并推进恢复测试节奏。
// 只有健康样本重新满足路由条件时才恢复自动禁用状态，手动禁用不会被此函数修改。
func RecordRoutingKeyRecoverySample(
	routingKeyID uint,
	channelID int,
	source string,
	success bool,
	firstLatencyMs int64,
	now time.Time,
	errorSummary ...string,
) error {
	if routingKeyID == 0 || DB == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	nowUnix := now.Unix()
	if firstLatencyMs < 0 {
		firstLatencyMs = 0
	}
	sanitizedErrorSummary := ""
	if len(errorSummary) > 0 {
		sanitizedErrorSummary = sanitizeRoutingKeyHealthErrorSummary(errorSummary[0])
	}

	var lastErr error
	for range 2 {
		lastErr = DB.Transaction(func(tx *gorm.DB) error {
			if !routingKeyHealthTableReady(tx) {
				return nil
			}
			var health RoutingKeyHealth
			err := lockForUpdate(tx).
				Where("routing_key_id = ?", routingKeyID).
				First(&health).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			samples := make([]RoutingKeyHealthSample, 0, routingKeyHealthWindowSize)
			if err == nil {
				parsed, parseErr := healthSamplesFromJSON(health.SamplesJSON)
				if parseErr != nil {
					return parseErr
				}
				samples = append(samples, parsed...)
			}
			samples = append(samples, RoutingKeyHealthSample{
				Success:        success,
				FirstLatencyMs: firstLatencyMs,
				CreatedAt:      nowUnix,
				ErrorSummary:   sanitizedErrorSummary,
			})
			if len(samples) > routingKeyHealthWindowSize {
				samples = samples[len(samples)-routingKeyHealthWindowSize:]
			}
			encoded, marshalErr := common.Marshal(samples)
			if marshalErr != nil {
				return marshalErr
			}
			sampleCount, successCount, maxFirstLatencyMs, lastFirstLatencyMs, lastSampleAt :=
				summarizeHealthSamples(samples)

			phase := health.RecoveryPhase
			confirmationsDone := health.RecoveryConfirmationsDone
			confirmationsTotal := health.RecoveryConfirmationsTotal
			if confirmationsTotal <= 0 {
				confirmationsTotal = RoutingKeyRecoveryConfirmations
			}
			nextTestAt := nowUnix + int64(RoutingKeyRecoveryRetryInterval.Seconds())
			if phase == RoutingKeyRecoveryPhaseConfirming {
				confirmationsDone++
				if confirmationsDone < confirmationsTotal {
					nextTestAt = nowUnix + int64(RoutingKeyRecoveryConfirmInterval.Seconds())
				} else {
					confirmationsDone = confirmationsTotal
					phase = ""
					nextTestAt = 0
				}
			} else if success {
				phase = RoutingKeyRecoveryPhaseConfirming
				confirmationsDone = 0
				nextTestAt = nowUnix + int64(RoutingKeyRecoveryConfirmInterval.Seconds())
			} else {
				phase = RoutingKeyRecoveryPhaseProbing
				confirmationsDone = 0
			}

			values := map[string]any{
				"channel_id":                   channelID,
				"source":                       strings.TrimSpace(source),
				"samples_json":                 string(encoded),
				"sample_count":                 sampleCount,
				"success_count":                successCount,
				"max_first_latency_ms":         maxFirstLatencyMs,
				"last_first_latency_ms":        lastFirstLatencyMs,
				"last_sample_at":               lastSampleAt,
				"updated_at":                   nowUnix,
				"recovery_phase":               phase,
				"recovery_next_test_at":        nextTestAt,
				"recovery_last_test_at":        nowUnix,
				"recovery_confirmations_done":  confirmationsDone,
				"recovery_confirmations_total": confirmationsTotal,
			}
			if err == nil {
				if updateErr := tx.Model(&RoutingKeyHealth{}).
					Where("routing_key_id = ?", routingKeyID).
					Updates(values).Error; updateErr != nil {
					return updateErr
				}
			} else {
				health = RoutingKeyHealth{
					RoutingKeyID:               routingKeyID,
					ChannelID:                  channelID,
					Source:                     strings.TrimSpace(source),
					SamplesJSON:                string(encoded),
					SampleCount:                sampleCount,
					SuccessCount:               successCount,
					MaxFirstLatencyMs:          maxFirstLatencyMs,
					LastFirstLatencyMs:         lastFirstLatencyMs,
					LastSampleAt:               lastSampleAt,
					UpdatedAt:                  nowUnix,
					RecoveryPhase:              phase,
					RecoveryNextTestAt:         nextTestAt,
					RecoveryLastTestAt:         nowUnix,
					RecoveryConfirmationsDone:  confirmationsDone,
					RecoveryConfirmationsTotal: confirmationsTotal,
				}
				if createErr := tx.Create(&health).Error; createErr != nil {
					return createErr
				}
			}
			return nil
		})
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return lastErr
	}

	if err := restoreAutoDisabledRoutingKeyIfHealthy(routingKeyID, channelID, success, now); err != nil {
		return err
	}
	return ensureRoutingKeyRecoveryRetryIfStillInvalid(routingKeyID, channelID, now)
}

func restoreAutoDisabledRoutingKeyIfHealthy(routingKeyID uint, channelID int, success bool, now time.Time) error {
	if !success {
		return nil
	}
	var routingKey RoutingKey
	if err := DB.Where("id = ? AND channel_id = ?", routingKeyID, channelID).First(&routingKey).Error; err != nil {
		return err
	}
	var status int
	availabilityReason := UpstreamAvailabilityRoutable
	switch routingKey.Source {
	case RoutingKeySourceKeyChannel:
		var key ChannelKey
		if err := DB.Where("routing_key_id = ? AND channel_id = ?", routingKeyID, channelID).First(&key).Error; err != nil {
			return err
		}
		status = key.Status
		if status == 0 {
			status = common.ChannelStatusEnabled
		}
	case RoutingKeySourcePlatformSite:
		var key UpstreamKey
		if err := DB.Where("routing_key_id = ? AND channel_id = ?", routingKeyID, channelID).First(&key).Error; err != nil {
			return err
		}
		status = key.Status
		if status == 0 {
			status = common.ChannelStatusEnabled
		}
		enabledKey := key
		enabledKey.Status = UpstreamKeyStatusEnabled
		availabilityReason = enabledKey.AvailabilityReason(now)
	default:
		return nil
	}
	if status != common.ChannelStatusAutoDisabled ||
		availabilityReason != UpstreamAvailabilityRoutable {
		return nil
	}
	health := GetRoutingKeyHealthSummary(
		routingKeyID,
		common.ChannelStatusEnabled,
		availabilityReason,
		now,
	)
	if health.Status == RoutingKeyHealthDisabled || health.Status == RoutingKeyHealthInvalid {
		return nil
	}
	if !UpdateRoutingKeyStatus(channelID, routingKeyID, common.ChannelStatusEnabled, "") {
		return errors.New("failed to restore auto-disabled routing key")
	}
	return nil
}

func ensureRoutingKeyRecoveryRetryIfStillInvalid(routingKeyID uint, channelID int, now time.Time) error {
	var health RoutingKeyHealth
	if err := DB.Where("routing_key_id = ?", routingKeyID).First(&health).Error; err != nil {
		return err
	}
	if health.RecoveryPhase != "" || health.RecoveryNextTestAt != 0 {
		return nil
	}

	var routingKey RoutingKey
	if err := DB.Where("id = ? AND channel_id = ?", routingKeyID, channelID).First(&routingKey).Error; err != nil {
		return err
	}
	status := common.ChannelStatusEnabled
	availabilityReason := UpstreamAvailabilityRoutable
	switch routingKey.Source {
	case RoutingKeySourceKeyChannel:
		var key ChannelKey
		if err := DB.Where("routing_key_id = ? AND channel_id = ?", routingKeyID, channelID).First(&key).Error; err != nil {
			return err
		}
		status = key.Status
		if status == 0 {
			status = common.ChannelStatusEnabled
		}
	case RoutingKeySourcePlatformSite:
		var key UpstreamKey
		if err := DB.Where("routing_key_id = ? AND channel_id = ?", routingKeyID, channelID).First(&key).Error; err != nil {
			return err
		}
		status = key.Status
		if status == 0 {
			status = common.ChannelStatusEnabled
		}
		enabledKey := key
		enabledKey.Status = UpstreamKeyStatusEnabled
		availabilityReason = enabledKey.AvailabilityReason(now)
	default:
		return nil
	}
	summary := GetRoutingKeyHealthSummary(
		routingKeyID,
		common.ChannelStatusEnabled,
		availabilityReason,
		now,
	)
	if status != common.ChannelStatusAutoDisabled &&
		summary.Status != RoutingKeyHealthInvalid {
		return nil
	}
	return DB.Model(&RoutingKeyHealth{}).
		Where("routing_key_id = ?", routingKeyID).
		Updates(map[string]any{
			"recovery_phase":        RoutingKeyRecoveryPhaseProbing,
			"recovery_next_test_at": now.Unix() + int64(RoutingKeyRecoveryRetryInterval.Seconds()),
		}).Error
}
