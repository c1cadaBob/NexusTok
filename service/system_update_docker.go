package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/logger"
	"github.com/c1cadaBob/NexusTok/model"
	"gopkg.in/yaml.v3"
)

const (
	systemUpdateDockerSockDefault = "/var/run/docker.sock"

	// Docker Engine 拉取镜像时会自行访问 registry；短暂的 DNS、连接复用或
	// registry 网关抖动不应直接让后台更新任务失败。重试次数保持有限，避免
	// 真正的镜像名称、权限或配置错误被长时间掩盖。
	systemUpdateDockerPullRetryDelayFirst  = 2 * time.Second
	systemUpdateDockerPullRetryDelaySecond = 5 * time.Second

	systemUpdateDockerHelperActionUpdate   = "update"
	systemUpdateDockerHelperActionRollback = "rollback"

	systemUpdateHelperEnvTaskID             = "SYSTEM_UPDATE_HELPER_TASK_ID"
	systemUpdateHelperEnvRunnerID           = "SYSTEM_UPDATE_HELPER_RUNNER_ID"
	systemUpdateHelperEnvAction             = "SYSTEM_UPDATE_HELPER_ACTION"
	systemUpdateHelperEnvMode               = "SYSTEM_UPDATE_HELPER_MODE"
	systemUpdateHelperEnvCurrentContainerID = "SYSTEM_UPDATE_HELPER_CURRENT_CONTAINER_ID"
	systemUpdateHelperEnvTargetImage        = "SYSTEM_UPDATE_HELPER_TARGET_IMAGE"
	systemUpdateHelperEnvTargetVersion      = "SYSTEM_UPDATE_HELPER_TARGET_VERSION"
	systemUpdateHelperEnvBackupID           = "SYSTEM_UPDATE_HELPER_BACKUP_ID"
	systemUpdateHelperEnvContainerName      = "SYSTEM_UPDATE_HELPER_CONTAINER_NAME"
)

var systemUpdateDockerHealthPollInterval = time.Second
var systemUpdateDockerHelperStartupPollInterval = 100 * time.Millisecond
var systemUpdateDockerHelperStartupGrace = 500 * time.Millisecond

var containerIDPattern = regexp.MustCompile(`[0-9a-f]{64}`)

type dockerEngineClient struct {
	socketPath      string
	httpClient      *http.Client
	pullRetryDelays []time.Duration
}

type dockerInspectContainer struct {
	ID              string                         `json:"Id"`
	Name            string                         `json:"Name"`
	Image           string                         `json:"Image"`
	Config          dockerContainerConfig          `json:"Config"`
	HostConfig      dockerHostConfig               `json:"HostConfig"`
	Mounts          []dockerMount                  `json:"Mounts"`
	NetworkSettings dockerContainerNetworkSettings `json:"NetworkSettings"`
	State           dockerContainerState           `json:"State"`
}

type dockerContainerConfig struct {
	Image        string             `json:"Image,omitempty"`
	Env          []string           `json:"Env,omitempty"`
	Cmd          []string           `json:"Cmd,omitempty"`
	Entrypoint   []string           `json:"Entrypoint,omitempty"`
	WorkingDir   string             `json:"WorkingDir,omitempty"`
	User         string             `json:"User,omitempty"`
	Labels       map[string]string  `json:"Labels,omitempty"`
	ExposedPorts map[string]any     `json:"ExposedPorts,omitempty"`
	Healthcheck  *dockerHealthcheck `json:"Healthcheck,omitempty"`
}

type dockerHostConfig struct {
	Binds         []string                       `json:"Binds,omitempty"`
	Mounts        []dockerMountSpec              `json:"Mounts,omitempty"`
	PortBindings  map[string][]dockerPortBinding `json:"PortBindings,omitempty"`
	RestartPolicy dockerRestartPolicy            `json:"RestartPolicy,omitempty"`
	NetworkMode   string                         `json:"NetworkMode,omitempty"`
	Privileged    bool                           `json:"Privileged,omitempty"`
	ExtraHosts    []string                       `json:"ExtraHosts,omitempty"`
	DNS           []string                       `json:"Dns,omitempty"`
	DNSSearch     []string                       `json:"DnsSearch,omitempty"`
	CapAdd        []string                       `json:"CapAdd,omitempty"`
	CapDrop       []string                       `json:"CapDrop,omitempty"`
	SecurityOpt   []string                       `json:"SecurityOpt,omitempty"`
}

type dockerMountSpec struct {
	Type     string `json:"Type"`
	Source   string `json:"Source,omitempty"`
	Target   string `json:"Target"`
	ReadOnly bool   `json:"ReadOnly,omitempty"`
}

type dockerPortBinding struct {
	HostIP   string `json:"HostIp,omitempty"`
	HostPort string `json:"HostPort,omitempty"`
}

type dockerRestartPolicy struct {
	Name              string `json:"Name,omitempty"`
	MaximumRetryCount int    `json:"MaximumRetryCount,omitempty"`
}

type dockerMount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name,omitempty"`
	Source      string `json:"Source,omitempty"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}

type dockerContainerNetworkSettings struct {
	Networks map[string]dockerEndpointSettings `json:"Networks,omitempty"`
}

type dockerEndpointSettings struct {
	Aliases []string `json:"Aliases,omitempty"`
}

type dockerContainerState struct {
	Running bool                   `json:"Running"`
	Status  string                 `json:"Status"`
	Error   string                 `json:"Error,omitempty"`
	Health  *dockerContainerHealth `json:"Health,omitempty"`
}

type dockerHealthcheck struct {
	Test        []string `json:"Test,omitempty"`
	Interval    int64    `json:"Interval,omitempty"`
	Timeout     int64    `json:"Timeout,omitempty"`
	Retries     int      `json:"Retries,omitempty"`
	StartPeriod int64    `json:"StartPeriod,omitempty"`
}

type dockerContainerHealth struct {
	Status        string `json:"Status,omitempty"`
	FailingStreak int    `json:"FailingStreak,omitempty"`
}

type dockerCreateContainerRequest struct {
	Hostname         string                       `json:"Hostname,omitempty"`
	User             string                       `json:"User,omitempty"`
	Tty              bool                         `json:"Tty,omitempty"`
	Env              []string                     `json:"Env,omitempty"`
	Cmd              []string                     `json:"Cmd,omitempty"`
	Entrypoint       []string                     `json:"Entrypoint,omitempty"`
	Healthcheck      *dockerHealthcheck           `json:"Healthcheck,omitempty"`
	Image            string                       `json:"Image"`
	WorkingDir       string                       `json:"WorkingDir,omitempty"`
	Labels           map[string]string            `json:"Labels,omitempty"`
	ExposedPorts     map[string]any               `json:"ExposedPorts,omitempty"`
	HostConfig       dockerHostConfig             `json:"HostConfig,omitempty"`
	NetworkingConfig dockerCreateNetworkingConfig `json:"NetworkingConfig,omitempty"`
}

type dockerCreateNetworkingConfig struct {
	EndpointsConfig map[string]dockerEndpointSettings `json:"EndpointsConfig,omitempty"`
}

type dockerCreateContainerResponse struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings,omitempty"`
}

type dockerPullStatus struct {
	Status      string `json:"status,omitempty"`
	ID          string `json:"id,omitempty"`
	Error       string `json:"error,omitempty"`
	ErrorDetail *struct {
		Message string `json:"message,omitempty"`
	} `json:"errorDetail,omitempty"`
}

type dockerHelperOptions struct {
	TaskID             string
	RunnerID           string
	Action             string
	Mode               string
	CurrentContainerID string
	TargetVersion      string
	TargetImage        string
	BackupID           string
	ContainerName      string
}

type dockerCreateRequestMode int

const (
	dockerCreateRequestModeFinal dockerCreateRequestMode = iota
	dockerCreateRequestModePreflight
)

type systemUpdateComposeFileBackup struct {
	Path string
	Mode os.FileMode
	Data []byte
}

type systemUpdateComposeFileUpdate struct {
	Backup  systemUpdateComposeFileBackup
	Updated []byte
}

type dockerHelperMountConfig struct {
	Binds  []string
	Mounts []dockerMountSpec
}

func systemUpdateDockerSocketPath() string {
	return strings.TrimSpace(common.GetEnvOrDefaultString("SYSTEM_UPDATE_DOCKER_SOCK", systemUpdateDockerSockDefault))
}

func systemUpdateTargetDockerImage(currentImage string) string {
	repo := systemUpdateDockerImageRepository(currentImage)
	if repo == "" {
		repo = strings.TrimSuffix(systemUpdateDefaultDockerImage, ":latest")
	}
	return repo + ":latest"
}

func systemUpdateDockerImageRepository(currentImage string) string {
	override := strings.TrimSpace(common.GetEnvOrDefaultString("SYSTEM_UPDATE_DOCKER_IMAGE", ""))
	if override != "" {
		if !isSystemUpdateDockerRepositoryOverride(override) {
			return ""
		}
		currentImage = override
	}
	currentImage = strings.TrimSpace(currentImage)
	if currentImage == "" {
		return strings.TrimSuffix(systemUpdateDefaultDockerImage, ":latest")
	}
	if digestIndex := strings.Index(currentImage, "@"); digestIndex >= 0 {
		currentImage = currentImage[:digestIndex]
	}
	repo := currentImage
	if slash := strings.LastIndex(repo, "/"); slash >= 0 {
		if colon := strings.LastIndex(repo[slash+1:], ":"); colon >= 0 {
			repo = repo[:slash+1+colon]
		}
	} else if colon := strings.LastIndex(repo, ":"); colon >= 0 {
		repo = repo[:colon]
	}
	if repo == "" {
		return strings.TrimSuffix(systemUpdateDefaultDockerImage, ":latest")
	}
	if !isSystemUpdateDockerRepository(repo) {
		return ""
	}
	return repo
}

