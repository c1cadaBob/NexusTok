package model

import (
	"errors"

	"github.com/c1cadaBob/NexusTok/common"
)

const (
	PlatformSiteStagePending = "pending"
	PlatformSiteStageSuccess = "success"
	PlatformSiteStageWarning = "warning"
	PlatformSiteStageFailed  = "failed"
	PlatformSiteStageWaiting = "waiting"
	PlatformSiteStageSkipped = "skipped"
)

type PlatformSiteSyncStage struct {
	Status            string `json:"status"`
	UpdatedAt         int64  `json:"updated_at"`
	Error             string `json:"error"`
	HTTPStatus        int    `json:"http_status"`
	URL               string `json:"url"`
	ContentType       string `json:"content_type"`
	Redirected        bool   `json:"redirected"`
	ResponseCategory  string `json:"response_category"`
	VerificationType  string `json:"verification_type"`
	BrowserAuthStatus string `json:"browser_auth_status"`
	DiagnosisCategory string `json:"diagnosis_category"`
	UsedPrevious      bool   `json:"used_previous"`
	ItemsTotal        int    `json:"items_total"`
	ItemsSucceeded    int    `json:"items_succeeded"`
	ItemsFailed       int    `json:"items_failed"`
}

type PlatformSiteSyncStages struct {
	Version          int                   `json:"version"`
	OverallStatus    string                `json:"overall_status"`
	UpdatedAt        int64                 `json:"updated_at"`
	Authentication   PlatformSiteSyncStage `json:"authentication"`
	CurrentUser      PlatformSiteSyncStage `json:"current_user"`
	BalanceUsage     PlatformSiteSyncStage `json:"balance_usage"`
	GroupsRates      PlatformSiteSyncStage `json:"groups_rates"`
	KeyPagination    PlatformSiteSyncStage `json:"key_pagination"`
	KeySecrets       PlatformSiteSyncStage `json:"key_secrets"`
	KeyModels        PlatformSiteSyncStage `json:"key_models"`
	Sub2APIEndpoints PlatformSiteSyncStage `json:"sub2api_endpoints"`
}

func NewPlatformSiteSyncStages(now int64) PlatformSiteSyncStages {
	pending := PlatformSiteSyncStage{Status: PlatformSiteStagePending}
	return PlatformSiteSyncStages{
		Version:          1,
		OverallStatus:    PlatformSiteStagePending,
		UpdatedAt:        now,
		Authentication:   pending,
		CurrentUser:      pending,
		BalanceUsage:     pending,
		GroupsRates:      pending,
		KeyPagination:    pending,
		KeySecrets:       pending,
		KeyModels:        pending,
		Sub2APIEndpoints: pending,
	}
}

func EncodePlatformSiteSyncStages(stages PlatformSiteSyncStages) (string, error) {
	data, err := common.Marshal(stages)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func DecodePlatformSiteSyncStages(raw string) (PlatformSiteSyncStages, error) {
	if raw == "" {
		return NewPlatformSiteSyncStages(common.GetTimestamp()), nil
	}
	stages := NewPlatformSiteSyncStages(common.GetTimestamp())
	if err := common.Unmarshal([]byte(raw), &stages); err != nil {
		return PlatformSiteSyncStages{}, err
	}
	if stages.Version == 0 {
		return PlatformSiteSyncStages{}, errors.New("平台同步阶段版本无效")
	}
	if stages.OverallStatus == "" {
		stages.OverallStatus = PlatformSiteStagePending
	}
	for _, stage := range []*PlatformSiteSyncStage{
		&stages.Authentication,
		&stages.CurrentUser,
		&stages.BalanceUsage,
		&stages.GroupsRates,
		&stages.KeyPagination,
		&stages.KeySecrets,
		&stages.KeyModels,
		&stages.Sub2APIEndpoints,
	} {
		if stage.Status == "" {
			stage.Status = PlatformSiteStagePending
		}
	}
	return stages, nil
}
