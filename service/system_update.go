// Package service - system_update.go
// 该文件实现系统维护更新能力。裸机二进制部署会从 GitHub Release 下载与当前平台
// 匹配的 NexusTok 二进制，校验 SHA256 后在当前可执行文件同目录完成备份和原子替换；
// Docker 部署则通过 Docker Engine API 拉取目标镜像并重建当前容器。所有会改变运行
// 形态的动作都挂在 SystemTask 上，便于 Root 管理员观察进度、失败原因和回滚入口。
package service

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
)

const (
	systemUpdateDefaultGitHubRepo = "c1cadaBob/NexusTok"
	systemUpdateCacheTTL          = 20 * time.Minute
	systemUpdateMaxDownloadSize   = 500 * 1024 * 1024
	systemUpdateChecksumMaxSize   = 2 * 1024 * 1024

	systemUpdateAllowedDownloadHost = "github.com"
	systemUpdateAllowedAssetHost    = "objects.githubusercontent.com"

	systemUpdateBuildRelease   = "release"
	systemUpdateBuildSource    = "source"
	systemUpdateBuildContainer = "container"
	systemUpdateActiveKey      = "system_binary_update"

	systemUpdateReleaseStatusPublished = "published"
	systemUpdateReleaseStatusNone      = "none"

	systemUpdateDeploymentBinary           = "binary"
	systemUpdateDeploymentDockerRun        = "docker_run"
	systemUpdateDeploymentDockerCompose    = "docker_compose"
	systemUpdateDeploymentContainerUnknown = "container_unknown"
	systemUpdateDeploymentSource           = "source"

	systemUpdateComparisonNewer   = "newer"
	systemUpdateComparisonLatest  = "latest"
	systemUpdateComparisonOlder   = "older"
	systemUpdateComparisonUnknown = "unknown"

	systemUpdateMethodBinaryReplace = "binary_replace"
	systemUpdateMethodDockerEngine  = "docker_engine"
	systemUpdateMethodDockerCompose = "docker_compose"
	systemUpdateMethodManual        = "manual"

	systemUpdateDefaultDockerImage = "c1cadabob/nexustok:latest"
)

const (
	SystemUpdatePhaseChecking            = "checking"
	SystemUpdatePhaseDownloading         = "downloading"
	SystemUpdatePhaseVerifying           = "verifying"
	SystemUpdatePhaseBackingUp           = "backing_up"
	SystemUpdatePhaseReplacing           = "replacing"
	SystemUpdatePhaseReady               = "ready"
	SystemUpdatePhaseRollingBack         = "rolling_back"
	SystemUpdatePhasePullingImage        = "pulling_image"
	SystemUpdatePhaseStartingHelper      = "starting_helper"
	SystemUpdatePhaseRecreatingContainer = "recreating_container"
	SystemUpdatePhaseProbing             = "probing"
)

var (
	ErrSystemUpdateUnavailable = errors.New("no system update available")
	ErrSystemUpdateDisabled    = errors.New("system update cannot be applied")
	ErrSystemRollbackDisabled  = errors.New("no backup executable found")
	ErrSystemUpdateNoRelease   = errors.New("no published GitHub release was found")
	ErrSystemUpdateHandedOff   = errors.New("system update task handed off to docker helper")
)

// SystemUpdateInfo 是系统更新检查接口返回给前端的完整视图。
type SystemUpdateInfo struct {
	CurrentVersion         string                   `json:"current_version"`
	LatestVersion          string                   `json:"latest_version"`
	HasUpdate              bool                     `json:"has_update"`
	Cached                 bool                     `json:"cached"`
	ReleaseInfo            *SystemUpdateReleaseInfo `json:"release_info,omitempty"`
	MatchedAsset           *SystemUpdateAsset       `json:"matched_asset,omitempty"`
	ChecksumAsset          *SystemUpdateAsset       `json:"checksum_asset,omitempty"`
	Runtime                SystemUpdateRuntime      `json:"runtime"`
	BuildType              string                   `json:"build_type"`
	DeploymentMode         string                   `json:"deployment_mode"`
	ComparisonStatus       string                   `json:"comparison_status"`
	UpdateMethod           string                   `json:"update_method"`
	TargetImage            string                   `json:"target_image,omitempty"`
	Docker                 *SystemUpdateDockerInfo  `json:"docker,omitempty"`
	DockerControlAvailable bool                     `json:"docker_control_available"`
	CanApply               bool                     `json:"can_apply"`
	ApplyDisabledReason    string                   `json:"apply_disabled_reason,omitempty"`
	RollbackAvailable      bool                     `json:"rollback_available"`
	Warning                string                   `json:"warning,omitempty"`
	ReleaseStatus          string                   `json:"release_status"`
	ManualUpdateHint       string                   `json:"manual_update_hint,omitempty"`
}

// SystemUpdateRuntime 描述当前进程运行平台，用于前端解释自动更新可用性。
type SystemUpdateRuntime struct {
	GOOS                 string `json:"goos"`
	GOARCH               string `json:"goarch"`
	IsRunningInContainer bool   `json:"is_running_in_container"`
}

// SystemUpdateDockerInfo 描述当前 Docker 部署的可更新信息。
type SystemUpdateDockerInfo struct {
	ContainerID          string `json:"container_id,omitempty"`
	ContainerName        string `json:"container_name,omitempty"`
	CurrentImage         string `json:"current_image,omitempty"`
	CurrentImageID       string `json:"current_image_id,omitempty"`
	TargetImage          string `json:"target_image,omitempty"`
	SocketPath           string `json:"socket_path,omitempty"`
	SocketAvailable      bool   `json:"socket_available"`
	ComposeProject       string `json:"compose_project,omitempty"`
	ComposeService       string `json:"compose_service,omitempty"`
	ComposeWorkingDir    string `json:"compose_working_dir,omitempty"`
	ComposeConfigFiles   string `json:"compose_config_files,omitempty"`
	OneTimeEnableCommand string `json:"one_time_enable_command,omitempty"`
	ManualUpdateCommand  string `json:"manual_update_command,omitempty"`
}

