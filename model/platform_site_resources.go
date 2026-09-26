package model

import "time"

const (
	PlatformSiteResourceIdentity  = "identity"
	PlatformSiteResourceGroups    = "groups"
	PlatformSiteResourceEndpoints = "endpoints"
	PlatformSiteResourceKeys      = "keys"
	PlatformSiteResourceModels    = "models"
	PlatformSiteResourceUsage     = "usage"

	PlatformSiteResourceStatusSuccess                    = "success"
	PlatformSiteResourceStatusFailed                     = "failed"
	PlatformSiteResourceStatusPartial                    = "partial"
	PlatformSiteResourceStatusSecureVerificationRequired = "secure_verification_required"
	PlatformSiteResourceStatusStale                      = "stale"
)

type PlatformSiteIdentity struct {
	ID                uint    `json:"id" gorm:"primaryKey"`
	ChannelID         int     `json:"channel_id" gorm:"not null;uniqueIndex"`
	PlatformUserID    string  `json:"platform_user_id" gorm:"type:varchar(255);index"`
	Username          string  `json:"username" gorm:"type:varchar(255)"`
	Email             string  `json:"email" gorm:"type:varchar(320)"`
	DisplayName       string  `json:"display_name" gorm:"type:varchar(255)"`
	Role              string  `json:"role" gorm:"type:varchar(128)"`
	CurrentGroup      string  `json:"current_group" gorm:"type:varchar(255)"`
	Status            string  `json:"status" gorm:"type:varchar(64)"`
	QuotaUnit         string  `json:"quota_unit" gorm:"type:varchar(64)"`
	Balance           float64 `json:"balance"`
	UsedQuota         int64   `json:"used_quota" gorm:"bigint"`
	CurrentValueAt    int64   `json:"current_value_at" gorm:"bigint"`
	SnapshotValueAt   int64   `json:"snapshot_value_at" gorm:"bigint"`
	SourceEndpoint    string  `json:"source_endpoint" gorm:"type:varchar(1024)"`
	UpstreamUpdatedAt int64   `json:"upstream_updated_at" gorm:"bigint"`
	LastSyncAt        int64   `json:"last_sync_at" gorm:"bigint;index"`
}

type PlatformSiteGroup struct {
	ID                uint    `json:"id" gorm:"primaryKey"`
	ChannelID         int     `json:"channel_id" gorm:"not null;uniqueIndex:platform_site_group_identity,priority:1"`
	ExternalID        string  `json:"external_id" gorm:"type:varchar(255);not null;uniqueIndex:platform_site_group_identity,priority:2"`
	Name              string  `json:"name" gorm:"type:varchar(255)"`
	Ratio             float64 `json:"ratio"`
	Available         bool    `json:"available"`
	Usable            bool    `json:"usable"`
	SourceEndpoint    string  `json:"source_endpoint" gorm:"type:varchar(1024)"`
	UpstreamUpdatedAt int64   `json:"upstream_updated_at" gorm:"bigint"`
	LastSyncAt        int64   `json:"last_sync_at" gorm:"bigint;index"`
}

type PlatformSiteEndpoint struct {
	ID              uint   `json:"id" gorm:"primaryKey"`
	ChannelID       int    `json:"channel_id" gorm:"not null;uniqueIndex"`
	ManagementURL   string `json:"management_url" gorm:"type:varchar(1024)"`
	RelayURL        string `json:"relay_url" gorm:"type:varchar(1024)"`
	ModelsURL       string `json:"models_url" gorm:"type:varchar(1024)"`
	PricingURL      string `json:"pricing_url" gorm:"type:varchar(1024)"`
	UsageURL        string `json:"usage_url" gorm:"type:varchar(1024)"`
	TokenURL        string `json:"token_url" gorm:"type:varchar(1024)"`
	AdminURL        string `json:"admin_url" gorm:"type:varchar(1024)"`
	OpenAIURL       string `json:"openai_url" gorm:"type:varchar(1024)"`
	ClaudeURL       string `json:"claude_url" gorm:"type:varchar(1024)"`
	GeminiURL       string `json:"gemini_url" gorm:"type:varchar(1024)"`
	ResponsesURL    string `json:"responses_url" gorm:"type:varchar(1024)"`
	Source          string `json:"source" gorm:"type:varchar(255)"`
	DiscoveryMethod string `json:"discovery_method" gorm:"type:varchar(64)"`
	Enabled         bool   `json:"enabled"`
	LastConfirmedAt int64  `json:"last_confirmed_at" gorm:"bigint;index"`
}

type PlatformSiteEndpointCapability struct {
	ID              uint   `json:"id" gorm:"primaryKey"`
	EndpointID      uint   `json:"endpoint_id" gorm:"not null;uniqueIndex:platform_site_endpoint_capability,priority:1"`
	Protocol        string `json:"protocol" gorm:"type:varchar(64);not null;uniqueIndex:platform_site_endpoint_capability,priority:2"`
	HTTPMethod      string `json:"http_method" gorm:"type:varchar(16);not null;uniqueIndex:platform_site_endpoint_capability,priority:3"`
	Path            string `json:"path" gorm:"type:varchar(512);not null;uniqueIndex:platform_site_endpoint_capability,priority:4"`
	Supported       bool   `json:"supported"`
	SourceData      string `json:"source_data" gorm:"type:text"`
	LastConfirmedAt int64  `json:"last_confirmed_at" gorm:"bigint;index"`
}

type PlatformSiteResourceSync struct {
	ID                           uint   `json:"id" gorm:"primaryKey"`
	ChannelID                    int    `json:"channel_id" gorm:"not null;uniqueIndex:platform_site_resource_sync,priority:1"`
	ResourceType                 string `json:"resource_type" gorm:"type:varchar(64);not null;uniqueIndex:platform_site_resource_sync,priority:2"`
	Status                       string `json:"status" gorm:"type:varchar(64);index"`
	AttemptedAt                  int64  `json:"attempted_at" gorm:"bigint;index"`
	SucceededAt                  int64  `json:"succeeded_at" gorm:"bigint;index"`
	SourceEndpoint               string `json:"source_endpoint" gorm:"type:varchar(1024)"`
	RecordCount                  int    `json:"record_count"`
	FailureReason                string `json:"failure_reason" gorm:"type:text"`
	Partial                      bool   `json:"partial"`
	RequiresSecurityVerification bool   `json:"requires_security_verification"`
	UsingSnapshot                bool   `json:"using_snapshot"`
}

func (sync *PlatformSiteResourceSync) MarkSuccess(now int64, source string, count int) {
	if sync == nil {
		return
	}
	sync.Status = PlatformSiteResourceStatusSuccess
	sync.AttemptedAt = now
	sync.SucceededAt = now
	sync.SourceEndpoint = source
	sync.RecordCount = count
	sync.FailureReason = ""
	sync.Partial = false
	sync.RequiresSecurityVerification = false
	sync.UsingSnapshot = false
}

func (sync *PlatformSiteResourceSync) MarkFailure(now int64, reason string, snapshot bool) {
	if sync == nil {
		return
	}
	sync.Status = PlatformSiteResourceStatusFailed
	sync.AttemptedAt = now
	sync.FailureReason = reason
	sync.UsingSnapshot = snapshot
}

func (sync *PlatformSiteResourceSync) SnapshotAge(now int64) time.Duration {
	if sync == nil || sync.SucceededAt <= 0 || now <= sync.SucceededAt {
		return 0
	}
	return time.Duration(now-sync.SucceededAt) * time.Second
}