func isSystemUpdateDockerRepositoryOverride(repository string) bool {
	repository = strings.TrimSpace(repository)
	if strings.Contains(repository, "@") {
		return false
	}
	slash := strings.LastIndex(repository, "/")
	if strings.LastIndex(repository, ":") > slash {
		return false
	}
	return isSystemUpdateDockerRepository(repository)
}

func isSystemUpdateDockerRepository(repository string) bool {
	repository = strings.TrimSpace(repository)
	if repository == "" ||
		strings.ContainsAny(repository, "@ \t\r\n") ||
		strings.Contains(repository, "://") ||
		strings.Contains(repository, "//") ||
		strings.HasPrefix(repository, "/") ||
		strings.HasSuffix(repository, "/") {
		return false
	}
	for _, char := range repository {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			strings.ContainsRune("._-/:", char) {
			continue
		}
		return false
	}
	return true
}

func systemUpdateDockerImageForVersion(currentImage string, version string) string {
	if _, ok := parseSystemUpdateVersion(version); !ok {
		return ""
	}
	repo := systemUpdateDockerImageRepository(currentImage)
	if repo == "" {
		return ""
	}
	return repo + ":" + strings.TrimSpace(version)
}

func systemUpdateCurrentContainerID() string {
	if hostname, err := os.Hostname(); err == nil && strings.TrimSpace(hostname) != "" {
		return strings.TrimSpace(hostname)
	}
	for _, path := range []string{"/proc/self/cgroup", "/proc/self/mountinfo"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if match := containerIDPattern.FindString(string(data)); match != "" {
			return match
		}
	}
	return ""
}

func newDockerEngineClient(socketPath string) *dockerEngineClient {
	if strings.TrimSpace(socketPath) == "" {
		socketPath = systemUpdateDockerSockDefault
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &dockerEngineClient{
		socketPath: socketPath,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Minute,
		},
		pullRetryDelays: []time.Duration{
			systemUpdateDockerPullRetryDelayFirst,
			systemUpdateDockerPullRetryDelaySecond,
		},
	}
}

var systemUpdateDockerClientFactory = newDockerEngineClient

func systemUpdateDockerClient(socketPath string) *dockerEngineClient {
	return systemUpdateDockerClientFactory(socketPath)
}

func (c *dockerEngineClient) ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/_ping", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Docker Engine ping returned %d", resp.StatusCode)
	}
	return nil
}

func (c *dockerEngineClient) doJSON(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := common.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Docker Engine %s %s returned %d: %s", method, path, resp.StatusCode, maskSystemUpdateSensitiveInfo(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := common.Unmarshal(data, out); err != nil {
		return fmt.Errorf("解析 Docker Engine 响应失败：%w", err)
	}
	return nil
}

func (c *dockerEngineClient) inspectContainer(ctx context.Context, idOrName string) (*dockerInspectContainer, error) {
	idOrName = strings.TrimSpace(idOrName)
	if idOrName == "" {
		return nil, fmt.Errorf("容器 ID 不能为空")
	}
	var inspect dockerInspectContainer
	if err := c.doJSON(ctx, http.MethodGet, "/containers/"+url.PathEscape(idOrName)+"/json", nil, &inspect); err != nil {
		return nil, err
	}
	return &inspect, nil
}

func (c *dockerEngineClient) pullImage(ctx context.Context, image string, onStatus func(status string)) error {
	retryDelays := c.pullRetryDelays
	if retryDelays == nil {
		retryDelays = []time.Duration{
			systemUpdateDockerPullRetryDelayFirst,
			systemUpdateDockerPullRetryDelaySecond,
		}
	}

	attempts := len(retryDelays) + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		lastErr = c.pullImageOnce(ctx, image, onStatus)
		if lastErr == nil {
			return nil
		}
		if attempt == attempts || !isRetryableDockerPullError(lastErr) || ctx.Err() != nil {
			break
		}

		delay := retryDelays[attempt-1]
		if onStatus != nil {
			onStatus(fmt.Sprintf("Docker image pull temporarily failed; retrying in %s (%d/%d)", delay, attempt, attempts-1))
		}
		if err := waitForDockerPullRetry(ctx, delay); err != nil {
			return err
		}
	}
	return lastErr
}

// pullImageOnce 执行一次 Docker Engine 镜像流式拉取。
//
// Docker Engine 的 pull 接口即使 HTTP 状态为 200，也可能在响应流中返回
// error/errorDetail；因此状态码错误和流内错误都统一转换为可分类的
// dockerPullError，供外层只对瞬时网络错误重试。
func (c *dockerEngineClient) pullImageOnce(ctx context.Context, image string, onStatus func(status string)) error {
	repo, tag := splitDockerImage(image)
	query := url.Values{"fromImage": {repo}}
	if tag != "" {
		query.Set("tag", tag)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker/images/create?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return &dockerPullError{
			statusCode: resp.StatusCode,
			message:    maskSystemUpdateSensitiveInfo(string(data)),
		}
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var status dockerPullStatus
		if err := common.Unmarshal([]byte(line), &status); err != nil {
			continue
		}
		if status.Error != "" {
			if status.ErrorDetail != nil && status.ErrorDetail.Message != "" {
				return &dockerPullError{
					message: maskSystemUpdateSensitiveInfo(status.ErrorDetail.Message),
				}
			}
			return &dockerPullError{
				message: maskSystemUpdateSensitiveInfo(status.Error),
			}
		}
		if onStatus != nil {
			onStatus(firstNonEmptySystemUpdateString(status.Status, status.ID))
		}
	}
	return scanner.Err()
}

type dockerPullError struct {
	statusCode int
	message    string
}

func (e *dockerPullError) Error() string {
	if e == nil {
		return ""
	}
	if e.statusCode > 0 {
		return fmt.Sprintf("Docker image pull returned %d: %s", e.statusCode, e.message)
	}
	return e.message
}

// waitForDockerPullRetry 在重试间隔内监听任务上下文，避免请求已经取消后仍然
// 阻塞 helper。delay 为零主要用于测试，生产默认使用短暂指数退避。
func waitForDockerPullRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// isRetryableDockerPullError 只识别可以通过稍后再次拉取缓解的错误。
//
// Docker Engine 常把 registry 的网络超时包装成 HTTP 500，因此不能只看
// HTTP 状态码；同时也不能对 manifest unknown、pull access denied 等确定性
// 错误盲目重试。
func isRetryableDockerPullError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}

	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return true
	}

	var pullErr *dockerPullError
	if errors.As(err, &pullErr) && pullErr.statusCode >= http.StatusBadGateway && pullErr.statusCode <= http.StatusGatewayTimeout {
		return true
	}

	text := strings.ToLower(err.Error())
	for _, marker := range []string{
		"timeout",
		"timed out",
		"awaiting headers",
		"i/o timeout",
		"connection reset",
		"connection refused",
		"temporary failure",
		"temporarily unavailable",
		"unexpected eof",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// isDockerRegistryTimeoutError 判断错误是否明确表示 registry 连接等待超时。
// Docker Engine 经常把这类错误包装成 HTTP 500，因此同时检查 registry 地址和
// “awaiting headers”等网络错误文本，而不是单独依赖 HTTP 状态码。
func isDockerRegistryTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	hasTimeout := strings.Contains(text, "timeout") ||
		strings.Contains(text, "timed out") ||
		strings.Contains(text, "awaiting headers") ||
		strings.Contains(text, "i/o timeout")
	if !hasTimeout {
		return false
	}
	return strings.Contains(text, "registry") ||
		strings.Contains(text, "docker.io") ||
		strings.Contains(text, "awaiting headers")
}

// formatDockerImagePullError 将 registry 网络故障转换成管理员可直接执行的
// 排查提示；原始错误仍保留在末尾，方便从任务记录和日志定位具体节点。
func formatDockerImagePullError(image string, err error) error {
	if err == nil {
		return nil
	}
	if isDockerRegistryTimeoutError(err) {
		return fmt.Errorf(
			"pull Docker image %q failed: Docker Engine could not reach the image registry before timing out. Check the host network and DNS, configure a Docker registry mirror if needed, then retry. Last error: %w",
			image,
			err,
		)
	}
	if isRetryableDockerPullError(err) {
		return fmt.Errorf(
			"pull Docker image %q failed: Docker Engine encountered a transient network error while contacting the image registry. Check the host network and DNS, configure a Docker registry mirror if needed, then retry. Last error: %w",
			image,
			err,
		)
	}
	return fmt.Errorf("pull Docker image %q failed: %w", image, err)
}

func (c *dockerEngineClient) createContainer(ctx context.Context, name string, request dockerCreateContainerRequest) (string, error) {
	var response dockerCreateContainerResponse
	path := "/containers/create"
	if strings.TrimSpace(name) != "" {
		path += "?name=" + url.QueryEscape(strings.TrimPrefix(name, "/"))
	}
	if err := c.doJSON(ctx, http.MethodPost, path, request, &response); err != nil {
		return "", err
	}
	return response.ID, nil
}

func (c *dockerEngineClient) startContainer(ctx context.Context, idOrName string) error {
	err := c.doJSON(ctx, http.MethodPost, "/containers/"+url.PathEscape(idOrName)+"/start", nil, nil)
	if err != nil && strings.Contains(err.Error(), "returned 304") {
		return nil
	}
	return err
}

