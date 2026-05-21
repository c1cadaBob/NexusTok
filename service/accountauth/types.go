package accountauth

import "time"

const (
	StatusUnknown    = "unknown"
	StatusReady      = "ready"
	StatusRefreshing = "refreshing"
	StatusCooling    = "cooling"
	StatusDisabled   = "disabled"
	StatusError      = "error"
)

type LoginOptions struct {
	NoBrowser    bool              `json:"no_browser"`
	ProjectID    string            `json:"project_id"`
	CallbackPort int               `json:"callback_port"`
	Proxy        string            `json:"proxy"`
	Metadata     map[string]string `json:"metadata"`
}

type LoginStartRequest struct {
	PoolGroupID int          `json:"pool_group_id"`
	Name        string       `json:"name"`
	Options     LoginOptions `json:"options"`
}

type LoginStartResult struct {
	SessionID       string            `json:"session_id"`
	Provider        string            `json:"provider"`
	Mode            string            `json:"mode"`
	AuthorizeURL    string            `json:"authorize_url,omitempty"`
	VerificationURL string            `json:"verification_url,omitempty"`
	UserCode        string            `json:"user_code,omitempty"`
	ExpiresAt       int64             `json:"expires_at,omitempty"`
	PollInterval    int64             `json:"poll_interval,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type LoginSessionStatus string

const (
	LoginSessionPending   LoginSessionStatus = "pending"
	LoginSessionCompleted LoginSessionStatus = "completed"
	LoginSessionFailed    LoginSessionStatus = "failed"
	LoginSessionCancelled LoginSessionStatus = "cancelled"
)

type LoginSessionView struct {
	SessionID       string             `json:"session_id"`
	AccountID       int                `json:"account_id,omitempty"`
	Provider        string             `json:"provider"`
	Mode            string             `json:"mode"`
	Status          LoginSessionStatus `json:"status"`
	StatusMessage   string             `json:"status_message,omitempty"`
	PoolGroupID     int                `json:"pool_group_id"`
	Name            string             `json:"name,omitempty"`
	AuthorizeURL    string             `json:"authorize_url,omitempty"`
	VerificationURL string             `json:"verification_url,omitempty"`
	UserCode        string             `json:"user_code,omitempty"`
	ExpiresAt       int64              `json:"expires_at,omitempty"`
	PollInterval    int64              `json:"poll_interval,omitempty"`
	CreatedAt       int64              `json:"created_at"`
	UpdatedAt       int64              `json:"updated_at"`
	Account         *AccountCredential `json:"account,omitempty"`
}

type LoginCompleteRequest struct {
	SessionID   string       `json:"session_id"`
	PoolGroupID int          `json:"pool_group_id"`
	Name        string       `json:"name"`
	Input       string       `json:"input"`
	Options     LoginOptions `json:"options"`
}

type LoginCompleteResult struct {
	AccountID int                 `json:"account_id"`
	Account   *AccountCredential  `json:"account,omitempty"`
	Runtime   *AccountRuntimeView `json:"runtime,omitempty"`
}

type AccountCredential struct {
	Provider        string            `json:"provider"`
	AuthType        string            `json:"auth_type"`
	Label           string            `json:"label"`
	Credentials     string            `json:"credentials"`
	Summary         string            `json:"summary"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
	Attributes      map[string]string `json:"attributes,omitempty"`
	ExpiresAt       time.Time         `json:"expires_at,omitempty"`
	LastRefreshedAt time.Time         `json:"last_refreshed_at,omitempty"`
	NextRefreshAt   time.Time         `json:"next_refresh_at,omitempty"`
}

type AccountRuntimeView struct {
	Status             string                    `json:"status"`
	StatusMessage      string                    `json:"status_message,omitempty"`
	Unavailable        bool                      `json:"unavailable"`
	Quota              QuotaState                `json:"quota"`
	ModelStates        map[string]*ModelState    `json:"model_states,omitempty"`
	RecentRequests     []RecentRequestBucket     `json:"recent_requests,omitempty"`
	LastError          *ProviderError            `json:"last_error,omitempty"`
	LastRefreshedTime  int64                     `json:"last_refreshed_time,omitempty"`
	NextRefreshTime    int64                     `json:"next_refresh_time,omitempty"`
	NextRetryTime      int64                     `json:"next_retry_time,omitempty"`
	SuccessCount       int64                     `json:"success_count"`
	FailedCount        int64                     `json:"failed_count"`
	CredentialMetadata map[string]any            `json:"credential_metadata,omitempty"`
	CredentialAttrs    map[string]string         `json:"credential_attributes,omitempty"`
	Extra              map[string]map[string]any `json:"extra,omitempty"`
}

type QuotaState struct {
	Exceeded      bool      `json:"exceeded"`
	Reason        string    `json:"reason,omitempty"`
	NextRecoverAt time.Time `json:"next_recover_at,omitempty"`
	BackoffLevel  int       `json:"backoff_level,omitempty"`
}

type ModelState struct {
	Status         string         `json:"status"`
	StatusMessage  string         `json:"status_message,omitempty"`
	Unavailable    bool           `json:"unavailable"`
	NextRetryAfter time.Time      `json:"next_retry_after,omitempty"`
	LastError      *ProviderError `json:"last_error,omitempty"`
	Quota          QuotaState     `json:"quota"`
	UpdatedAt      time.Time      `json:"updated_at,omitempty"`
}

type ProviderError struct {
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	Retryable  bool   `json:"retryable"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type RecentRequestBucket struct {
	Time    string `json:"time"`
	Success int64  `json:"success"`
	Failed  int64  `json:"failed"`
}

type ProviderInfo struct {
	Name            string `json:"name"`
	DisplayName     string `json:"display_name"`
	SupportsOAuth   bool   `json:"supports_oauth"`
	SupportsDevice  bool   `json:"supports_device"`
	SupportsRefresh bool   `json:"supports_refresh"`
}