// SystemUpdateReleaseInfo 是 GitHub Release 中用于维护页展示的字段。
type SystemUpdateReleaseInfo struct {
	TagName     string              `json:"tag_name"`
	Name        string              `json:"name"`
	Body        string              `json:"body"`
	HTMLURL     string              `json:"html_url"`
	PublishedAt string              `json:"published_at"`
	Assets      []SystemUpdateAsset `json:"assets,omitempty"`
}

// SystemUpdateAsset 表示一个 GitHub Release 产物。
type SystemUpdateAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
}

// SystemUpdateTaskPayload 保存系统更新任务的目标版本和产物快照。
type SystemUpdateTaskPayload struct {
	TargetVersion string `json:"target_version,omitempty"`
	AssetName     string `json:"asset_name,omitempty"`
	Mode          string `json:"mode,omitempty"`
	TargetImage   string `json:"target_image,omitempty"`
}

// SystemUpdateTaskState 是更新/回滚任务的可观察进度。
type SystemUpdateTaskState struct {
	Phase           string `json:"phase"`
	Progress        int    `json:"progress"`
	DownloadedBytes int64  `json:"downloaded_bytes,omitempty"`
	TotalBytes      int64  `json:"total_bytes,omitempty"`
	TargetVersion   string `json:"target_version,omitempty"`
	AssetName       string `json:"asset_name,omitempty"`
	Mode            string `json:"mode,omitempty"`
	TargetImage     string `json:"target_image,omitempty"`
}

// SystemUpdateTaskResult 保存更新任务完成后的关键信息，方便前端提示重启和回滚。
type SystemUpdateTaskResult struct {
	CurrentVersion      string `json:"current_version"`
	TargetVersion       string `json:"target_version,omitempty"`
	AssetName           string `json:"asset_name,omitempty"`
	CurrentExecutable   string `json:"current_executable,omitempty"`
	BackupPath          string `json:"backup_path,omitempty"`
	SHA256              string `json:"sha256,omitempty"`
	HTMLURL             string `json:"html_url,omitempty"`
	PublishedAt         string `json:"published_at,omitempty"`
	RestartRequired     bool   `json:"restart_required"`
	RollbackAvailable   bool   `json:"rollback_available"`
	PreviousExecutable  string `json:"previous_executable,omitempty"`
	Mode                string `json:"mode,omitempty"`
	TargetImage         string `json:"target_image,omitempty"`
	OldContainerID      string `json:"old_container_id,omitempty"`
	NewContainerID      string `json:"new_container_id,omitempty"`
	BackupContainerName string `json:"backup_container_name,omitempty"`
}

// SystemRestartResult 描述一次重启请求是否已被调度。
type SystemRestartResult struct {
	RestartSupported bool   `json:"restart_supported"`
	RestartScheduled bool   `json:"restart_scheduled"`
	ManualRequired   bool   `json:"manual_required"`
	Message          string `json:"message"`
}

// SystemUpdateGitHubClient 抽象 GitHub Release 请求，便于测试中替换为本地实现。
type SystemUpdateGitHubClient interface {
	FetchLatestRelease(ctx context.Context, repo string) (*systemUpdateGitHubRelease, error)
	DownloadFile(ctx context.Context, rawURL string, dest string, maxSize int64, onProgress func(downloaded, total int64)) error
	FetchFile(ctx context.Context, rawURL string, maxSize int64) ([]byte, error)
}

type systemUpdateGitHubRelease struct {
	TagName     string                    `json:"tag_name"`
	Name        string                    `json:"name"`
	Body        string                    `json:"body"`
	PublishedAt string                    `json:"published_at"`
	HTMLURL     string                    `json:"html_url"`
	Assets      []systemUpdateGitHubAsset `json:"assets"`
}

type systemUpdateGitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type systemUpdateHTTPClient struct {
	metadataClient *http.Client
	downloadClient *http.Client
}

// NewSystemUpdateHTTPClient 创建默认 GitHub Release 客户端。
//
// 元数据请求和下载请求使用不同超时：检查更新应快速返回，二进制下载则允许更长时间。
func NewSystemUpdateHTTPClient() SystemUpdateGitHubClient {
	return &systemUpdateHTTPClient{
		metadataClient: &http.Client{Timeout: 30 * time.Second},
		downloadClient: &http.Client{Timeout: 10 * time.Minute},
	}
}

// SystemUpdateService 封装系统更新检查、执行、回滚和重启。
type SystemUpdateService struct {
	githubClient SystemUpdateGitHubClient

	currentVersionFn     func() string
	goosFn               func() string
	goarchFn             func() string
	isContainerFn        func() bool
	executableFn         func() (string, error)
	evalSymlinksFn       func(string) (string, error)
	dockerSocketPathFn   func() string
	currentContainerIDFn func() string
	nowFn                func() time.Time

	cacheMu  sync.Mutex
	cached   *SystemUpdateInfo
	cachedAt time.Time
}

var defaultSystemUpdateService = NewSystemUpdateService(NewSystemUpdateHTTPClient())