func (c *dockerEngineClient) stopContainer(ctx context.Context, idOrName string, timeoutSeconds int) error {
	path := "/containers/" + url.PathEscape(idOrName) + "/stop"
	if timeoutSeconds >= 0 {
		path += "?t=" + url.QueryEscape(fmt.Sprintf("%d", timeoutSeconds))
	}
	err := c.doJSON(ctx, http.MethodPost, path, nil, nil)
	if err != nil && strings.Contains(err.Error(), "returned 304") {
		return nil
	}
	return err
}

func (c *dockerEngineClient) renameContainer(ctx context.Context, idOrName string, name string) error {
	return c.doJSON(ctx, http.MethodPost, "/containers/"+url.PathEscape(idOrName)+"/rename?name="+url.QueryEscape(strings.TrimPrefix(name, "/")), nil, nil)
}

func (c *dockerEngineClient) removeContainer(ctx context.Context, idOrName string, force bool) error {
	path := "/containers/" + url.PathEscape(idOrName)
	if force {
		path += "?force=true"
	}
	err := c.doJSON(ctx, http.MethodDelete, path, nil, nil)
	if err != nil && (strings.Contains(err.Error(), "returned 404") || strings.Contains(err.Error(), "No such container")) {
		return nil
	}
	return err
}

func (c *dockerEngineClient) waitContainer(ctx context.Context, idOrName string) (int, error) {
	var response struct {
		StatusCode int `json:"StatusCode"`
		Error      *struct {
			Message string `json:"Message,omitempty"`
		} `json:"Error,omitempty"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/containers/"+url.PathEscape(idOrName)+"/wait", nil, &response); err != nil {
		return 0, err
	}
	if response.Error != nil && strings.TrimSpace(response.Error.Message) != "" {
		return response.StatusCode, errors.New(maskSystemUpdateSensitiveInfo(response.Error.Message))
	}
	return response.StatusCode, nil
}

func (c *dockerEngineClient) readContainerLogs(ctx context.Context, idOrName string) ([]byte, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"http://docker/containers/"+url.PathEscape(idOrName)+"/logs?stdout=1&stderr=1",
		nil,
	)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return nil, fmt.Errorf(
			"Docker Engine logs returned %d: %s",
			resp.StatusCode,
			maskSystemUpdateSensitiveInfo(string(data)),
		)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func isDockerNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "returned 404") ||
		strings.Contains(text, "no such container") ||
		strings.Contains(text, "not found")
}

func splitDockerImage(image string) (string, string) {
	image = strings.TrimSpace(image)
	if image == "" {
		return systemUpdateDefaultDockerImage, ""
	}
	if strings.Contains(image, "@sha256:") {
		return image, ""
	}
	slash := strings.LastIndex(image, "/")
	colon := strings.LastIndex(image, ":")
	if colon > slash {
		return image[:colon], image[colon+1:]
	}
	return image, "latest"
}

func cleanDockerContainerName(name string) string {
	name = strings.TrimSpace(strings.TrimPrefix(name, "/"))
	if name == "" {
		return "nexustok"
	}
	return name
}

func canonicalDockerContainerName(name string) string {
	name = cleanDockerContainerName(name)
	for {
		switch {
		case strings.Contains(name, ".backup"):
			name = name[:strings.Index(name, ".backup")]
		case strings.Contains(name, ".failed-"):
			name = name[:strings.Index(name, ".failed-")]
		case strings.Contains(name, ".preflight-"):
			name = name[:strings.Index(name, ".preflight-")]
		case strings.Contains(name, ".staging-"):
			name = name[:strings.Index(name, ".staging-")]
		case strings.Contains(name, "-update-helper-"):
			name = name[:strings.Index(name, "-update-helper-")]
		default:
			return cleanDockerContainerName(name)
		}
		name = cleanDockerContainerName(name)
	}
}

func dockerFailedContainerName(name string) string {
	return canonicalDockerContainerName(name) + ".failed-" + dockerUniqueSuffix()
}

func dockerPreflightContainerName(name string) string {
	return canonicalDockerContainerName(name) + ".preflight-" + dockerUniqueSuffix()
}

func dockerStagingContainerName(name string) string {
	return canonicalDockerContainerName(name) + ".staging-" + dockerUniqueSuffix()
}

func dockerUniqueSuffix() string {
	suffix := strings.ToLower(strings.ReplaceAll(common.GetUUID(), "-", ""))
	if suffix == "" {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return suffix
}

func dockerHasHealthcheck(inspect *dockerInspectContainer) bool {
	return inspect != nil &&
		inspect.Config.Healthcheck != nil &&
		len(inspect.Config.Healthcheck.Test) > 0
}

func dockerHealthSummary(inspect *dockerInspectContainer) (string, bool, bool) {
	if !dockerHasHealthcheck(inspect) {
		if inspect != nil && inspect.State.Running {
			return "running_without_healthcheck", false, true
		}
		return "unavailable", false, true
	}
	if inspect.State.Health == nil || strings.TrimSpace(inspect.State.Health.Status) == "" {
		return "starting", true, false
	}
	return strings.ToLower(strings.TrimSpace(inspect.State.Health.Status)), true, false
}

func systemUpdateDockerRecoveryContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func confirmDockerContainerRunning(ctx context.Context, client *dockerEngineClient, containerID string) error {
	inspect, err := client.inspectContainer(ctx, containerID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(inspect.State.Error) != "" {
		return fmt.Errorf(
			"container reported a startup error: %s",
			maskSystemUpdateSensitiveInfo(inspect.State.Error),
		)
	}
	if !inspect.State.Running {
		return fmt.Errorf(
			"container is not running: %s",
			firstNonEmptySystemUpdateString(inspect.State.Error, inspect.State.Status),
		)
	}
	return nil
}

func waitDockerHelperStarted(ctx context.Context, client *dockerEngineClient, helperID string, taskID string) error {
	deadline := time.Now().Add(systemUpdateDockerHelperStartupGrace)
	for {
		inspect, err := client.inspectContainer(ctx, helperID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(inspect.State.Error) != "" {
			return fmt.Errorf(
				"container reported a startup error: %s",
				maskSystemUpdateSensitiveInfo(inspect.State.Error),
			)
		}
		if !inspect.State.Running {
			task, taskErr := model.GetSystemTaskByTaskID(taskID)
			if taskErr == nil && task != nil &&
				(task.Status == model.SystemTaskStatusSucceeded || task.Status == model.SystemTaskStatusFailed) {
				return ErrSystemUpdateHandedOff
			}
			return fmt.Errorf(
				"container is not running: %s",
				firstNonEmptySystemUpdateString(inspect.State.Error, inspect.State.Status),
			)
		}
		if time.Now().After(deadline) {
			return nil
		}
		interval := systemUpdateDockerHelperStartupPollInterval
		if interval <= 0 {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *SystemUpdateService) enrichDockerInfo(ctx context.Context, info *SystemUpdateInfo) {
	if info == nil || info.BuildType != systemUpdateBuildContainer {
		return
	}
	socketPath := s.dockerSocketPathFn()
	targetImage := systemUpdateTargetDockerImage("")
	dockerInfo := &SystemUpdateDockerInfo{
		SocketPath:           socketPath,
		TargetImage:          targetImage,
		OneTimeEnableCommand: systemUpdateDockerSocketEnableCommand("nexustok"),
		ManualUpdateCommand:  systemUpdateDockerRunManualCommand("nexustok"),
	}
	info.TargetImage = targetImage
	info.Docker = dockerInfo

	client := systemUpdateDockerClient(socketPath)
	if err := client.ping(ctx); err != nil {
		info.DeploymentMode = systemUpdateDeploymentContainerUnknown
		return
	}
	dockerInfo.SocketAvailable = true
	info.DockerControlAvailable = true

	containerID := s.currentContainerIDFn()
	inspect, err := client.inspectContainer(ctx, containerID)
	if err != nil {
		info.DeploymentMode = systemUpdateDeploymentContainerUnknown
		return
	}
	containerName := cleanDockerContainerName(inspect.Name)
	targetImage = systemUpdateTargetDockerImage(inspect.Config.Image)
	dockerInfo.ContainerID = inspect.ID
	dockerInfo.ContainerName = containerName
	dockerInfo.CurrentImage = inspect.Config.Image
	dockerInfo.CurrentImageID = inspect.Image
	dockerInfo.TargetImage = targetImage
	dockerInfo.HealthStatus, dockerInfo.HealthcheckAvailable, dockerInfo.HealthcheckDegraded = dockerHealthSummary(inspect)
	dockerInfo.OneTimeEnableCommand = systemUpdateDockerSocketEnableCommand(containerName)
	dockerInfo.ManualUpdateCommand = systemUpdateDockerManualCommandFromInspect(inspect, targetImage)
	info.TargetImage = targetImage

	labels := inspect.Config.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	dockerInfo.ComposeProject = labels["com.docker.compose.project"]
	dockerInfo.ComposeService = labels["com.docker.compose.service"]
	dockerInfo.ComposeWorkingDir = labels["com.docker.compose.project.working_dir"]
	dockerInfo.ComposeConfigFiles = labels["com.docker.compose.project.config_files"]
	if dockerInfo.ComposeProject != "" && dockerInfo.ComposeService != "" {
		info.DeploymentMode = systemUpdateDeploymentDockerCompose
		return
	}
	info.DeploymentMode = systemUpdateDeploymentDockerRun
}

func systemUpdateDockerSocketEnableCommand(containerName string) string {
	name := cleanDockerContainerName(containerName)
	quotedName := shellQuote(name)
	return fmt.Sprintf(`docker rm -f %[1]s 2>/dev/null || true

docker run --name %[1]s -d --restart always \
  -p 3030:3030 \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest`, quotedName, quotedName)
}

func systemUpdateDockerRunManualCommand(containerName string) string {
	name := cleanDockerContainerName(containerName)
	quotedName := shellQuote(name)
	return fmt.Sprintf(`docker pull c1cadabob/nexustok:latest
docker stop %[1]s
docker rm %[1]s
# 使用原挂载目录重新 docker run，并建议挂载 /var/run/docker.sock 以启用后台自动更新。`, quotedName)
}

func systemUpdateDockerManualCommandFromInspect(inspect *dockerInspectContainer, targetImage string) string {
	if inspect == nil {
		return systemUpdateDockerRunManualCommand("nexustok")
	}
	name := cleanDockerContainerName(inspect.Name)
	if inspect.Config.Labels != nil {
		project := inspect.Config.Labels["com.docker.compose.project"]
		service := inspect.Config.Labels["com.docker.compose.service"]
		if project != "" && service != "" {
			command := fmt.Sprintf(
				"docker compose pull %s\ndocker compose up -d --no-deps %s",
				shellQuote(service),
				shellQuote(service),
			)
			if workingDir := inspect.Config.Labels["com.docker.compose.project.working_dir"]; workingDir != "" {
				command = "cd " + shellQuote(workingDir) + "\n" + command
			}
			return command
		}
	}
	return minimalSystemUpdateDockerRunCommand(targetImage, name, dockerRuntimeBinds(inspect), inspect.HostConfig.PortBindings, inspect.Config.Env)
}

func (s *SystemUpdateService) DockerRollbackAvailable(ctx context.Context) bool {
	return false
}

func (s *SystemUpdateService) HandOffDockerUpdate(ctx context.Context, task *model.SystemTask, runnerID string, info *SystemUpdateInfo, targetVersion string) (*SystemUpdateTaskResult, error) {
	if task == nil {
		return s.performDockerUpdateInProcess(ctx, nil, runnerID, dockerHelperOptions{
			Action:        systemUpdateDockerHelperActionUpdate,
			TargetImage:   infoTargetImage(info),
			TargetVersion: targetVersion,
		})
	}
	targetImage := infoTargetImage(info)
	return nil, s.startDockerHelper(ctx, task, runnerID, dockerHelperOptions{
		Action:        systemUpdateDockerHelperActionUpdate,
		Mode:          firstNonEmptySystemUpdateString(infoDeploymentMode(info), systemUpdateDeploymentDockerRun),
		TargetImage:   targetImage,
		TargetVersion: targetVersion,
	})
}

func (s *SystemUpdateService) HandOffDockerRollback(ctx context.Context, task *model.SystemTask, runnerID string, info *SystemUpdateInfo, targetVersion string) (*SystemUpdateTaskResult, error) {
	if task == nil {
		return s.performDockerUpdateInProcess(ctx, nil, runnerID, dockerHelperOptions{
			Action:        systemUpdateDockerHelperActionRollback,
			Mode:          firstNonEmptySystemUpdateString(infoDeploymentMode(info), systemUpdateDeploymentDockerRun),
			TargetImage:   infoTargetImage(info),
			TargetVersion: targetVersion,
		})
	}
	return nil, s.startDockerHelper(ctx, task, runnerID, dockerHelperOptions{
		Action:        systemUpdateDockerHelperActionRollback,
		Mode:          firstNonEmptySystemUpdateString(infoDeploymentMode(info), systemUpdateDeploymentDockerRun),
		TargetImage:   infoTargetImage(info),
		TargetVersion: targetVersion,
	})
}

func infoDeploymentMode(info *SystemUpdateInfo) string {
	if info == nil {
		return ""
	}
	return info.DeploymentMode
}

func infoTargetImage(info *SystemUpdateInfo) string {
	if info == nil {
		return ""
	}
	return info.TargetImage
}

func (s *SystemUpdateService) startDockerHelper(ctx context.Context, task *model.SystemTask, runnerID string, options dockerHelperOptions) error {
	state := SystemUpdateTaskState{
		Phase:         SystemUpdatePhaseStartingHelper,
		Progress:      12,
		TargetVersion: options.TargetVersion,
		Mode:          options.Mode,
		TargetImage:   options.TargetImage,
	}
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return err
	}

	socketPath := s.dockerSocketPathFn()
	client := systemUpdateDockerClient(socketPath)
	if err := client.ping(ctx); err != nil {
		return fmt.Errorf("%w: Docker socket is not available: %v", ErrSystemUpdateDisabled, err)
	}
	currentContainerID := s.currentContainerIDFn()
	inspect, err := client.inspectContainer(ctx, currentContainerID)
	if err != nil {
		return fmt.Errorf("inspect current Docker container failed: %w", err)
	}
	containerName := cleanDockerContainerName(inspect.Name)
	containerName = canonicalDockerContainerName(containerName)
	if cleanDockerContainerName(inspect.Name) != containerName {
		return fmt.Errorf("当前 Docker 容器 %q 不是正式服务容器，拒绝创建更新 helper", cleanDockerContainerName(inspect.Name))
	}
	if options.TargetImage == "" {
		options.TargetImage = systemUpdateTargetDockerImage(inspect.Config.Image)
	}
	if options.Mode == "" {
		options.Mode = systemUpdateDeploymentDockerRun
	}
	helperMounts, err := dockerHelperMountsForMode(inspect, socketPath, options.Mode)
	if err != nil {
		return fmt.Errorf("prepare Docker update helper mounts failed: %w", err)
	}
	helperRunnerID := fmt.Sprintf("%s-helper-%s", runnerID, common.GetRandomString(6))
	helperName := fmt.Sprintf("%s-update-helper-%s", containerName, common.GetRandomString(6))
	helperImage := firstNonEmptySystemUpdateString(inspect.Config.Image, options.TargetImage, systemUpdateDefaultDockerImage)

	helperRequest := dockerCreateContainerRequest{
		Image:      helperImage,
		Hostname:   helperName,
		Env:        dockerMergeEnv(inspect.Config.Env, dockerHelperEnv(task.TaskID, helperRunnerID, options, inspect.ID, helperName)),
		Cmd:        []string{"system-update-helper"},
		Entrypoint: dockerHelperEntrypoint(inspect.Config.Entrypoint),
		WorkingDir: firstNonEmptySystemUpdateString(inspect.Config.WorkingDir, "/data"),
		Labels: map[string]string{
			"dev.c1cada.nexustok.system_update_helper": "true",
			"dev.c1cada.nexustok.system_update_task":   task.TaskID,
		},
		HostConfig: dockerHostConfig{
			Binds:       helperMounts.Binds,
			Mounts:      helperMounts.Mounts,
			NetworkMode: inspect.HostConfig.NetworkMode,
		},
	}

	helperID, err := client.createContainer(ctx, helperName, helperRequest)
	if err != nil {
		return fmt.Errorf("create Docker update helper failed: %w", err)
	}
	if strings.TrimSpace(helperID) == "" {
		return errors.New("create Docker update helper returned an empty container ID")
	}
	cleanupHelper := func() error {
		recoveryCtx, cancel := systemUpdateDockerRecoveryContext()
		defer cancel()
		return client.removeContainer(recoveryCtx, helperID, true)
	}
	transferred := false
	if err := model.TransferSystemTaskLock(task.TaskID, task.Type, runnerID, helperRunnerID, common.GetTimestamp()+600); err != nil {
		_ = cleanupHelper()
		return fmt.Errorf("transfer system update task to helper failed: %w", err)
	}
	transferred = true
	restoreLease := func(operationErr error) error {
		if !transferred {
			return operationErr
		}
		if transferErr := model.TransferSystemTaskLock(task.TaskID, task.Type, helperRunnerID, runnerID, common.GetTimestamp()+60); transferErr != nil {
			return fmt.Errorf("%w; transfer system update task back to runner failed: %s", operationErr, SystemUpdateErrorMessage(transferErr))
		}
		return operationErr
	}
	if err := client.startContainer(ctx, helperID); err != nil {
		cleanupErr := cleanupHelper()
		restoredErr := restoreLease(err)
		if cleanupErr != nil && !isDockerNotFoundError(cleanupErr) {
			restoredErr = fmt.Errorf("%w; remove Docker update helper failed: %s", restoredErr, cleanupErr)
		}
		return fmt.Errorf("start Docker update helper failed: %w", restoredErr)
	}
	if err := waitDockerHelperStarted(ctx, client, helperID, task.TaskID); err != nil {
		if errors.Is(err, ErrSystemUpdateHandedOff) {
			_ = cleanupHelper()
			return ErrSystemUpdateHandedOff
		}
		cleanupErr := cleanupHelper()
		restoredErr := restoreLease(err)
		if cleanupErr != nil && !isDockerNotFoundError(cleanupErr) {
			restoredErr = fmt.Errorf("%w; remove Docker update helper failed: %s", restoredErr, cleanupErr)
		}
		return fmt.Errorf("Docker update helper did not stay running: %w", restoredErr)
	}
	return ErrSystemUpdateHandedOff
}

