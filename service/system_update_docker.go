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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
)

const (
	systemUpdateDockerSockDefault = "/var/run/docker.sock"

	systemUpdateDockerHelperActionUpdate   = "update"
	systemUpdateDockerHelperActionRollback = "rollback"

	systemUpdateHelperEnvTaskID             = "SYSTEM_UPDATE_HELPER_TASK_ID"
	systemUpdateHelperEnvRunnerID           = "SYSTEM_UPDATE_HELPER_RUNNER_ID"
	systemUpdateHelperEnvAction             = "SYSTEM_UPDATE_HELPER_ACTION"
	systemUpdateHelperEnvMode               = "SYSTEM_UPDATE_HELPER_MODE"
	systemUpdateHelperEnvCurrentContainerID = "SYSTEM_UPDATE_HELPER_CURRENT_CONTAINER_ID"
	systemUpdateHelperEnvTargetImage        = "SYSTEM_UPDATE_HELPER_TARGET_IMAGE"
	systemUpdateHelperEnvBackupName         = "SYSTEM_UPDATE_HELPER_BACKUP_NAME"
)

var containerIDPattern = regexp.MustCompile(`[0-9a-f]{64}`)

type dockerEngineClient struct {
	socketPath string
	httpClient *http.Client
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
	Image        string            `json:"Image,omitempty"`
	Env          []string          `json:"Env,omitempty"`
	Cmd          []string          `json:"Cmd,omitempty"`
	Entrypoint   []string          `json:"Entrypoint,omitempty"`
	WorkingDir   string            `json:"WorkingDir,omitempty"`
	User         string            `json:"User,omitempty"`
	Labels       map[string]string `json:"Labels,omitempty"`
	ExposedPorts map[string]any    `json:"ExposedPorts,omitempty"`
}

type dockerHostConfig struct {
	Binds         []string                       `json:"Binds,omitempty"`
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
	Running bool   `json:"Running"`
	Status  string `json:"Status"`
	Error   string `json:"Error,omitempty"`
}

type dockerCreateContainerRequest struct {
	Hostname         string                       `json:"Hostname,omitempty"`
	User             string                       `json:"User,omitempty"`
	Env              []string                     `json:"Env,omitempty"`
	Cmd              []string                     `json:"Cmd,omitempty"`
	Entrypoint       []string                     `json:"Entrypoint,omitempty"`
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
	TargetImage        string
	BackupName         string
}

func systemUpdateDockerSocketPath() string {
	return strings.TrimSpace(common.GetEnvOrDefaultString("SYSTEM_UPDATE_DOCKER_SOCK", systemUpdateDockerSockDefault))
}

func systemUpdateTargetDockerImage(currentImage string) string {
	override := strings.TrimSpace(common.GetEnvOrDefaultString("SYSTEM_UPDATE_DOCKER_IMAGE", ""))
	if override != "" {
		return override
	}
	currentImage = strings.TrimSpace(currentImage)
	if currentImage == "" || strings.Contains(currentImage, "@sha256:") {
		return systemUpdateDefaultDockerImage
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
		return systemUpdateDefaultDockerImage
	}
	return repo + ":latest"
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
	}
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
		return fmt.Errorf("Docker Engine %s %s returned %d: %s", method, path, resp.StatusCode, common.MaskSensitiveInfo(string(data)))
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
		return fmt.Errorf("Docker image pull returned %d: %s", resp.StatusCode, common.MaskSensitiveInfo(string(data)))
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
				return errors.New(status.ErrorDetail.Message)
			}
			return errors.New(status.Error)
		}
		if onStatus != nil {
			onStatus(firstNonEmptySystemUpdateString(status.Status, status.ID))
		}
	}
	return scanner.Err()
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
	return c.doJSON(ctx, http.MethodPost, "/containers/"+url.PathEscape(idOrName)+"/start", nil, nil)
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

func dockerBackupContainerName(name string) string {
	return cleanDockerContainerName(name) + ".backup"
}

func dockerFailedContainerName(name string) string {
	return cleanDockerContainerName(name) + ".failed-" + time.Now().Format("20060102150405")
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

	client := newDockerEngineClient(socketPath)
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
	return fmt.Sprintf(`docker rm -f %[1]s 2>/dev/null || true

docker run --name %[1]s -d --restart always \
  -p 3030:3030 \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest`, name)
}