// getSystemUpdateGitHubRepo 返回系统更新使用的 GitHub 仓库。
//
// 这里不能依赖 Go module 路径：历史 module path 仍是 github.com/c1cada/NexusTok，
// 但真实 Release 仓库可能迁移 owner。通过环境变量允许私有部署指向自己的 fork，
// 默认值则使用当前项目实际远程仓库，避免维护页查询不存在的 owner 后只暴露 404。
func getSystemUpdateGitHubRepo() string {
	repo := strings.TrimSpace(common.GetEnvOrDefaultString("SYSTEM_UPDATE_GITHUB_REPO", systemUpdateDefaultGitHubRepo))
	if repo == "" {
		return systemUpdateDefaultGitHubRepo
	}
	return repo
}

// NewSystemUpdateService 创建系统更新服务。
func NewSystemUpdateService(githubClient SystemUpdateGitHubClient) *SystemUpdateService {
	return &SystemUpdateService{
		githubClient:         githubClient,
		currentVersionFn:     func() string { return common.Version },
		goosFn:               func() string { return runtime.GOOS },
		goarchFn:             func() string { return runtime.GOARCH },
		isContainerFn:        common.IsRunningInContainer,
		executableFn:         os.Executable,
		evalSymlinksFn:       filepath.EvalSymlinks,
		dockerSocketPathFn:   systemUpdateDockerSocketPath,
		currentContainerIDFn: systemUpdateCurrentContainerID,
		nowFn:                time.Now,
	}
}

func init() {
	RegisterSystemTaskHandler(systemUpdateHandler{})
	RegisterSystemTaskHandler(systemRollbackHandler{})
}

// CheckLatestSystemUpdate 使用默认服务检查最新版本。
func CheckLatestSystemUpdate(ctx context.Context, force bool) (*SystemUpdateInfo, error) {
	return defaultSystemUpdateService.CheckLatest(ctx, force)
}

// StartSystemUpdateTask 创建系统更新任务。
//
// 创建任务前会先做一次强制检查，避免在 Docker、源码构建或无可用产物时排入必然失败的
// 后台任务。任务执行时仍会再次检查并校验下载内容，保证使用最新 Release 状态。
func StartSystemUpdateTask(ctx context.Context) (*model.SystemTask, error) {
	activeTask, err := model.GetActiveSystemTaskByActiveKey(systemUpdateActiveKey)
	if err != nil {
		return nil, err
	}
	if activeTask != nil {
		return activeTask, nil
	}

	info, err := CheckLatestSystemUpdate(ctx, true)
	if err != nil {
		return nil, err
	}
	if !info.HasUpdate && info.UpdateMethod != systemUpdateMethodDockerEngine && info.UpdateMethod != systemUpdateMethodDockerCompose {
		return nil, ErrSystemUpdateUnavailable
	}
	if !info.CanApply {
		if info.ApplyDisabledReason != "" {
			return nil, fmt.Errorf("%w: %s", ErrSystemUpdateDisabled, info.ApplyDisabledReason)
		}
		return nil, ErrSystemUpdateDisabled
	}

	payload := SystemUpdateTaskPayload{
		TargetVersion: info.LatestVersion,
		Mode:          info.DeploymentMode,
		TargetImage:   info.TargetImage,
	}
	if info.MatchedAsset != nil {
		payload.AssetName = info.MatchedAsset.Name
	}

	task, err := model.CreateSystemTaskWithActiveKey(model.SystemTaskTypeSystemUpdate, systemUpdateActiveKey, payload, SystemUpdateTaskState{
		Phase:         SystemUpdatePhaseChecking,
		Progress:      0,
		TargetVersion: payload.TargetVersion,
		AssetName:     payload.AssetName,
		Mode:          payload.Mode,
		TargetImage:   payload.TargetImage,
	})
	if err != nil {
		activeTask, activeErr := model.GetActiveSystemTaskByActiveKey(systemUpdateActiveKey)
		if activeErr == nil && activeTask != nil {
			return activeTask, nil
		}
		return nil, err
	}
	notifySystemTaskRunner()
	return task, nil
}

// StartSystemRollbackTask 创建系统回滚任务。
func StartSystemRollbackTask() (*model.SystemTask, error) {
	activeTask, err := model.GetActiveSystemTaskByActiveKey(systemUpdateActiveKey)
	if err != nil {
		return nil, err
	}
	if activeTask != nil {
		return activeTask, nil
	}
	if !defaultSystemUpdateService.RollbackAvailable() {
		return nil, ErrSystemRollbackDisabled
	}
	task, err := model.CreateSystemTaskWithActiveKey(model.SystemTaskTypeSystemRollback, systemUpdateActiveKey, nil, SystemUpdateTaskState{
		Phase:    SystemUpdatePhaseRollingBack,
		Progress: 0,
	})
	if err != nil {
		activeTask, activeErr := model.GetActiveSystemTaskByActiveKey(systemUpdateActiveKey)
		if activeErr == nil && activeTask != nil {
			return activeTask, nil
		}
		return nil, err
	}
	notifySystemTaskRunner()
	return task, nil
}

// RestartSystemService 调度服务重启。
//
// Linux 裸机部署通常由 systemd/supervisor 配置 Restart=always；这里延迟退出当前进程，
// 给 HTTP 响应和日志留出刷写时间。非 Linux 平台不假定进程管理器能力，只返回手动重启提示。
func RestartSystemService() SystemRestartResult {
	return restartSystemService()
}