func dockerHelperEnv(taskID string, runnerID string, options dockerHelperOptions, currentContainerID string, containerName string) []string {
	return []string{
		systemUpdateHelperEnvTaskID + "=" + taskID,
		systemUpdateHelperEnvRunnerID + "=" + runnerID,
		systemUpdateHelperEnvAction + "=" + options.Action,
		systemUpdateHelperEnvMode + "=" + options.Mode,
		systemUpdateHelperEnvCurrentContainerID + "=" + currentContainerID,
		systemUpdateHelperEnvTargetImage + "=" + options.TargetImage,
		systemUpdateHelperEnvTargetVersion + "=" + options.TargetVersion,
		systemUpdateHelperEnvBackupID + "=" + options.BackupID,
		systemUpdateHelperEnvContainerName + "=" + containerName,
	}
}

func dockerHelperEntrypoint(current []string) []string {
	if len(current) > 0 {
		return append([]string(nil), current...)
	}
	return []string{"/nexustok"}
}

func dockerMergeEnv(base []string, overrides []string) []string {
	values := map[string]string{}
	order := make([]string, 0, len(base)+len(overrides))
	add := func(item string) {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}
	for _, item := range base {
		add(item)
	}
	for _, item := range overrides {
		add(item)
	}
	result := make([]string, 0, len(order))
	for _, key := range order {
		result = append(result, key+"="+values[key])
	}
	return result
}

