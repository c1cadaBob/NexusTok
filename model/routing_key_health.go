package model

import (
	"errors"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"

	"gorm.io/gorm"
)

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
)

type RoutingKeyHealth struct {
	RoutingKeyID       uint   `json:"routing_key_id" gorm:"primaryKey;autoIncrement:false"`
	ChannelID          int    `json:"channel_id" gorm:"not null;index"`
	Source             string `json:"source" gorm:"type:varchar(32);not null;index"`
	SamplesJSON        string `json:"-" gorm:"type:text;not null"`
	SampleCount        int    `json:"sample_count" gorm:"index"`
	SuccessCount       int    `json:"success_count" gorm:"index"`
	MaxFirstLatencyMs  int64  `json:"max_first_latency_ms" gorm:"bigint"`
	LastFirstLatencyMs int64  `json:"last_first_latency_ms" gorm:"bigint"`
	LastSampleAt       int64  `json:"last_sample_at" gorm:"bigint;index"`
	UpdatedAt          int64  `json:"updated_at" gorm:"bigint;index"`
}

type RoutingKeyHealthSample struct {
	Success        bool  `json:"success"`
	FirstLatencyMs int64 `json:"first_latency_ms"`
	CreatedAt      int64 `json:"created_at"`
}

type RoutingKeyHealthSummary struct {
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	SampleCount    int    `json:"sample_count"`
	SuccessCount   int    `json:"success_count"`
	FirstLatencyMs int64  `json:"first_latency_ms,omitempty"`
	LastSampleAt   int64  `json:"last_sample_at,omitempty"`
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

func routingKeyHealthSummaryFromRecord(health *RoutingKeyHealth, now time.Time) RoutingKeyHealthSummary {
	if health == nil || health.SampleCount < routingKeyHealthWindowSize {
		return RoutingKeyHealthSummary{Status: RoutingKeyHealthEnabled, Reason: "not_enough_samples"}
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
	}
}

func GetRoutingKeyHealthSummary(
	routingKeyID uint,
	status int,
	availabilityReason string,
	now time.Time,
) RoutingKeyHealthSummary {
	if summary, done := healthSummaryFromBase(status, availabilityReason); done {
		return summary
	}
	if routingKeyID == 0 || DB == nil || !routingKeyHealthTableReady(DB) {
		return RoutingKeyHealthSummary{Status: RoutingKeyHealthEnabled, Reason: "no_health_record"}
	}
	var health RoutingKeyHealth
	if err := DB.Where("routing_key_id = ?", routingKeyID).First(&health).Error; err != nil {
		return RoutingKeyHealthSummary{Status: RoutingKeyHealthEnabled, Reason: "no_health_record"}
	}
	return routingKeyHealthSummaryFromRecord(&health, now)
}

func RoutingKeyHealthAllowsRouting(
	routingKeyID uint,
	status int,
	availabilityReason string,
	now time.Time,
) bool {
	summary := GetRoutingKeyHealthSummary(routingKeyID, status, availabilityReason, now)
	return summary.Status != RoutingKeyHealthDisabled && summary.Status != RoutingKeyHealthInvalid
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

func RecordRoutingKeyHealthSample(
	routingKeyID uint,
	channelID int,
	source string,
	success bool,
	firstLatencyMs int64,
	unixTime int64,
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