// CheckLatest 检查最新 GitHub Release，并补充当前部署是否允许自动应用。
func (s *SystemUpdateService) CheckLatest(ctx context.Context, force bool) (*SystemUpdateInfo, error) {
	if !force {
		if cached := s.getCachedInfo(); cached != nil {
			return cached, nil
		}
	}

	release, err := s.githubClient.FetchLatestRelease(ctx, getSystemUpdateGitHubRepo())
	if err != nil {
		if errors.Is(err, ErrSystemUpdateNoRelease) {
			info := s.buildNoReleaseInfo()
			s.setCachedInfo(info)
			return cloneSystemUpdateInfo(info), nil
		}
		if cached := s.getCachedInfo(); cached != nil {
			cached.Warning = "Using cached update data: " + err.Error()
			return cached, nil
		}
		return nil, err
	}

	info := s.buildUpdateInfo(release)
	s.setCachedInfo(info)
	return cloneSystemUpdateInfo(info), nil
}

func (s *SystemUpdateService) buildNoReleaseInfo() *SystemUpdateInfo {
	currentVersion := strings.TrimSpace(s.currentVersionFn())
	_, currentOK := parseSystemUpdateVersion(currentVersion)
	buildType := s.buildType(currentOK)
	info := &SystemUpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  "",
		HasUpdate:      false,
		Runtime: SystemUpdateRuntime{
			GOOS:                 s.goosFn(),
			GOARCH:               s.goarchFn(),
			IsRunningInContainer: s.isContainerFn(),
		},
		BuildType:           buildType,
		DeploymentMode:      s.deploymentMode(buildType),
		ComparisonStatus:    systemUpdateComparisonUnknown,
		UpdateMethod:        systemUpdateMethodManual,
		RollbackAvailable:   s.RollbackAvailable(),
		ReleaseStatus:       systemUpdateReleaseStatusNone,
		ManualUpdateHint:    systemUpdateManualUpdateHint(buildType),
		CanApply:            false,
		ApplyDisabledReason: "No published GitHub release was found.",
		Warning:             "No published GitHub release was found.",
	}
	s.enrichDockerInfo(context.Background(), info)
	s.syncApplicability(info)
	return info
}

func (s *SystemUpdateService) buildUpdateInfo(release *systemUpdateGitHubRelease) *SystemUpdateInfo {
	currentVersion := strings.TrimSpace(s.currentVersionFn())
	latestVersion := strings.TrimSpace(release.TagName)
	if latestVersion == "" {
		latestVersion = strings.TrimSpace(release.Name)
	}

	currentParsed, currentOK := parseSystemUpdateVersion(currentVersion)
	latestParsed, latestOK := parseSystemUpdateVersion(latestVersion)
	hasUpdate := false
	comparisonStatus := systemUpdateComparisonUnknown
	warning := ""
	if currentOK && latestOK {
		compareResult := compareParsedSystemUpdateVersion(currentParsed, latestParsed)
		switch {
		case compareResult < 0:
			comparisonStatus = systemUpdateComparisonOlder
			hasUpdate = true
		case compareResult == 0:
			comparisonStatus = systemUpdateComparisonLatest
		default:
			comparisonStatus = systemUpdateComparisonNewer
		}
	} else {
		warning = "Unable to compare current version with latest release."
	}

	releaseInfo := convertSystemUpdateRelease(release)
	matchedAsset := matchSystemUpdateAsset(releaseInfo.Assets, s.goosFn(), s.goarchFn(), latestVersion)
	checksumAsset := matchSystemUpdateChecksumAsset(releaseInfo.Assets, s.goosFn())
	info := &SystemUpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		HasUpdate:      hasUpdate,
		ReleaseInfo:    releaseInfo,
		MatchedAsset:   matchedAsset,
		ChecksumAsset:  checksumAsset,
		Runtime: SystemUpdateRuntime{
			GOOS:                 s.goosFn(),
			GOARCH:               s.goarchFn(),
			IsRunningInContainer: s.isContainerFn(),
		},
		BuildType:         s.buildType(currentOK),
		DeploymentMode:    s.deploymentMode(s.buildType(currentOK)),
		ComparisonStatus:  comparisonStatus,
		UpdateMethod:      systemUpdateMethodManual,
		RollbackAvailable: s.RollbackAvailable(),
		Warning:           warning,
		ReleaseStatus:     systemUpdateReleaseStatusPublished,
	}
	s.enrichDockerInfo(context.Background(), info)
	s.syncApplicability(info)
	return info
}

func (s *SystemUpdateService) buildType(versionComparable bool) string {
	if s.isContainerFn() {
		return systemUpdateBuildContainer
	}
	version := strings.TrimSpace(s.currentVersionFn())
	if version == "" || version == "v0.0.0" || version == "0.0.0" || !versionComparable {
		return systemUpdateBuildSource
	}
	return systemUpdateBuildRelease
}

func (s *SystemUpdateService) deploymentMode(buildType string) string {
	switch buildType {
	case systemUpdateBuildContainer:
		return systemUpdateDeploymentContainerUnknown
	case systemUpdateBuildSource:
		return systemUpdateDeploymentSource
	default:
		return systemUpdateDeploymentBinary
	}
}