func dockerHelperMountsForMode(inspect *dockerInspectContainer, socketPath string, deploymentMode string) (dockerHelperMountConfig, error) {
	if inspect == nil {
		return dockerHelperMountConfig{}, errors.New("Docker container metadata is unavailable")
	}
	binds := append([]string(nil), inspect.HostConfig.Binds...)
	mounts := make([]dockerMountSpec, 0)
	seenDestinations := map[string]struct{}{}
	composeMounts := map[string]dockerHelperMount{}
	for _, bind := range binds {
		if destination := dockerBindDestination(bind); destination != "" {
			seenDestinations[destination] = struct{}{}
			parts := strings.Split(bind, ":")
			writable := true
			if len(parts) > 2 && strings.HasPrefix(parts[2], "ro") {
				writable = false
			}
			composeMounts[destination] = dockerHelperMount{Source: parts[0], Writable: writable}
		}
	}
	for _, mount := range inspect.Mounts {
		if mount.Destination == "" {
			continue
		}
		if _, exists := seenDestinations[mount.Destination]; exists {
			continue
		}
		source := mount.Source
		if mount.Type == "volume" && mount.Name != "" {
			source = mount.Name
		}
		if source == "" {
			continue
		}
		mode := "rw"
		if !mount.RW {
			mode = "ro"
		}
		binds = append(binds, source+":"+mount.Destination+":"+mode)
		seenDestinations[mount.Destination] = struct{}{}
		composeMounts[mount.Destination] = dockerHelperMount{Source: source, Writable: mount.RW}
	}
	if deploymentMode == systemUpdateDeploymentDockerCompose {
		composeDirectories, err := dockerComposeConfigDirectories(inspect)
		if err != nil {
			return dockerHelperMountConfig{}, err
		}
		for _, directory := range composeDirectories {
			if existingMount, exists := composeMounts[directory]; exists {
				if existingMount.Source != directory || !existingMount.Writable {
					return dockerHelperMountConfig{}, fmt.Errorf("Docker Compose configuration directory %q is already mounted incompatibly", directory)
				}
				continue
			}
			if _, exists := seenDestinations[directory]; exists {
				return dockerHelperMountConfig{}, fmt.Errorf("Docker Compose configuration directory %q conflicts with an existing helper mount", directory)
			}
			mounts = append(mounts, dockerMountSpec{
				Type:   "bind",
				Source: directory,
				Target: directory,
			})
			seenDestinations[directory] = struct{}{}
			composeMounts[directory] = dockerHelperMount{Source: directory, Writable: true}
		}
	}
	socketPath = firstNonEmptySystemUpdateString(socketPath, systemUpdateDockerSockDefault)
	if _, exists := seenDestinations[socketPath]; !exists {
		binds = append(binds, socketPath+":"+socketPath)
	}
	return dockerHelperMountConfig{Binds: binds, Mounts: mounts}, nil
}

type dockerHelperMount struct {
	Source   string
	Writable bool
}

func dockerBindDestination(bind string) string {
	parts := strings.Split(bind, ":")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// CleanupSystemUpdateDockerHelper 清理当前 Docker 更新 helper 容器。
//
// 该函数必须能够在数据库初始化失败时运行，因此不依赖 model.DB 或 SystemTask。
func CleanupSystemUpdateDockerHelper(ctx context.Context) {
	containerName := strings.TrimSpace(os.Getenv(systemUpdateHelperEnvContainerName))
	if containerName == "" {
		return
	}
	cleanupCtx, cancel := systemUpdateDockerRecoveryContext()
	defer cancel()
	client := systemUpdateDockerClient(systemUpdateDockerSocketPath())
	if err := client.removeContainer(cleanupCtx, containerName, true); err != nil && !isDockerNotFoundError(err) {
		logger.LogWarn(ctx, fmt.Sprintf("清理 Docker 更新 helper %s 失败：%s", containerName, SystemUpdateErrorMessage(err)))
	}
}

// RunSystemUpdateDockerHelper 在独立 helper 容器中执行 Docker 更新或回滚。
//
// helper 只初始化数据库并持有 SystemTask 租约，不启动 HTTP 服务。主容器被停止后，
// helper 仍能通过同一个 Docker socket 和同一份数据库/数据卷写入任务终态。
func RunSystemUpdateDockerHelper(ctx context.Context) error {
	options := dockerHelperOptions{
		TaskID:             strings.TrimSpace(os.Getenv(systemUpdateHelperEnvTaskID)),
		RunnerID:           strings.TrimSpace(os.Getenv(systemUpdateHelperEnvRunnerID)),
		Action:             strings.TrimSpace(os.Getenv(systemUpdateHelperEnvAction)),
		Mode:               strings.TrimSpace(os.Getenv(systemUpdateHelperEnvMode)),
		CurrentContainerID: strings.TrimSpace(os.Getenv(systemUpdateHelperEnvCurrentContainerID)),
		TargetImage:        strings.TrimSpace(os.Getenv(systemUpdateHelperEnvTargetImage)),
		TargetVersion:      strings.TrimSpace(os.Getenv(systemUpdateHelperEnvTargetVersion)),
		BackupID:           strings.TrimSpace(os.Getenv(systemUpdateHelperEnvBackupID)),
		ContainerName:      strings.TrimSpace(os.Getenv(systemUpdateHelperEnvContainerName)),
	}
	defer CleanupSystemUpdateDockerHelper(ctx)
	if options.TaskID == "" || options.RunnerID == "" {
		return errors.New("Docker update helper missing task id or runner id")
	}
	task, err := model.GetSystemTaskByTaskID(options.TaskID)
	if err != nil {
		return errors.New(SystemUpdateErrorMessage(err))
	}
	if task == nil {
		return fmt.Errorf("Docker update helper task not found: %s", options.TaskID)
	}
	if task.Status != model.SystemTaskStatusRunning || task.LockedBy != options.RunnerID {
		return model.ErrSystemTaskLockLost
	}

	finishTask := func(result *SystemUpdateTaskResult, runErr error) error {
		status := model.SystemTaskStatusSucceeded
		errorMessage := ""
		if runErr != nil {
			status = model.SystemTaskStatusFailed
			errorMessage = SystemUpdateErrorMessage(runErr)
			logger.LogWarn(ctx, fmt.Sprintf("Docker system update helper task %s failed: %s", options.TaskID, errorMessage))
		}
		if finishErr := model.FinishSystemTask(options.TaskID, options.RunnerID, status, result, errorMessage); finishErr != nil {
			return fmt.Errorf("finish Docker update helper task failed: %s", SystemUpdateErrorMessage(finishErr))
		}
		if runErr != nil {
			return errors.New(errorMessage)
		}
		return nil
	}

	if options.Action != systemUpdateDockerHelperActionUpdate &&
		options.Action != systemUpdateDockerHelperActionRollback {
		return finishTask(nil, errors.New("Docker update helper action is invalid"))
	}

	operationCtx, cancelOperation := context.WithCancel(ctx)
	defer cancelOperation()
	heartbeatDone := make(chan struct{})
	heartbeatStopped := make(chan struct{})
	heartbeatErr := make(chan error, 1)
	go func() {
		defer close(heartbeatStopped)
		ticker := time.NewTicker(systemTaskLockTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatDone:
				return
			case <-ctx.Done():
				cancelOperation()
				return
			case <-ticker.C:
				if err := model.RenewSystemTaskLock(options.TaskID, options.RunnerID, common.GetTimestamp()+int64(systemTaskLockTTL.Seconds())); err != nil {
					select {
					case heartbeatErr <- err:
					default:
					}
					logger.LogWarn(ctx, fmt.Sprintf("Docker update helper failed to renew task lock: %s", SystemUpdateErrorMessage(err)))
					cancelOperation()
					return
				}
			}
		}
	}()

	var result *SystemUpdateTaskResult
	result, err = defaultSystemUpdateService.performDockerUpdateInProcess(operationCtx, task, options.RunnerID, options)
	close(heartbeatDone)
	<-heartbeatStopped
	select {
	case lockErr := <-heartbeatErr:
		if err == nil || errors.Is(err, context.Canceled) {
			err = fmt.Errorf("Docker update helper lost the system task lease: %w", lockErr)
		}
	default:
	}
	return finishTask(result, err)
}