func systemUpdateDockerRunManualCommand(containerName string) string {
	name := cleanDockerContainerName(containerName)
	return fmt.Sprintf(`docker pull c1cadabob/nexustok:latest
docker stop %[1]s
docker rm %[1]s
# 使用原挂载目录重新 docker run，并建议挂载 /var/run/docker.sock 以启用后台自动更新。`, name)
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
			command := fmt.Sprintf("docker compose pull %s\ndocker compose up -d --no-deps %s", service, service)
			if workingDir := inspect.Config.Labels["com.docker.compose.project.working_dir"]; workingDir != "" {
				command = "cd " + workingDir + "\n" + command
			}
			return command
		}
	}
	return minimalSystemUpdateDockerRunCommand(targetImage, name, dockerRuntimeBinds(inspect), inspect.HostConfig.PortBindings, inspect.Config.Env)
}

func (s *SystemUpdateService) DockerRollbackAvailable(ctx context.Context) bool {
	socketPath := s.dockerSocketPathFn()
	client := newDockerEngineClient(socketPath)
	if err := client.ping(ctx); err != nil {
		return false
	}
	inspect, err := client.inspectContainer(ctx, s.currentContainerIDFn())
	if err != nil {
		return false
	}
	_, err = client.inspectContainer(ctx, dockerBackupContainerName(inspect.Name))
	return err == nil
}

func (s *SystemUpdateService) HandOffDockerUpdate(ctx context.Context, task *model.SystemTask, runnerID string, info *SystemUpdateInfo) (*SystemUpdateTaskResult, error) {
	if task == nil {
		return s.performDockerUpdateInProcess(ctx, nil, runnerID, dockerHelperOptions{
			Action:      systemUpdateDockerHelperActionUpdate,
			TargetImage: systemUpdateTargetDockerImage(""),
		})
	}
	targetImage := systemUpdateTargetDockerImage("")
	if info != nil && info.TargetImage != "" {
		targetImage = info.TargetImage
	}
	return nil, s.startDockerHelper(ctx, task, runnerID, dockerHelperOptions{
		Action:      systemUpdateDockerHelperActionUpdate,
		Mode:        firstNonEmptySystemUpdateString(infoDeploymentMode(info), systemUpdateDeploymentDockerRun),
		TargetImage: targetImage,
	})
}

func (s *SystemUpdateService) HandOffDockerRollback(ctx context.Context, task *model.SystemTask, runnerID string) (*SystemUpdateTaskResult, error) {
	if task == nil {
		return s.performDockerRollbackInProcess(ctx, nil, runnerID, dockerHelperOptions{Action: systemUpdateDockerHelperActionRollback})
	}
	return nil, s.startDockerHelper(ctx, task, runnerID, dockerHelperOptions{
		Action: systemUpdateDockerHelperActionRollback,
		Mode:   systemUpdateDeploymentDockerRun,
	})
}

func infoDeploymentMode(info *SystemUpdateInfo) string {
	if info == nil {
		return ""
	}
	return info.DeploymentMode
}

func (s *SystemUpdateService) startDockerHelper(ctx context.Context, task *model.SystemTask, runnerID string, options dockerHelperOptions) error {
	state := SystemUpdateTaskState{
		Phase:       SystemUpdatePhaseStartingHelper,
		Progress:    12,
		Mode:        options.Mode,
		TargetImage: options.TargetImage,
	}
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return err
	}

	socketPath := s.dockerSocketPathFn()
	client := newDockerEngineClient(socketPath)
	if err := client.ping(ctx); err != nil {
		return fmt.Errorf("%w: Docker socket is not available: %v", ErrSystemUpdateDisabled, err)
	}
	currentContainerID := s.currentContainerIDFn()
	inspect, err := client.inspectContainer(ctx, currentContainerID)
	if err != nil {
		return fmt.Errorf("inspect current Docker container failed: %w", err)
	}
	containerName := cleanDockerContainerName(inspect.Name)
	if options.TargetImage == "" {
		options.TargetImage = systemUpdateTargetDockerImage(inspect.Config.Image)
	}
	if options.Mode == "" {
		options.Mode = systemUpdateDeploymentDockerRun
	}
	if options.BackupName == "" {
		options.BackupName = dockerBackupContainerName(containerName)
	}
	helperRunnerID := fmt.Sprintf("%s-helper-%s", runnerID, common.GetRandomString(6))
	helperName := fmt.Sprintf("%s-update-helper-%s", containerName, common.GetRandomString(6))
	helperImage := firstNonEmptySystemUpdateString(inspect.Config.Image, options.TargetImage, systemUpdateDefaultDockerImage)

	helperRequest := dockerCreateContainerRequest{
		Image:      helperImage,
		Env:        dockerMergeEnv(inspect.Config.Env, dockerHelperEnv(task.TaskID, helperRunnerID, options, inspect.ID)),
		Cmd:        []string{"system-update-helper"},
		Entrypoint: dockerHelperEntrypoint(inspect.Config.Entrypoint),
		WorkingDir: firstNonEmptySystemUpdateString(inspect.Config.WorkingDir, "/data"),
		Labels: map[string]string{
			"dev.c1cada.nexustok.system_update_helper": "true",
			"dev.c1cada.nexustok.system_update_task":   task.TaskID,
		},
		HostConfig: dockerHostConfig{
			Binds:       dockerHelperBinds(inspect),
			NetworkMode: inspect.HostConfig.NetworkMode,
		},
	}

	helperID, err := client.createContainer(ctx, helperName, helperRequest)
	if err != nil {
		return fmt.Errorf("create Docker update helper failed: %w", err)
	}
	transferred := false
	if err := model.TransferSystemTaskLock(task.TaskID, task.Type, runnerID, helperRunnerID, common.GetTimestamp()+600); err != nil {
		_ = client.removeContainer(ctx, helperID, true)
		return fmt.Errorf("transfer system update task to helper failed: %w", err)
	}
	transferred = true
	if err := client.startContainer(ctx, helperID); err != nil {
		if transferred {
			_ = model.TransferSystemTaskLock(task.TaskID, task.Type, helperRunnerID, runnerID, common.GetTimestamp()+60)
		}
		_ = client.removeContainer(ctx, helperID, true)
		return fmt.Errorf("start Docker update helper failed: %w", err)
	}
	return ErrSystemUpdateHandedOff
}