func (s *SystemUpdateService) syncApplicability(info *SystemUpdateInfo) {
	switch {
	case info.BuildType == systemUpdateBuildContainer:
		s.syncDockerApplicability(info)
	case info.ReleaseStatus == systemUpdateReleaseStatusNone:
		info.CanApply = false
		info.ApplyDisabledReason = "No published GitHub release was found."
		info.ManualUpdateHint = systemUpdateManualUpdateHint(info.BuildType)
	case !info.HasUpdate:
		info.CanApply = false
		info.ApplyDisabledReason = "Already running the latest version."
	case info.BuildType == systemUpdateBuildSource:
		info.CanApply = false
		info.ApplyDisabledReason = "Source or development builds cannot be replaced safely from the dashboard."
		info.ManualUpdateHint = systemUpdateManualUpdateHint(info.BuildType)
	case info.MatchedAsset == nil:
		info.CanApply = false
		info.ApplyDisabledReason = fmt.Sprintf("No compatible release asset found for %s/%s.", info.Runtime.GOOS, info.Runtime.GOARCH)
	case info.ChecksumAsset == nil:
		info.CanApply = false
		info.ApplyDisabledReason = "No checksum file is available for this platform."
	default:
		info.CanApply = true
		info.ApplyDisabledReason = ""
	}
}

func (s *SystemUpdateService) syncDockerApplicability(info *SystemUpdateInfo) {
	info.UpdateMethod = systemUpdateMethodDockerEngine
	if info.DeploymentMode == systemUpdateDeploymentDockerCompose {
		// 第一版仍通过 Docker Engine helper 原子重建当前容器；Compose 标签用于识别和展示
		// 手动命令。这样不依赖镜像内 docker CLI，也避免把 compose 文件路径挂入应用容器。
		info.UpdateMethod = systemUpdateMethodDockerCompose
	}
	if info.ComparisonStatus == systemUpdateComparisonUnknown {
		info.HasUpdate = true
	}
	if info.TargetImage == "" {
		info.TargetImage = systemUpdateTargetDockerImage("")
	}
	if info.Docker != nil {
		info.DockerControlAvailable = info.Docker.SocketAvailable
	}
	if !info.DockerControlAvailable {
		info.CanApply = false
		info.ApplyDisabledReason = "Docker automatic updates require mounting /var/run/docker.sock into the NexusTok container."
		info.ManualUpdateHint = systemUpdateManualUpdateHint(info.BuildType)
		return
	}
	info.CanApply = true
	info.ApplyDisabledReason = ""
	if info.ReleaseStatus == systemUpdateReleaseStatusNone {
		info.Warning = "No published GitHub release was found. Docker update will pull the configured target image."
	}
}

func systemUpdateManualUpdateHint(buildType string) string {
	switch buildType {
	case systemUpdateBuildContainer:
		return systemUpdateDockerManualUpdateHint()
	case systemUpdateBuildSource:
		return "Source or development builds should be updated by pulling the latest code, rebuilding, and restarting the service manually."
	default:
		return "Publish a GitHub Release with matching binary assets and checksums before applying dashboard updates."
	}
}

func systemUpdateDockerManualUpdateHint() string {
	return "Docker deployments should update by pulling c1cadabob/nexustok:latest and recreating the container with the same mounted data directories."
}

// RollbackAvailable 检查当前可执行文件旁边是否存在 .backup。
func (s *SystemUpdateService) RollbackAvailable() bool {
	if s.isContainerFn() {
		return s.DockerRollbackAvailable(context.Background())
	}
	exePath, err := s.executablePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(exePath + ".backup")
	return err == nil
}