// performDockerRollbackInProcess 保留内部兼容入口；回滚与更新现在都按目标镜像执行同一套切换流程。
func (s *SystemUpdateService) performDockerRollbackInProcess(ctx context.Context, task *model.SystemTask, runnerID string, options dockerHelperOptions) (*SystemUpdateTaskResult, error) {
	options.Action = systemUpdateDockerHelperActionRollback
	return s.performDockerUpdateInProcess(ctx, task, runnerID, options)
}

func (s *SystemUpdateService) performDockerUpdateInProcess(ctx context.Context, task *model.SystemTask, runnerID string, options dockerHelperOptions) (*SystemUpdateTaskResult, error) {
	client := systemUpdateDockerClient(s.dockerSocketPathFn())
	if err := client.ping(ctx); err != nil {
		return nil, fmt.Errorf("Docker socket is not available: %w", err)
	}
	currentID := firstNonEmptySystemUpdateString(options.CurrentContainerID, s.currentContainerIDFn())
	current, err := client.inspectContainer(ctx, currentID)
	if err != nil {
		return nil, fmt.Errorf("inspect current Docker container failed: %w", err)
	}
	containerName := canonicalDockerContainerName(current.Name)
	if cleanDockerContainerName(current.Name) != containerName {
		return nil, fmt.Errorf("当前 Docker 容器 %q 不是正式服务容器，拒绝从辅助容器继续更新", cleanDockerContainerName(current.Name))
	}
	legacyBackupName := containerName + ".backup"
	if err := client.removeContainer(ctx, legacyBackupName, true); err != nil && !isDockerNotFoundError(err) {
		logger.LogWarn(ctx, fmt.Sprintf("remove legacy Docker backup container %s failed: %s", legacyBackupName, maskSystemUpdateSensitiveInfo(err.Error())))
	}
	targetImage := firstNonEmptySystemUpdateString(options.TargetImage, systemUpdateTargetDockerImage(current.Config.Image))
	if options.TargetVersion == "" {
		_, tag := splitDockerImage(targetImage)
		if tag != "latest" {
			options.TargetVersion = tag
		}
	}
	state := SystemUpdateTaskState{
		Phase:         SystemUpdatePhasePullingImage,
		Progress:      10,
		Mode:          firstNonEmptySystemUpdateString(options.Mode, systemUpdateDeploymentDockerRun),
		TargetVersion: options.TargetVersion,
		TargetImage:   targetImage,
	}
	if task != nil {
		payload := SystemUpdateTaskPayload{}
		if err := task.DecodePayload(&payload); err == nil {
			if options.TargetVersion == "" {
				options.TargetVersion = payload.TargetVersion
				state.TargetVersion = payload.TargetVersion
			}
			if options.BackupID == "" {
				options.BackupID = payload.BackupID
			}
		}
	}
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	lastPullUpdate := time.Time{}
	if err := client.pullImage(ctx, targetImage, func(_ string) {
		if time.Since(lastPullUpdate) < 2*time.Second {
			return
		}
		lastPullUpdate = time.Now()
		_ = updateSystemUpdateTaskState(task, runnerID, state)
	}); err != nil {
		return nil, formatDockerImagePullError(targetImage, err)

	}
	state.Phase = SystemUpdatePhaseProbing
	state.Progress = 35
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}
	preflightReadiness, err := s.preflightDockerContainer(ctx, client, current, containerName, targetImage)
	if err != nil {
		return nil, err
	}
	if preflightReadiness.Status != "" {
		state.HealthStatus = preflightReadiness.Status
		state.HealthcheckAvailable = preflightReadiness.HealthcheckAvailable
		state.HealthcheckDegraded = preflightReadiness.HealthcheckDegraded
	}
	state.Phase = SystemUpdatePhaseBackingUpData
	state.Progress = 45
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}
	backup, err := s.createUpdateBackup(ctx, task, runnerID, options.TargetVersion, state)
	if err != nil {
		return nil, err
	}
	options.BackupID = backup.ID
	state.BackupID = backup.ID
	state.BackupStatus = backup.Status
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}
	var composeBackup []systemUpdateComposeFileBackup
	if state.Mode == systemUpdateDeploymentDockerCompose {
		composeBackup, err = prepareSystemUpdateComposeImage(current, targetImage)
		if err != nil {
			return nil, err
		}
	}
	composeChanged := len(composeBackup) > 0
	restoreCompose := func(operationErr error) error {
		if !composeChanged {
			return operationErr
		}
		if restoreErr := restoreSystemUpdateComposeFiles(composeBackup); restoreErr != nil {
			return fmt.Errorf("%w; restore Docker Compose configuration failed: %s", operationErr, restoreErr)
		}
		return operationErr
	}

	state.Phase = SystemUpdatePhaseRecreatingContainer
	state.Progress = 55
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	newContainerID, readiness, switched, err := s.recreateDockerContainer(ctx, client, current, containerName, targetImage, preflightReadiness)
	if err != nil {
		if switched {
			err = s.restoreSystemUpdateDataAfterDockerFailure(ctx, task, runnerID, state, options.BackupID, err)
		}
		return nil, restoreCompose(err)
	}

	state.Phase = SystemUpdatePhaseProbing
	state.Progress = 90
	state.HealthStatus = readiness.Status
	state.HealthcheckAvailable = readiness.HealthcheckAvailable
	state.HealthcheckDegraded = readiness.HealthcheckDegraded
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	state.Phase = SystemUpdatePhaseReady
	state.Progress = 100
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}
	return &SystemUpdateTaskResult{
		CurrentVersion:       s.currentVersionFn(),
		TargetVersion:        options.TargetVersion,
		Mode:                 state.Mode,
		TargetImage:          targetImage,
		OldContainerID:       current.ID,
		NewContainerID:       newContainerID,
		RestartRequired:      false,
		RollbackAvailable:    false,
		BackupID:             backup.ID,
		BackupStatus:         backup.Status,
		HealthStatus:         readiness.Status,
		HealthcheckAvailable: readiness.HealthcheckAvailable,
		HealthcheckDegraded:  readiness.HealthcheckDegraded,
	}, nil
}

func (s *SystemUpdateService) restoreSystemUpdateDataAfterDockerFailure(
	ctx context.Context,
	task *model.SystemTask,
	runnerID string,
	state SystemUpdateTaskState,
	backupID string,
	operationErr error,
) error {
	if backupID == "" || s.restoreFn == nil {
		return operationErr
	}
	state.Phase = SystemUpdatePhaseRestoringData
	state.Progress = 95
	state.RestoreStatus = systemUpdateRestoreStatusNone
	_ = updateSystemUpdateTaskState(task, runnerID, state)
	if err := s.restoreFn(ctx, backupID); err != nil {
		state.RestoreStatus = systemUpdateRestoreStatusFailed
		state.DataRestoreFailed = true
		_ = updateSystemUpdateTaskState(task, runnerID, state)
		return fmt.Errorf("%w; data_restore_failed: %s", operationErr, SystemUpdateErrorMessage(err))
	}
	state.RestoreStatus = systemUpdateRestoreStatusReady
	_ = updateSystemUpdateTaskState(task, runnerID, state)
	return operationErr
}

func prepareSystemUpdateComposeImage(current *dockerInspectContainer, targetImage string) ([]systemUpdateComposeFileBackup, error) {
	paths, err := dockerComposeConfigFiles(current)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Config.Labels == nil {
		return nil, errors.New("Docker Compose metadata is unavailable")
	}
	serviceName := strings.TrimSpace(current.Config.Labels["com.docker.compose.service"])
	if serviceName == "" {
		return nil, errors.New("Docker Compose service name is unavailable")
	}
	updates := make([]systemUpdateComposeFileUpdate, 0, len(paths))
	for _, path := range paths {
		fileInfo, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("stat Docker Compose configuration failed: %w", err)
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
			return nil, errors.New("Docker Compose configuration must be a regular file and must not be a symbolic link")
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
			return nil, errors.New("Docker Compose configuration must be a regular file and must not be a symbolic link")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read Docker Compose configuration failed: %w", err)
		}
		var document yaml.Node
		if err := yaml.Unmarshal(data, &document); err != nil {
			return nil, fmt.Errorf("parse Docker Compose configuration failed: %w", err)
		}
		if !replaceComposeServiceImage(&document, serviceName, targetImage) {
			continue
		}
		updated, err := yaml.Marshal(&document)
		if err != nil {
			return nil, fmt.Errorf("encode Docker Compose configuration failed: %w", err)
		}
		updates = append(updates, systemUpdateComposeFileUpdate{
			Backup:  systemUpdateComposeFileBackup{Path: path, Mode: fileInfo.Mode().Perm(), Data: data},
			Updated: updated,
		})
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("Docker Compose service %q image field was not found", serviceName)
	}
	backups := make([]systemUpdateComposeFileBackup, 0, len(updates))
	for _, update := range updates {
		if err := atomicWriteSystemUpdateFile(update.Backup.Path, update.Updated, update.Backup.Mode); err != nil {
			if restoreErr := restoreSystemUpdateComposeFiles(backups); restoreErr != nil {
				return nil, fmt.Errorf("%w; restore Docker Compose configuration failed: %v", err, restoreErr)
			}
			return nil, err
		}
		backups = append(backups, update.Backup)
	}
	return backups, nil
}