func dockerHelperEnv(taskID string, runnerID string, options dockerHelperOptions, currentContainerID string) []string {
	return []string{
		systemUpdateHelperEnvTaskID + "=" + taskID,
		systemUpdateHelperEnvRunnerID + "=" + runnerID,
		systemUpdateHelperEnvAction + "=" + options.Action,
		systemUpdateHelperEnvMode + "=" + options.Mode,
		systemUpdateHelperEnvCurrentContainerID + "=" + currentContainerID,
		systemUpdateHelperEnvTargetImage + "=" + options.TargetImage,
		systemUpdateHelperEnvBackupName + "=" + options.BackupName,
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

func dockerHelperBinds(inspect *dockerInspectContainer) []string {
	binds := append([]string(nil), inspect.HostConfig.Binds...)
	seenDestinations := map[string]struct{}{}
	for _, bind := range binds {
		if destination := dockerBindDestination(bind); destination != "" {
			seenDestinations[destination] = struct{}{}
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
	}
	if _, exists := seenDestinations[systemUpdateDockerSockDefault]; !exists {
		binds = append(binds, systemUpdateDockerSockDefault+":"+systemUpdateDockerSockDefault)
	}
	return binds
}

func dockerBindDestination(bind string) string {
	parts := strings.Split(bind, ":")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
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
		BackupName:         strings.TrimSpace(os.Getenv(systemUpdateHelperEnvBackupName)),
	}
	if options.TaskID == "" || options.RunnerID == "" {
		return fmt.Errorf("Docker update helper missing task id or runner id")
	}
	task, err := model.GetSystemTaskByTaskID(options.TaskID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("Docker update helper task not found: %s", options.TaskID)
	}

	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(systemTaskLockTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatDone:
				return
			case <-ticker.C:
				if err := model.RenewSystemTaskLock(options.TaskID, options.RunnerID, common.GetTimestamp()+int64(systemTaskLockTTL.Seconds())); err != nil {
					logger.LogWarn(ctx, fmt.Sprintf("Docker update helper failed to renew task lock: %v", err))
					return
				}
			}
		}
	}()
	defer close(heartbeatDone)

	var result *SystemUpdateTaskResult
	switch options.Action {
	case systemUpdateDockerHelperActionRollback:
		result, err = defaultSystemUpdateService.performDockerRollbackInProcess(ctx, task, options.RunnerID, options)
	default:
		result, err = defaultSystemUpdateService.performDockerUpdateInProcess(ctx, task, options.RunnerID, options)
	}
	status := model.SystemTaskStatusSucceeded
	errorMessage := ""
	if err != nil {
		status = model.SystemTaskStatusFailed
		errorMessage = err.Error()
		logger.LogWarn(ctx, fmt.Sprintf("Docker system update helper task %s failed: %v", options.TaskID, err))
	}
	if finishErr := model.FinishSystemTask(options.TaskID, options.RunnerID, status, result, errorMessage); finishErr != nil {
		return fmt.Errorf("finish Docker update helper task failed: %w", finishErr)
	}
	return err
}