// PerformUpdate 下载并替换当前可执行文件。
func (s *SystemUpdateService) PerformUpdate(ctx context.Context, task *model.SystemTask, runnerID string) (*SystemUpdateTaskResult, error) {
	state := SystemUpdateTaskState{Phase: SystemUpdatePhaseChecking, Progress: 5}
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	info, err := s.CheckLatest(ctx, true)
	if err != nil {
		return nil, err
	}
	if !info.HasUpdate && info.UpdateMethod != systemUpdateMethodDockerEngine && info.UpdateMethod != systemUpdateMethodDockerCompose {
		return nil, ErrSystemUpdateUnavailable
	}
	if !info.CanApply {
		return nil, fmt.Errorf("%w: %s", ErrSystemUpdateDisabled, info.ApplyDisabledReason)
	}
	if info.BuildType == systemUpdateBuildContainer {
		return s.HandOffDockerUpdate(ctx, task, runnerID, info)
	}
	if info.MatchedAsset == nil || info.ChecksumAsset == nil || info.ReleaseInfo == nil {
		return nil, ErrSystemUpdateDisabled
	}

	if err := validateSystemUpdateDownloadURL(info.MatchedAsset.DownloadURL); err != nil {
		return nil, fmt.Errorf("invalid release asset URL: %w", err)
	}
	if err := validateSystemUpdateDownloadURL(info.ChecksumAsset.DownloadURL); err != nil {
		return nil, fmt.Errorf("invalid checksum asset URL: %w", err)
	}

	exePath, err := s.executablePath()
	if err != nil {
		return nil, err
	}
	exeDir := filepath.Dir(exePath)
	tempDir, err := os.MkdirTemp(exeDir, ".nexustok-update-*")
	if err != nil {
		return nil, fmt.Errorf("create update temp dir failed: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	assetName := filepath.Base(info.MatchedAsset.Name)
	downloadPath := filepath.Join(tempDir, assetName)
	state = SystemUpdateTaskState{
		Phase:         SystemUpdatePhaseDownloading,
		Progress:      10,
		TargetVersion: info.LatestVersion,
		AssetName:     assetName,
		TotalBytes:    info.MatchedAsset.Size,
	}
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	progressReporter := newSystemUpdateDownloadProgressReporter(task, runnerID, state)
	if err := s.githubClient.DownloadFile(ctx, info.MatchedAsset.DownloadURL, downloadPath, systemUpdateMaxDownloadSize, progressReporter); err != nil {
		return nil, fmt.Errorf("download update failed: %w", err)
	}

	state.Phase = SystemUpdatePhaseVerifying
	state.Progress = 75
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	checksumData, err := s.githubClient.FetchFile(ctx, info.ChecksumAsset.DownloadURL, systemUpdateChecksumMaxSize)
	if err != nil {
		return nil, fmt.Errorf("download checksum failed: %w", err)
	}
	actualSHA256, err := verifySystemUpdateChecksum(downloadPath, assetName, checksumData)
	if err != nil {
		return nil, err
	}

	if s.goosFn() != "windows" {
		if err := os.Chmod(downloadPath, 0755); err != nil {
			return nil, fmt.Errorf("chmod update binary failed: %w", err)
		}
	}

	state.Phase = SystemUpdatePhaseBackingUp
	state.Progress = 85
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	state.Phase = SystemUpdatePhaseReplacing
	state.Progress = 95
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	backupPath, err := replaceSystemUpdateExecutable(exePath, downloadPath)
	if err != nil {
		return nil, err
	}

	state.Phase = SystemUpdatePhaseReady
	state.Progress = 100
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	return &SystemUpdateTaskResult{
		CurrentVersion:    info.CurrentVersion,
		TargetVersion:     info.LatestVersion,
		AssetName:         assetName,
		CurrentExecutable: exePath,
		BackupPath:        backupPath,
		SHA256:            actualSHA256,
		HTMLURL:           info.ReleaseInfo.HTMLURL,
		PublishedAt:       info.ReleaseInfo.PublishedAt,
		RestartRequired:   true,
		RollbackAvailable: true,
		Mode:              systemUpdateDeploymentBinary,
	}, nil
}

// PerformRollback 将 .backup 恢复为当前可执行文件。
func (s *SystemUpdateService) PerformRollback(_ context.Context, task *model.SystemTask, runnerID string) (*SystemUpdateTaskResult, error) {
	if s.isContainerFn() {
		return s.HandOffDockerRollback(context.Background(), task, runnerID)
	}
	state := SystemUpdateTaskState{Phase: SystemUpdatePhaseRollingBack, Progress: 20}
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	exePath, err := s.executablePath()
	if err != nil {
		return nil, err
	}
	backupPath := exePath + ".backup"
	if _, err := os.Stat(backupPath); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrSystemRollbackDisabled
		}
		return nil, err
	}

	if err := os.Rename(backupPath, exePath); err != nil {
		return nil, fmt.Errorf("rollback executable failed: %w", err)
	}

	state.Progress = 100
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	return &SystemUpdateTaskResult{
		CurrentVersion:     s.currentVersionFn(),
		CurrentExecutable:  exePath,
		PreviousExecutable: backupPath,
		RestartRequired:    true,
		RollbackAvailable:  false,
	}, nil
}

func (s *SystemUpdateService) executablePath() (string, error) {
	exePath, err := s.executableFn()
	if err != nil {
		return "", fmt.Errorf("get executable path failed: %w", err)
	}
	resolved, err := s.evalSymlinksFn(exePath)
	if err != nil {
		return "", fmt.Errorf("resolve executable symlink failed: %w", err)
	}
	return resolved, nil
}

func (s *SystemUpdateService) getCachedInfo() *SystemUpdateInfo {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.cached == nil || s.nowFn().Sub(s.cachedAt) > systemUpdateCacheTTL {
		return nil
	}
	info := cloneSystemUpdateInfo(s.cached)
	info.Cached = true
	info.RollbackAvailable = s.RollbackAvailable()
	s.syncApplicability(info)
	return info
}

func (s *SystemUpdateService) setCachedInfo(info *SystemUpdateInfo) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	cached := cloneSystemUpdateInfo(info)
	cached.Cached = false
	s.cached = cached
	s.cachedAt = s.nowFn()
}

func (systemUpdateHandler) Type() string {
	return model.SystemTaskTypeSystemUpdate
}

type systemUpdateHandler struct{}

func (systemUpdateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	result, err := defaultSystemUpdateService.PerformUpdate(ctx, task, runnerID)
	if errors.Is(err, ErrSystemUpdateHandedOff) {
		return
	}
	status := model.SystemTaskStatusSucceeded
	errorMessage := ""
	if err != nil {
		status = model.SystemTaskStatusFailed
		errorMessage = err.Error()
		logger.LogWarn(ctx, fmt.Sprintf("system update task %s failed: %v", task.TaskID, err))
	}
	if finishErr := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); finishErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("system update task %s failed to save result: %v", task.TaskID, finishErr))
	}
}

type systemRollbackHandler struct{}

func (systemRollbackHandler) Type() string {
	return model.SystemTaskTypeSystemRollback
}

func (systemRollbackHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	result, err := defaultSystemUpdateService.PerformRollback(ctx, task, runnerID)
	if errors.Is(err, ErrSystemUpdateHandedOff) {
		return
	}
	status := model.SystemTaskStatusSucceeded
	errorMessage := ""
	if err != nil {
		status = model.SystemTaskStatusFailed
		errorMessage = err.Error()
		logger.LogWarn(ctx, fmt.Sprintf("system rollback task %s failed: %v", task.TaskID, err))
	}
	if finishErr := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); finishErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("system rollback task %s failed to save result: %v", task.TaskID, finishErr))
	}
}

func (c *systemUpdateHTTPClient) FetchLatestRelease(ctx context.Context, repo string) (*systemUpdateGitHubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "NexusTok-Updater")

	resp, err := c.metadataClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, ErrSystemUpdateNoRelease
		}
		return nil, fmt.Errorf("GitHub releases API returned %d", resp.StatusCode)
	}

	var release systemUpdateGitHubRelease
	if err := common.DecodeJson(resp.Body, &release); err != nil {
		return nil, err
	}
	return &release, nil
}