func dockerComposeConfigFiles(current *dockerInspectContainer) ([]string, error) {
	if current == nil || current.Config.Labels == nil {
		return nil, errors.New("Docker Compose metadata is unavailable")
	}
	configFiles := strings.TrimSpace(current.Config.Labels["com.docker.compose.project.config_files"])
	if configFiles == "" {
		return nil, errors.New("Docker Compose configuration file is unavailable")
	}
	workingDir := strings.TrimSpace(current.Config.Labels["com.docker.compose.project.working_dir"])
	if workingDir != "" {
		if !filepath.IsAbs(workingDir) {
			return nil, errors.New("Docker Compose working directory must be an absolute path")
		}
		absoluteWorkingDir, err := filepath.Abs(workingDir)
		if err != nil {
			return nil, fmt.Errorf("resolve Docker Compose working directory failed: %w", err)
		}
		workingDir = absoluteWorkingDir
	}
	paths := strings.FieldsFunc(configFiles, func(char rune) bool {
		return char == ',' || (os.PathListSeparator == ';' && char == os.PathListSeparator)
	})
	resolved := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, rawPath := range paths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			if workingDir == "" {
				return nil, errors.New("Docker Compose working directory is unavailable for a relative configuration file")
			}
			path = filepath.Join(workingDir, path)
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve Docker Compose configuration failed: %w", err)
		}
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		resolved = append(resolved, path)
	}
	if len(resolved) == 0 {
		return nil, errors.New("Docker Compose configuration file is unavailable")
	}
	return resolved, nil
}

func dockerComposeConfigDirectories(current *dockerInspectContainer) ([]string, error) {
	paths, err := dockerComposeConfigFiles(current)
	if err != nil {
		return nil, err
	}
	directories := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		directory, err := filepath.Abs(filepath.Dir(path))
		if err != nil {
			return nil, fmt.Errorf("resolve Docker Compose configuration directory failed: %w", err)
		}
		if _, exists := seen[directory]; exists {
			continue
		}
		seen[directory] = struct{}{}
		directories = append(directories, directory)
	}
	return directories, nil
}

func replaceComposeServiceImage(document *yaml.Node, serviceName string, targetImage string) bool {
	if document == nil || len(document.Content) == 0 {
		return false
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "services" {
			continue
		}
		services := root.Content[i+1]
		if services.Kind != yaml.MappingNode {
			return false
		}
		for j := 0; j+1 < len(services.Content); j += 2 {
			if services.Content[j].Value != serviceName {
				continue
			}
			service := services.Content[j+1]
			if service.Kind != yaml.MappingNode {
				return false
			}
			for k := 0; k+1 < len(service.Content); k += 2 {
				if service.Content[k].Value == "image" {
					service.Content[k+1].Value = targetImage
					service.Content[k+1].Tag = "!!str"
					return true
				}
			}
			service.Content = append(service.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "image", Tag: "!!str"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: targetImage, Tag: "!!str"},
			)
			return true
		}
	}
	return false
}

func atomicWriteSystemUpdateFile(path string, data []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".nexustok-compose-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(mode); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func restoreSystemUpdateComposeFiles(backups []systemUpdateComposeFileBackup) error {
	var restoreErrs []string
	for _, backup := range backups {
		if err := atomicWriteSystemUpdateFile(backup.Path, backup.Data, backup.Mode); err != nil {
			restoreErrs = append(restoreErrs, err.Error())
		}
	}
	if len(restoreErrs) > 0 {
		return errors.New(strings.Join(restoreErrs, "; "))
	}
	return nil
}

type dockerContainerReadiness struct {
	Status               string
	HealthcheckAvailable bool
	HealthcheckDegraded  bool
}

func (s *SystemUpdateService) preflightDockerContainer(
	ctx context.Context,
	client *dockerEngineClient,
	current *dockerInspectContainer,
	containerName string,
	targetImage string,
) (dockerContainerReadiness, error) {
	if !dockerCanRunParallelPreflight(current) {
		return dockerContainerReadiness{}, nil
	}
	preflightRequest := dockerCreateRequestFromInspectForMode(current, targetImage, dockerCreateRequestModePreflight)
	preflightName := dockerPreflightContainerName(containerName)
	preflightID, err := client.createContainer(ctx, preflightName, preflightRequest)
	if err != nil {
		return dockerContainerReadiness{}, fmt.Errorf("create preflight updated container failed: %w", err)
	}
	if strings.TrimSpace(preflightID) == "" {
		return dockerContainerReadiness{}, errors.New("create preflight updated container returned an empty container ID")
	}
	cleanupPreflight := func(cleanupCtx context.Context) error {
		if err := client.removeContainer(cleanupCtx, preflightID, true); err != nil && !isDockerNotFoundError(err) {
			return err
		}
		return nil
	}
	if err := client.startContainer(ctx, preflightID); err != nil {
		recoveryCtx, cancel := systemUpdateDockerRecoveryContext()
		cleanupErr := cleanupPreflight(recoveryCtx)
		cancel()
		startErr := dockerUpdatedContainerStartError("start preflight updated container failed", err)
		if cleanupErr != nil {
			return dockerContainerReadiness{}, fmt.Errorf("%w; remove failed preflight container failed: %s", startErr, cleanupErr)
		}
		return dockerContainerReadiness{}, startErr
	}
	readiness, err := waitDockerContainerReady(ctx, client, preflightID, 30*time.Second)
	if err != nil {
		recoveryCtx, cancel := systemUpdateDockerRecoveryContext()
		cleanupErr := cleanupPreflight(recoveryCtx)
		cancel()
		if cleanupErr != nil {
			return dockerContainerReadiness{}, fmt.Errorf("%w; remove failed preflight container failed: %s", err, cleanupErr)
		}
		return dockerContainerReadiness{}, err
	}
	recoveryCtx, cancel := systemUpdateDockerRecoveryContext()
	cleanupErr := cleanupPreflight(recoveryCtx)
	cancel()
	if cleanupErr != nil {
		return dockerContainerReadiness{}, fmt.Errorf("remove preflight updated container failed: %w", cleanupErr)
	}
	return readiness, nil
}

func (s *SystemUpdateService) recreateDockerContainer(
	ctx context.Context,
	client *dockerEngineClient,
	current *dockerInspectContainer,
	containerName string,
	targetImage string,
	preflightReadiness dockerContainerReadiness,
) (string, dockerContainerReadiness, bool, error) {
	createRequest := dockerCreateRequestFromInspect(current, targetImage)
	stagingName := dockerStagingContainerName(containerName)
	failedName := dockerFailedContainerName(containerName)
	stagingID, err := client.createContainer(ctx, stagingName, createRequest)
	if err != nil {
		return "", dockerContainerReadiness{}, false, fmt.Errorf("create updated container failed: %w", err)
	}
	if strings.TrimSpace(stagingID) == "" {
		return "", dockerContainerReadiness{}, false, errors.New("create updated container returned an empty container ID")
	}
	cleanupStaging := func() {
		recoveryCtx, cancel := systemUpdateDockerRecoveryContext()
		defer cancel()
		_ = client.removeContainer(recoveryCtx, stagingID, true)
	}

	currentStopped := false
	currentStaged := false
	stagingActive := false
	restore := func(operationErr error) error {
		recoveryCtx, cancel := systemUpdateDockerRecoveryContext()
		defer cancel()
		var restoreErrs []string

		if stagingActive {
			if err := client.renameContainer(recoveryCtx, containerName, stagingName); err != nil {
				restoreErrs = append(restoreErrs, "stage failed updated container: "+err.Error())
			}
		}
		if err := client.removeContainer(recoveryCtx, stagingID, true); err != nil && !isDockerNotFoundError(err) {
			restoreErrs = append(restoreErrs, "remove failed staging container: "+err.Error())
		}
		if currentStaged {
			if err := client.renameContainer(recoveryCtx, failedName, containerName); err != nil {
				restoreErrs = append(restoreErrs, "restore current container: "+err.Error())
			}
		}
		if currentStopped {
			if err := restartDockerContainerAfterUpdateFailure(recoveryCtx, client, current.ID, containerName); err != nil {
				restoreErrs = append(restoreErrs, "restart current container: "+err.Error())
			}
		}
		if len(restoreErrs) > 0 {
			return fmt.Errorf("%w; Docker update recovery failed: %s", operationErr, strings.Join(restoreErrs, "; "))
		}
		return operationErr
	}

	if err := client.stopContainer(ctx, current.ID, 20); err != nil {
		recoveryCtx, cancel := systemUpdateDockerRecoveryContext()
		recoveryErr := restartDockerContainerAfterUpdateFailure(recoveryCtx, client, current.ID, containerName)
		cancel()
		cleanupStaging()
		if recoveryErr != nil {
			return "", dockerContainerReadiness{}, false, fmt.Errorf(
				"stop current container failed: %w; Docker update recovery failed: %s",
				err,
				recoveryErr,
			)
		}
		return "", dockerContainerReadiness{}, false, fmt.Errorf("stop current container failed: %w", err)
	}
	currentStopped = true

	if err := client.renameContainer(ctx, current.ID, failedName); err != nil {
		return "", dockerContainerReadiness{}, false, restore(fmt.Errorf("stage current container failed: %w", err))
	}
	currentStaged = true

	if err := client.renameContainer(ctx, stagingID, containerName); err != nil {
		return "", dockerContainerReadiness{}, false, restore(fmt.Errorf("activate updated container failed: %w", err))
	}
	stagingActive = true

	if err := client.startContainer(ctx, stagingID); err != nil {
		return "", dockerContainerReadiness{}, true, restore(dockerUpdatedContainerStartError("start updated container failed", err))
	}
	readiness, err := waitDockerContainerReady(ctx, client, stagingID, 30*time.Second)
	if err != nil {
		if preflightReadiness.Status != "" {
			err = fmt.Errorf("updated container passed preflight with health status %q but failed after replacing the current container: %w", preflightReadiness.Status, err)
		}
		return "", dockerContainerReadiness{}, true, restore(err)
	}

	currentStaged = false
	stagingActive = false
	currentStopped = false
	if err := client.removeContainer(ctx, failedName, true); err != nil && !isDockerNotFoundError(err) {
		logger.LogWarn(ctx, fmt.Sprintf("remove failed Docker container %s failed: %s", failedName, maskSystemUpdateSensitiveInfo(err.Error())))
	}
	return stagingID, readiness, true, nil
}