func (s *SystemUpdateService) performDockerUpdateInProcess(ctx context.Context, task *model.SystemTask, runnerID string, options dockerHelperOptions) (*SystemUpdateTaskResult, error) {
	client := newDockerEngineClient(s.dockerSocketPathFn())
	if err := client.ping(ctx); err != nil {
		return nil, fmt.Errorf("Docker socket is not available: %w", err)
	}
	currentID := firstNonEmptySystemUpdateString(options.CurrentContainerID, s.currentContainerIDFn())
	current, err := client.inspectContainer(ctx, currentID)
	if err != nil {
		return nil, fmt.Errorf("inspect current Docker container failed: %w", err)
	}
	containerName := cleanDockerContainerName(current.Name)
	targetImage := firstNonEmptySystemUpdateString(options.TargetImage, systemUpdateTargetDockerImage(current.Config.Image))
	backupName := firstNonEmptySystemUpdateString(options.BackupName, dockerBackupContainerName(containerName))

	state := SystemUpdateTaskState{Phase: SystemUpdatePhasePullingImage, Progress: 20, Mode: firstNonEmptySystemUpdateString(options.Mode, systemUpdateDeploymentDockerRun), TargetImage: targetImage}
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
		return nil, fmt.Errorf("pull Docker image failed: %w", err)
	}

	state.Phase = SystemUpdatePhaseRecreatingContainer
	state.Progress = 55
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}

	_ = client.removeContainer(ctx, backupName, true)
	newContainerID, err := s.recreateDockerContainer(ctx, client, current, containerName, backupName, targetImage)
	if err != nil {
		return nil, err
	}

	state.Phase = SystemUpdatePhaseProbing
	state.Progress = 90
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}
	if err := waitDockerContainerRunning(ctx, client, newContainerID, 30*time.Second); err != nil {
		return nil, err
	}

	state.Phase = SystemUpdatePhaseReady
	state.Progress = 100
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}
	return &SystemUpdateTaskResult{
		CurrentVersion:      s.currentVersionFn(),
		TargetVersion:       options.TargetImage,
		Mode:                state.Mode,
		TargetImage:         targetImage,
		OldContainerID:      current.ID,
		NewContainerID:      newContainerID,
		BackupContainerName: backupName,
		RestartRequired:     false,
		RollbackAvailable:   true,
	}, nil
}

func (s *SystemUpdateService) recreateDockerContainer(ctx context.Context, client *dockerEngineClient, current *dockerInspectContainer, containerName string, backupName string, targetImage string) (string, error) {
	createRequest := dockerCreateRequestFromInspect(current, targetImage)
	if err := client.stopContainer(ctx, current.ID, 20); err != nil {
		return "", fmt.Errorf("stop current container failed: %w", err)
	}
	if err := client.renameContainer(ctx, current.ID, backupName); err != nil {
		return "", fmt.Errorf("rename current container to backup failed: %w", err)
	}

	newContainerID, createErr := client.createContainer(ctx, containerName, createRequest)
	if createErr != nil {
		_ = client.renameContainer(ctx, backupName, containerName)
		_ = client.startContainer(ctx, containerName)
		return "", fmt.Errorf("create updated container failed and backup was restored: %w", createErr)
	}
	if startErr := client.startContainer(ctx, newContainerID); startErr != nil {
		_ = client.removeContainer(ctx, newContainerID, true)
		_ = client.renameContainer(ctx, backupName, containerName)
		_ = client.startContainer(ctx, containerName)
		return "", fmt.Errorf("start updated container failed and backup was restored: %w", startErr)
	}
	return newContainerID, nil
}

func dockerCreateRequestFromInspect(current *dockerInspectContainer, targetImage string) dockerCreateContainerRequest {
	labels := map[string]string{}
	for key, value := range current.Config.Labels {
		labels[key] = value
	}
	labels["dev.c1cada.nexustok.updated_by"] = "system_update"
	labels["dev.c1cada.nexustok.updated_at"] = fmt.Sprintf("%d", common.GetTimestamp())
	hostConfig := current.HostConfig
	hostConfig.Binds = dockerRuntimeBinds(current)
	return dockerCreateContainerRequest{
		User:         current.Config.User,
		Env:          append([]string(nil), current.Config.Env...),
		Cmd:          append([]string(nil), current.Config.Cmd...),
		Entrypoint:   append([]string(nil), current.Config.Entrypoint...),
		Image:        targetImage,
		WorkingDir:   current.Config.WorkingDir,
		Labels:       labels,
		ExposedPorts: current.Config.ExposedPorts,
		HostConfig:   hostConfig,
		NetworkingConfig: dockerCreateNetworkingConfig{
			EndpointsConfig: dockerEndpointConfigForCreate(current),
		},
	}
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

func dockerEndpointConfigForCreate(current *dockerInspectContainer) map[string]dockerEndpointSettings {
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
		if len(endpoint.Aliases) > 0 {
			result[name] = dockerEndpointSettings{Aliases: append([]string(nil), endpoint.Aliases...)}
		} else {
			result[name] = dockerEndpointSettings{}
		}
	}
	return result
}