func (c *systemUpdateHTTPClient) DownloadFile(ctx context.Context, rawURL string, dest string, maxSize int64, onProgress func(downloaded, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}
	if resp.ContentLength > maxSize {
		return fmt.Errorf("download size %d exceeds maximum %d", resp.ContentLength, maxSize)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}

	buf := make([]byte, 128*1024)
	var written int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			written += int64(n)
			if written > maxSize {
				_ = out.Close()
				_ = os.Remove(dest)
				return fmt.Errorf("download exceeded maximum size of %d bytes", maxSize)
			}
			if _, err := out.Write(buf[:n]); err != nil {
				_ = out.Close()
				_ = os.Remove(dest)
				return err
			}
			if onProgress != nil {
				onProgress(written, resp.ContentLength)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = out.Close()
			_ = os.Remove(dest)
			return readErr
		}
	}

	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}

func (c *systemUpdateHTTPClient) FetchFile(ctx context.Context, rawURL string, maxSize int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.metadataClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned %d", resp.StatusCode)
	}
	if resp.ContentLength > maxSize {
		return nil, fmt.Errorf("file size %d exceeds maximum %d", resp.ContentLength, maxSize)
	}
	limited := io.LimitReader(resp.Body, maxSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("download exceeded maximum size of %d bytes", maxSize)
	}
	return data, nil
}

func convertSystemUpdateRelease(release *systemUpdateGitHubRelease) *SystemUpdateReleaseInfo {
	assets := make([]SystemUpdateAsset, 0, len(release.Assets))
	for _, asset := range release.Assets {
		assets = append(assets, SystemUpdateAsset{
			Name:        asset.Name,
			DownloadURL: asset.BrowserDownloadURL,
			Size:        asset.Size,
		})
	}
	return &SystemUpdateReleaseInfo{
		TagName:     release.TagName,
		Name:        release.Name,
		Body:        release.Body,
		HTMLURL:     release.HTMLURL,
		PublishedAt: release.PublishedAt,
		Assets:      assets,
	}
}

func matchSystemUpdateAsset(assets []SystemUpdateAsset, goos string, goarch string, version string) *SystemUpdateAsset {
	expectedNames := expectedSystemUpdateAssetNames(goos, goarch, version)
	for _, expectedName := range expectedNames {
		for i := range assets {
			if assets[i].Name == expectedName {
				asset := assets[i]
				return &asset
			}
		}
	}
	return nil
}

func expectedSystemUpdateAssetNames(goos string, goarch string, version string) []string {
	versionCandidates := uniqueSystemUpdateVersionCandidates(version)
	names := make([]string, 0, len(versionCandidates)*2)
	for _, v := range versionCandidates {
		switch goos {
		case "linux":
			if goarch == "amd64" {
				names = append(names, fmt.Sprintf("nexustok-%s", v))
			}
			if goarch == "arm64" {
				names = append(names, fmt.Sprintf("nexustok-arm64-%s", v))
			}
		case "darwin":
			names = append(names, fmt.Sprintf("nexustok-macos-%s", v))
		case "windows":
			if goarch == "amd64" {
				names = append(names, fmt.Sprintf("nexustok-%s.exe", v))
			}
		}
	}
	return names
}

func uniqueSystemUpdateVersionCandidates(version string) []string {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return nil
	}
	candidates := []string{trimmed}
	withoutV := strings.TrimPrefix(trimmed, "v")
	if withoutV != trimmed {
		candidates = append(candidates, withoutV)
	}
	return candidates
}

func matchSystemUpdateChecksumAsset(assets []SystemUpdateAsset, goos string) *SystemUpdateAsset {
	checksumName := expectedSystemUpdateChecksumName(goos)
	if checksumName == "" {
		return nil
	}
	for i := range assets {
		if assets[i].Name == checksumName {
			asset := assets[i]
			return &asset
		}
	}
	return nil
}

func expectedSystemUpdateChecksumName(goos string) string {
	switch goos {
	case "linux":
		return "checksums-linux.txt"
	case "darwin":
		return "checksums-macos.txt"
	case "windows":
		return "checksums-windows.txt"
	default:
		return ""
	}
}

func validateSystemUpdateDownloadURL(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsedURL.Scheme != "https" {
		return errors.New("only HTTPS download URLs are allowed")
	}
	host := strings.ToLower(parsedURL.Hostname())
	if host == "" {
		return errors.New("download host is empty")
	}
	if host == systemUpdateAllowedDownloadHost ||
		strings.HasSuffix(host, "."+systemUpdateAllowedDownloadHost) ||
		host == systemUpdateAllowedAssetHost ||
		strings.HasSuffix(host, "."+systemUpdateAllowedAssetHost) {
		return nil
	}
	return fmt.Errorf("download from untrusted host: %s", host)
}

func verifySystemUpdateChecksum(filePath string, assetName string, checksumData []byte) (string, error) {
	expectedHash, err := findSystemUpdateChecksum(checksumData, assetName)
	if err != nil {
		return "", err
	}
	actualHash, err := sha256File(filePath)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(expectedHash, actualHash) {
		return "", fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}
	return actualHash, nil
}