func restartDockerContainerAfterUpdateFailure(ctx context.Context, client *dockerEngineClient, containerID string, containerName string) error {
	if err := client.startContainer(ctx, firstNonEmptySystemUpdateString(containerID, containerName)); err != nil {
		return err
	}
	return confirmDockerContainerRunning(ctx, client, firstNonEmptySystemUpdateString(containerID, containerName))
}

func dockerCreateRequestFromInspect(current *dockerInspectContainer, targetImage string) dockerCreateContainerRequest {
	return dockerCreateRequestFromInspectForMode(current, targetImage, dockerCreateRequestModeFinal)
}

func dockerCreateRequestFromInspectForMode(current *dockerInspectContainer, targetImage string, mode dockerCreateRequestMode) dockerCreateContainerRequest {
	labels := map[string]string{}
	for key, value := range current.Config.Labels {
		if mode == dockerCreateRequestModePreflight && strings.HasPrefix(key, "com.docker.compose.") {
			continue
		}
		labels[key] = value
	}
	labels["dev.c1cada.nexustok.updated_by"] = "system_update"
	labels["dev.c1cada.nexustok.updated_at"] = fmt.Sprintf("%d", common.GetTimestamp())
	hostConfig := current.HostConfig
	hostConfig.Binds = dockerRuntimeBinds(current)
	if mode == dockerCreateRequestModePreflight {
		hostConfig.PortBindings = nil
		hostConfig.RestartPolicy = dockerRestartPolicy{Name: "no"}
	}
	return dockerCreateContainerRequest{
		User:         current.Config.User,
		Env:          append([]string(nil), current.Config.Env...),
		Cmd:          append([]string(nil), current.Config.Cmd...),
		Entrypoint:   append([]string(nil), current.Config.Entrypoint...),
		Healthcheck:  current.Config.Healthcheck,
		Image:        targetImage,
		WorkingDir:   current.Config.WorkingDir,
		Labels:       labels,
		ExposedPorts: current.Config.ExposedPorts,
		HostConfig:   hostConfig,
		NetworkingConfig: dockerCreateNetworkingConfig{
			EndpointsConfig: dockerEndpointConfigForCreate(current, mode != dockerCreateRequestModePreflight),
		},
	}
}

func dockerCanRunParallelPreflight(current *dockerInspectContainer) bool {
	if current == nil {
		return false
	}
	networkMode := strings.TrimSpace(strings.ToLower(current.HostConfig.NetworkMode))
	return networkMode != "host" && !strings.HasPrefix(networkMode, "container:")
}

func dockerRuntimeBinds(current *dockerInspectContainer) []string {
	binds := append([]string(nil), current.HostConfig.Binds...)
	seenDestinations := map[string]struct{}{}
	for _, bind := range binds {
		if destination := dockerBindDestination(bind); destination != "" {
			seenDestinations[destination] = struct{}{}
		}
	}
	for _, mount := range current.Mounts {
		if mount.Destination == "" {
			continue
		}
		if _, exists := seenDestinations[mount.Destination]; exists {
			continue
		}
		source := mount.Source
		if mount.Type == "volume" && mount.Name != "" {
			source = mount.Name
		}
		if source == "" {
			continue
		}
		mode := "rw"
		if !mount.RW {
			mode = "ro"
		}
		binds = append(binds, source+":"+mount.Destination+":"+mode)
		seenDestinations[mount.Destination] = struct{}{}
	}
	return binds
}

func dockerEndpointConfigForCreate(current *dockerInspectContainer, includeAliases bool) map[string]dockerEndpointSettings {
	if current == nil || len(current.NetworkSettings.Networks) == 0 {
		return nil
	}
	names := make([]string, 0, len(current.NetworkSettings.Networks))
	for name := range current.NetworkSettings.Networks {
		names = append(names, name)
	}
	sort.Strings(names)
	result := map[string]dockerEndpointSettings{}
	for _, name := range names {
		endpoint := current.NetworkSettings.Networks[name]
		if includeAliases && len(endpoint.Aliases) > 0 {
			result[name] = dockerEndpointSettings{Aliases: append([]string(nil), endpoint.Aliases...)}
		} else {
			result[name] = dockerEndpointSettings{}
		}
	}
	return result
}

func dockerUpdatedContainerStartError(prefix string, err error) error {
	if isDockerPortAllocatedError(err) {
		return fmt.Errorf(
			"%s: Docker host port is already allocated. NexusTok uses a portless preflight container before switching the published service; check whether another host process or container is using the published port. NexusTok attempted to keep or restore the previous container. Last error: %s",
			prefix,
			maskSystemUpdateSensitiveInfo(err.Error()),
		)
	}
	return fmt.Errorf("%s: %s", prefix, maskSystemUpdateSensitiveInfo(err.Error()))
}

func isDockerPortAllocatedError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "port is already allocated") ||
		(strings.Contains(text, "bind for") && strings.Contains(text, "failed"))
}

func waitDockerContainerReady(ctx context.Context, client *dockerEngineClient, containerID string, timeout time.Duration) (dockerContainerReadiness, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		inspect, err := client.inspectContainer(ctx, containerID)
		if err == nil {
			if strings.TrimSpace(inspect.State.Error) != "" {
				return dockerContainerReadiness{}, fmt.Errorf(
					"updated container reported a startup error: %s",
					maskSystemUpdateSensitiveInfo(inspect.State.Error),
				)
			}
			if !inspect.State.Running {
				return dockerContainerReadiness{}, fmt.Errorf(
					"updated container stopped during health check: %s",
					firstNonEmptySystemUpdateString(inspect.State.Error, inspect.State.Status),
				)
			}
			healthStatus, healthcheckAvailable, degraded := dockerHealthSummary(inspect)
			if !healthcheckAvailable {
				return dockerContainerReadiness{
					Status:               healthStatus,
					HealthcheckAvailable: false,
					HealthcheckDegraded:  degraded,
				}, nil
			}
			switch healthStatus {
			case "healthy":
				return dockerContainerReadiness{
					Status:               healthStatus,
					HealthcheckAvailable: true,
				}, nil
			case "unhealthy":
				return dockerContainerReadiness{}, errors.New("updated container health check reported unhealthy")
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return dockerContainerReadiness{}, lastErr
			}
			return dockerContainerReadiness{}, errors.New("updated container did not become healthy before timeout")
		}
		select {
		case <-ctx.Done():
			return dockerContainerReadiness{}, ctx.Err()
		case <-time.After(systemUpdateDockerHealthPollInterval):
		}
	}
}

func waitDockerContainerRunning(ctx context.Context, client *dockerEngineClient, containerID string, timeout time.Duration) error {
	_, err := waitDockerContainerReady(ctx, client, containerID, timeout)
	return err
}

func minimalSystemUpdateDockerRunCommand(targetImage string, name string, binds []string, ports map[string][]dockerPortBinding, env []string) string {
	quotedTargetImage := shellQuote(targetImage)
	quotedName := shellQuote(cleanDockerContainerName(name))
	lines := []string{
		"docker pull " + quotedTargetImage,
		"docker rm -f " + quotedName + " 2>/dev/null || true",
		"docker run --name " + quotedName + " -d --restart always \\",
	}
	for _, portLine := range dockerPortFlagLines(ports) {
		lines = append(lines, "  -p "+shellQuote(portLine)+" \\")
	}
	for _, envLine := range dockerSelectedEnvLines(env) {
		lines = append(lines, "  -e "+shellQuote(envLine)+" \\")
	}
	for _, bind := range binds {
		lines = append(lines, "  -v "+shellQuote(bind)+" \\")
	}
	lines = append(lines, "  "+quotedTargetImage)
	return strings.Join(lines, "\n")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			strings.ContainsRune("_./:@=,+-", char) {
			continue
		}
		return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
	}
	return value
}

func dockerPortFlagLines(ports map[string][]dockerPortBinding) []string {
	lines := []string{}
	for containerPort, bindings := range ports {
		for _, binding := range bindings {
			hostPort := strings.TrimSpace(binding.HostPort)
			if hostPort == "" {
				continue
			}
			hostIP := strings.TrimSpace(binding.HostIP)
			prefix := ""
			if hostIP != "" && hostIP != "0.0.0.0" {
				prefix = hostIP + ":"
			}
			lines = append(lines, fmt.Sprintf("%s%s:%s", prefix, hostPort, containerPortPort(containerPort)))
		}
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return []string{"3030:3030"}
	}
	return lines
}

func containerPortPort(containerPort string) string {
	port, _, _ := strings.Cut(containerPort, "/")
	if port == "" {
		return containerPort
	}
	return port
}

func dockerSelectedEnvLines(env []string) []string {
	allowedPrefixes := []string{"TZ=", "PORT=", "SESSION_SECRET_FILE=", "NODE_NAME="}
	lines := []string{}
	hasPort := false
	for _, item := range env {
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(item, prefix) {
				lines = append(lines, item)
				if prefix == "PORT=" {
					hasPort = true
				}
				break
			}
		}
	}
	if !hasPort {
		lines = append(lines, "PORT=3030")
	}
	sort.Strings(lines)
	return lines
}

func firstNonEmptySystemUpdateString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