func waitDockerContainerRunning(ctx context.Context, client *dockerEngineClient, containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		inspect, err := client.inspectContainer(ctx, containerID)
		if err == nil && inspect.State.Running {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return fmt.Errorf("updated container did not enter running state")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (s *SystemUpdateService) performDockerRollbackInProcess(ctx context.Context, task *model.SystemTask, runnerID string, options dockerHelperOptions) (*SystemUpdateTaskResult, error) {
	client := newDockerEngineClient(s.dockerSocketPathFn())
	if err := client.ping(ctx); err != nil {
		return nil, fmt.Errorf("Docker socket is not available: %w", err)
	}
	current, err := client.inspectContainer(ctx, firstNonEmptySystemUpdateString(options.CurrentContainerID, s.currentContainerIDFn()))
	if err != nil {
		return nil, fmt.Errorf("inspect current Docker container failed: %w", err)
	}
	containerName := cleanDockerContainerName(current.Name)
	backupName := firstNonEmptySystemUpdateString(options.BackupName, dockerBackupContainerName(containerName))
	backup, err := client.inspectContainer(ctx, backupName)
	if err != nil {
		return nil, ErrSystemRollbackDisabled
	}
	state := SystemUpdateTaskState{Phase: SystemUpdatePhaseRollingBack, Progress: 40, Mode: systemUpdateDeploymentDockerRun}
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}
	failedName := dockerFailedContainerName(containerName)
	if err := client.stopContainer(ctx, current.ID, 20); err != nil {
		return nil, fmt.Errorf("stop current Docker container failed: %w", err)
	}
	if err := client.renameContainer(ctx, current.ID, failedName); err != nil {
		return nil, fmt.Errorf("rename current Docker container failed: %w", err)
	}
	if err := client.renameContainer(ctx, backup.ID, containerName); err != nil {
		_ = client.renameContainer(ctx, failedName, containerName)
		_ = client.startContainer(ctx, containerName)
		return nil, fmt.Errorf("restore backup Docker container failed: %w", err)
	}
	if err := client.startContainer(ctx, containerName); err != nil {
		return nil, fmt.Errorf("start backup Docker container failed: %w", err)
	}
	_ = client.removeContainer(ctx, failedName, true)

	state.Progress = 100
	state.Phase = SystemUpdatePhaseReady
	if err := updateSystemUpdateTaskState(task, runnerID, state); err != nil {
		return nil, err
	}
	return &SystemUpdateTaskResult{
		CurrentVersion:      s.currentVersionFn(),
		Mode:                systemUpdateDeploymentDockerRun,
		OldContainerID:      current.ID,
		NewContainerID:      backup.ID,
		BackupContainerName: backupName,
		RestartRequired:     false,
		RollbackAvailable:   false,
	}, nil
}

func minimalSystemUpdateDockerRunCommand(targetImage string, name string, binds []string, ports map[string][]dockerPortBinding, env []string) string {
	lines := []string{
		"docker pull " + targetImage,
		"docker rm -f " + cleanDockerContainerName(name) + " 2>/dev/null || true",
		"docker run --name " + cleanDockerContainerName(name) + " -d --restart always \\",
	}
	for _, portLine := range dockerPortFlagLines(ports) {
		lines = append(lines, "  -p "+portLine+" \\")
	}
	for _, envLine := range dockerSelectedEnvLines(env) {
		lines = append(lines, "  -e "+envLine+" \\")
	}
	for _, bind := range binds {
		lines = append(lines, "  -v "+bind+" \\")
	}
	lines = append(lines, "  "+targetImage)
	return strings.Join(lines, "\n")
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
	allowedPrefixes := []string{"TZ=", "PORT=", "SESSION_SECRET_FILE=", "SESSION_SECRET=", "SQL_DSN=", "REDIS_CONN_STRING=", "NODE_NAME="}
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