func findSystemUpdateChecksum(checksumData []byte, assetName string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(checksumData)))
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) < 2 {
			continue
		}
		fileName := strings.TrimPrefix(parts[len(parts)-1], "*")
		if fileName == assetName {
			return strings.ToLower(parts[0]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("checksum not found for %s", assetName)
}

func sha256File(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func replaceSystemUpdateExecutable(exePath string, newBinaryPath string) (string, error) {
	backupPath := exePath + ".backup"
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove old backup failed: %w", err)
	}
	if err := os.Rename(exePath, backupPath); err != nil {
		return "", fmt.Errorf("backup current executable failed: %w", err)
	}
	if err := os.Rename(newBinaryPath, exePath); err != nil {
		if restoreErr := os.Rename(backupPath, exePath); restoreErr != nil {
			return "", fmt.Errorf("replace executable failed: %w; restore backup failed: %v", err, restoreErr)
		}
		return "", fmt.Errorf("replace executable failed and original was restored: %w", err)
	}
	return backupPath, nil
}

func updateSystemUpdateTaskState(task *model.SystemTask, runnerID string, state SystemUpdateTaskState) error {
	if task == nil {
		return nil
	}
	if state.Progress < 0 {
		state.Progress = 0
	}
	if state.Progress > 100 {
		state.Progress = 100
	}
	return model.UpdateSystemTaskState(task.TaskID, runnerID, state)
}

func newSystemUpdateDownloadProgressReporter(task *model.SystemTask, runnerID string, baseState SystemUpdateTaskState) func(downloaded, total int64) {
	const minWriteInterval = 2 * time.Second
	var (
		lastProgress = -1
		lastWriteAt  time.Time
	)
	return func(downloaded, total int64) {
		progress := 35
		if total > 0 {
			progress = 10 + int(downloaded*60/total)
		}
		if progress > 70 {
			progress = 70
		}
		if progress < 10 {
			progress = 10
		}
		if progress == lastProgress && !lastWriteAt.IsZero() && time.Since(lastWriteAt) < minWriteInterval {
			return
		}
		lastProgress = progress
		lastWriteAt = time.Now()
		state := baseState
		state.Progress = progress
		state.DownloadedBytes = downloaded
		if total > 0 {
			state.TotalBytes = total
		}
		if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
			logSystemTaskLockError(context.Background(), task, err)
		}
	}
}

type parsedSystemUpdateVersion struct {
	major      int
	minor      int
	patch      int
	prerelease string
}

func parseSystemUpdateVersion(version string) (parsedSystemUpdateVersion, bool) {
	normalized := strings.TrimSpace(version)
	normalized = strings.TrimPrefix(normalized, "v")
	if normalized == "" {
		return parsedSystemUpdateVersion{}, false
	}
	if plusIndex := strings.Index(normalized, "+"); plusIndex >= 0 {
		normalized = normalized[:plusIndex]
	}
	prerelease := ""
	if dashIndex := strings.Index(normalized, "-"); dashIndex >= 0 {
		prerelease = normalized[dashIndex+1:]
		normalized = normalized[:dashIndex]
	}
	parts := strings.Split(normalized, ".")
	if len(parts) < 3 {
		return parsedSystemUpdateVersion{}, false
	}
	numbers := [3]int{}
	for i := 0; i < 3; i++ {
		value, err := strconv.Atoi(parts[i])
		if err != nil || value < 0 {
			return parsedSystemUpdateVersion{}, false
		}
		numbers[i] = value
	}
	return parsedSystemUpdateVersion{
		major:      numbers[0],
		minor:      numbers[1],
		patch:      numbers[2],
		prerelease: prerelease,
	}, true
}

func compareParsedSystemUpdateVersion(current parsedSystemUpdateVersion, latest parsedSystemUpdateVersion) int {
	if current.major != latest.major {
		return compareInt(current.major, latest.major)
	}
	if current.minor != latest.minor {
		return compareInt(current.minor, latest.minor)
	}
	if current.patch != latest.patch {
		return compareInt(current.patch, latest.patch)
	}
	if current.prerelease == latest.prerelease {
		return 0
	}
	if current.prerelease != "" && latest.prerelease == "" {
		return -1
	}
	if current.prerelease == "" && latest.prerelease != "" {
		return 1
	}
	return strings.Compare(current.prerelease, latest.prerelease)
}

func compareInt(a int, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func cloneSystemUpdateInfo(info *SystemUpdateInfo) *SystemUpdateInfo {
	if info == nil {
		return nil
	}
	clone := *info
	if info.ReleaseInfo != nil {
		releaseClone := *info.ReleaseInfo
		releaseClone.Assets = append([]SystemUpdateAsset(nil), info.ReleaseInfo.Assets...)
		clone.ReleaseInfo = &releaseClone
	}
	if info.MatchedAsset != nil {
		assetClone := *info.MatchedAsset
		clone.MatchedAsset = &assetClone
	}
	if info.ChecksumAsset != nil {
		assetClone := *info.ChecksumAsset
		clone.ChecksumAsset = &assetClone
	}
	if info.Docker != nil {
		dockerClone := *info.Docker
		clone.Docker = &dockerClone
	}
	return &clone
}

var (
	systemRestartGOOS  = func() string { return runtime.GOOS }
	systemRestartExit  = os.Exit
	systemRestartDelay = 100 * time.Millisecond
)

func restartSystemService() SystemRestartResult {
	if systemRestartGOOS() != "linux" {
		return SystemRestartResult{
			RestartSupported: false,
			RestartScheduled: false,
			ManualRequired:   true,
			Message:          "Automatic restart is only supported on Linux deployments managed by systemd or another supervisor.",
		}
	}

	go func() {
		time.Sleep(systemRestartDelay)
		systemRestartExit(0)
	}()

	return SystemRestartResult{
		RestartSupported: true,
		RestartScheduled: true,
		ManualRequired:   false,
		Message:          "Restart scheduled. The process will exit and should be restarted by the configured service manager.",
	}
}
