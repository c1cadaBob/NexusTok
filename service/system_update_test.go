package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSystemUpdateGitHubClient struct {
	release       *systemUpdateGitHubRelease
	fetchErr      error
	downloadData  []byte
	checksumData  []byte
	requestedRepo string
}

func (c *fakeSystemUpdateGitHubClient) FetchLatestRelease(_ context.Context, repo string) (*systemUpdateGitHubRelease, error) {
	c.requestedRepo = repo
	if c.fetchErr != nil {
		return nil, c.fetchErr
	}
	return c.release, nil
}

func (c *fakeSystemUpdateGitHubClient) DownloadFile(_ context.Context, _ string, dest string, _ int64, onProgress func(downloaded, total int64)) error {
	if err := os.WriteFile(dest, c.downloadData, 0755); err != nil {
		return err
	}
	if onProgress != nil {
		onProgress(int64(len(c.downloadData)), int64(len(c.downloadData)))
	}
	return nil
}

func (c *fakeSystemUpdateGitHubClient) FetchFile(_ context.Context, _ string, _ int64) ([]byte, error) {
	return c.checksumData, nil
}

func TestParseSystemUpdateVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		ok      bool
		parsed  parsedSystemUpdateVersion
	}{
		{
			name:    "with v prefix",
			version: "v1.2.3",
			ok:      true,
			parsed:  parsedSystemUpdateVersion{major: 1, minor: 2, patch: 3},
		},
		{
			name:    "without v prefix",
			version: "1.2.3",
			ok:      true,
			parsed:  parsedSystemUpdateVersion{major: 1, minor: 2, patch: 3},
		},
		{
			name:    "with prerelease and metadata",
			version: "v1.2.3-rc.1+build.7",
			ok:      true,
			parsed:  parsedSystemUpdateVersion{major: 1, minor: 2, patch: 3, prerelease: "rc.1"},
		},
		{
			name:    "invalid",
			version: "dev",
			ok:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, ok := parseSystemUpdateVersion(tt.version)
			require.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.parsed, parsed)
			}
		})
	}
}

func TestCompareParsedSystemUpdateVersionTreatsReleaseAfterPrerelease(t *testing.T) {
	current, ok := parseSystemUpdateVersion("v1.2.3-rc.1")
	require.True(t, ok)
	latest, ok := parseSystemUpdateVersion("v1.2.3")
	require.True(t, ok)

	assert.Equal(t, -1, compareParsedSystemUpdateVersion(current, latest))
	assert.Equal(t, 1, compareParsedSystemUpdateVersion(latest, current))
}

func TestSystemUpdateCheckFindsApplicableLinuxAsset(t *testing.T) {
	service := newTestSystemUpdateService("v1.0.0", "linux", "amd64", false, testSystemUpdateRelease("v1.1.0"))

	info, err := service.CheckLatest(context.Background(), true)

	require.NoError(t, err)
	require.True(t, info.HasUpdate)
	require.True(t, info.CanApply)
	require.NotNil(t, info.MatchedAsset)
	require.NotNil(t, info.ChecksumAsset)
	assert.Equal(t, "nexustok-v1.1.0", info.MatchedAsset.Name)
	assert.Equal(t, "checksums-linux.txt", info.ChecksumAsset.Name)
	assert.Equal(t, systemUpdateBuildRelease, info.BuildType)
	assert.Equal(t, systemUpdateReleaseStatusPublished, info.ReleaseStatus)
}

func TestSystemUpdateCheckMapsGitHubNotFoundToNoReleaseInfoForDocker(t *testing.T) {
	client := &fakeSystemUpdateGitHubClient{fetchErr: ErrSystemUpdateNoRelease}
	service := NewSystemUpdateService(client)
	service.currentVersionFn = func() string { return "v1.0.0" }
	service.goosFn = func() string { return "linux" }
	service.goarchFn = func() string { return "amd64" }
	service.isContainerFn = func() bool { return true }
	service.dockerSocketPathFn = func() string { return filepath.Join(t.TempDir(), "missing-docker.sock") }
	service.executableFn = func() (string, error) { return "", errors.New("not configured") }

	info, err := service.CheckLatest(context.Background(), true)

	require.NoError(t, err)
	assert.True(t, info.HasUpdate)
	assert.False(t, info.CanApply)
	assert.Equal(t, systemUpdateReleaseStatusNone, info.ReleaseStatus)
	assert.Equal(t, systemUpdateBuildContainer, info.BuildType)
	assert.Equal(t, systemUpdateDeploymentContainerUnknown, info.DeploymentMode)
	assert.Equal(t, systemUpdateMethodDockerEngine, info.UpdateMethod)
	assert.Equal(t, systemUpdateComparisonUnknown, info.ComparisonStatus)
	assert.Equal(t, systemUpdateDefaultDockerImage, info.TargetImage)
	assert.Contains(t, info.ApplyDisabledReason, "/var/run/docker.sock")
	assert.Contains(t, info.ManualUpdateHint, "c1cadabob/nexustok:latest")
	require.NotNil(t, info.Docker)
	assert.False(t, info.Docker.SocketAvailable)
	assert.Contains(t, info.Docker.OneTimeEnableCommand, "/var/run/docker.sock")
}

func TestSystemUpdateGitHubRepoCanBeOverriddenByEnv(t *testing.T) {
	t.Setenv("SYSTEM_UPDATE_GITHUB_REPO", "example/fork")
	client := &fakeSystemUpdateGitHubClient{release: testSystemUpdateRelease("v1.1.0")}
	service := NewSystemUpdateService(client)
	service.currentVersionFn = func() string { return "v1.0.0" }
	service.goosFn = func() string { return "linux" }
	service.goarchFn = func() string { return "amd64" }
	service.isContainerFn = func() bool { return false }
	service.executableFn = func() (string, error) { return "", errors.New("not configured") }

	_, err := service.CheckLatest(context.Background(), true)

	require.NoError(t, err)
	assert.Equal(t, "example/fork", client.requestedRepo)
}

func TestSystemUpdateGitHubRepoFallsBackWhenEnvBlank(t *testing.T) {
	t.Setenv("SYSTEM_UPDATE_GITHUB_REPO", " ")
	client := &fakeSystemUpdateGitHubClient{release: testSystemUpdateRelease("v1.1.0")}
	service := NewSystemUpdateService(client)
	service.currentVersionFn = func() string { return "v1.0.0" }
	service.goosFn = func() string { return "linux" }
	service.goarchFn = func() string { return "amd64" }
	service.isContainerFn = func() bool { return false }
	service.executableFn = func() (string, error) { return "", errors.New("not configured") }

	_, err := service.CheckLatest(context.Background(), true)

	require.NoError(t, err)
	assert.Equal(t, systemUpdateDefaultGitHubRepo, client.requestedRepo)
}

func TestSystemUpdateCheckDisablesContainerAndSourceBuilds(t *testing.T) {
	release := testSystemUpdateRelease("v1.1.0")

	containerService := newTestSystemUpdateService("v1.0.0", "linux", "amd64", true, release)
	containerInfo, err := containerService.CheckLatest(context.Background(), true)
	require.NoError(t, err)
	assert.False(t, containerInfo.CanApply)
	assert.Equal(t, systemUpdateBuildContainer, containerInfo.BuildType)
	assert.Equal(t, systemUpdateMethodDockerEngine, containerInfo.UpdateMethod)
	assert.Contains(t, containerInfo.ApplyDisabledReason, "/var/run/docker.sock")
	assert.Contains(t, containerInfo.ManualUpdateHint, "c1cadabob/nexustok:latest")

	sourceService := newTestSystemUpdateService("v0.0.0", "linux", "amd64", false, release)
	sourceInfo, err := sourceService.CheckLatest(context.Background(), true)
	require.NoError(t, err)
	assert.False(t, sourceInfo.CanApply)
	assert.Equal(t, systemUpdateBuildSource, sourceInfo.BuildType)
	assert.Contains(t, sourceInfo.ApplyDisabledReason, "Source or development")
	assert.Contains(t, sourceInfo.ManualUpdateHint, "pulling the latest code")
}

func TestSystemUpdateCheckKeepsUnparseableBuildComparisonUnknown(t *testing.T) {
	service := newTestSystemUpdateService("main-20260802-f7ba633", "linux", "amd64", true, testSystemUpdateRelease("v0.1.1"))

	info, err := service.CheckLatest(context.Background(), true)

	require.NoError(t, err)
	assert.Equal(t, systemUpdateComparisonUnknown, info.ComparisonStatus)
	assert.True(t, info.HasUpdate)
	assert.False(t, info.CanApply)
	assert.Contains(t, info.Warning, "Unable to compare")
	assert.Contains(t, info.ApplyDisabledReason, "/var/run/docker.sock")
}

func TestSystemUpdateCheckKeepsUnknownCustomPlatformNotApplicable(t *testing.T) {
	service := newTestSystemUpdateService("v1.0.0", "linux", "riscv64", false, testSystemUpdateRelease("v1.1.0"))

	info, err := service.CheckLatest(context.Background(), true)

	require.NoError(t, err)
	assert.True(t, info.HasUpdate)
	assert.False(t, info.CanApply)
	assert.Nil(t, info.MatchedAsset)
	assert.Contains(t, info.ApplyDisabledReason, "No compatible release asset")
}

func TestExpectedSystemUpdateAssetNames(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		goarch   string
		version  string
		expected []string
	}{
		{name: "linux amd64", goos: "linux", goarch: "amd64", version: "v1.2.3", expected: []string{"nexustok-v1.2.3", "nexustok-1.2.3"}},
		{name: "linux arm64", goos: "linux", goarch: "arm64", version: "v1.2.3", expected: []string{"nexustok-arm64-v1.2.3", "nexustok-arm64-1.2.3"}},
		{name: "macos", goos: "darwin", goarch: "arm64", version: "v1.2.3", expected: []string{"nexustok-macos-v1.2.3", "nexustok-macos-1.2.3"}},
		{name: "windows amd64", goos: "windows", goarch: "amd64", version: "v1.2.3", expected: []string{"nexustok-v1.2.3.exe", "nexustok-1.2.3.exe"}},
		{name: "unsupported", goos: "linux", goarch: "386", version: "v1.2.3", expected: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, expectedSystemUpdateAssetNames(tt.goos, tt.goarch, tt.version))
		})
	}
}

func TestValidateSystemUpdateDownloadURL(t *testing.T) {
	validURLs := []string{
		"https://github.com/c1cadaBob/NexusTok/releases/download/v1/nexustok-v1",
		"https://objects.githubusercontent.com/github-production-release-asset/file",
		"https://sub.objects.githubusercontent.com/file",
	}
	for _, rawURL := range validURLs {
		require.NoError(t, validateSystemUpdateDownloadURL(rawURL), rawURL)
	}

	invalidURLs := []string{
		"http://github.com/c1cadaBob/NexusTok/releases/download/v1/nexustok-v1",
		"https://example.com/file",
		"https://github.com.evil.test/file",
		"https://objects.githubusercontent.com.evil.test/file",
	}
	for _, rawURL := range invalidURLs {
		require.Error(t, validateSystemUpdateDownloadURL(rawURL), rawURL)
	}
}

func TestSystemUpdateHTTPClientDownloadRejectsOversizedContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "8")
		_, _ = w.Write([]byte("12345678"))
	}))
	defer server.Close()

	client := &systemUpdateHTTPClient{downloadClient: server.Client()}
	dest := filepath.Join(t.TempDir(), "download.bin")

	err := client.DownloadFile(context.Background(), server.URL, dest, 4, nil)

	require.Error(t, err)
	_, statErr := os.Stat(dest)
	assert.True(t, os.IsNotExist(statErr))
}

func TestSystemUpdateHTTPClientDownloadRejectsOversizedBodyWithoutContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Del("Content-Length")
		_, _ = w.Write([]byte("12345678"))
	}))
	defer server.Close()

	client := &systemUpdateHTTPClient{downloadClient: server.Client()}
	dest := filepath.Join(t.TempDir(), "download.bin")

	err := client.DownloadFile(context.Background(), server.URL, dest, 4, nil)

	require.Error(t, err)
	_, statErr := os.Stat(dest)
	assert.True(t, os.IsNotExist(statErr))
}

func TestVerifySystemUpdateChecksum(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "nexustok-v1.1.0")
	content := []byte("new binary")
	require.NoError(t, os.WriteFile(filePath, content, 0644))
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:]) + "  nexustok-v1.1.0\n"

	actual, err := verifySystemUpdateChecksum(filePath, "nexustok-v1.1.0", []byte(checksum))

	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(sum[:]), actual)
}

func TestVerifySystemUpdateChecksumDetectsMissingAndMismatch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "nexustok-v1.1.0")
	require.NoError(t, os.WriteFile(filePath, []byte("new binary"), 0644))

	_, missingErr := verifySystemUpdateChecksum(filePath, "nexustok-v1.1.0", []byte("abc  other-file\n"))
	require.Error(t, missingErr)

	_, mismatchErr := verifySystemUpdateChecksum(filePath, "nexustok-v1.1.0", []byte("abc  nexustok-v1.1.0\n"))
	require.Error(t, mismatchErr)
}

func TestReplaceSystemUpdateExecutableKeepsBackup(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "nexustok")
	newPath := filepath.Join(dir, "new-nexustok")
	require.NoError(t, os.WriteFile(exePath, []byte("old"), 0755))
	require.NoError(t, os.WriteFile(newPath, []byte("new"), 0755))

	backupPath, err := replaceSystemUpdateExecutable(exePath, newPath)

	require.NoError(t, err)
	assert.Equal(t, exePath+".backup", backupPath)
	assertFileContent(t, exePath, "new")
	assertFileContent(t, backupPath, "old")
}

func TestReplaceSystemUpdateExecutableRestoresBackupWhenNewRenameFails(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "nexustok")
	missingNewPath := filepath.Join(dir, "missing-nexustok")
	require.NoError(t, os.WriteFile(exePath, []byte("old"), 0755))

	err := func() error {
		_, replaceErr := replaceSystemUpdateExecutable(exePath, missingNewPath)
		return replaceErr
	}()

	require.Error(t, err)
	assertFileContent(t, exePath, "old")
	_, statErr := os.Stat(exePath + ".backup")
	assert.True(t, os.IsNotExist(statErr))
}

func TestReplaceSystemUpdateExecutableStopsWhenOldBackupCannotBeRemoved(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "nexustok")
	newPath := filepath.Join(dir, "new-nexustok")
	require.NoError(t, os.WriteFile(exePath, []byte("old"), 0755))
	require.NoError(t, os.WriteFile(newPath, []byte("new"), 0755))
	require.NoError(t, os.Mkdir(exePath+".backup", 0755))
	require.NoError(t, os.WriteFile(filepath.Join(exePath+".backup", "keep"), []byte("backup"), 0644))

	_, err := replaceSystemUpdateExecutable(exePath, newPath)

	require.Error(t, err)
	assertFileContent(t, exePath, "old")
	assertFileContent(t, newPath, "new")
}

func TestPerformRollbackRestoresBackup(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "nexustok")
	backupPath := exePath + ".backup"
	require.NoError(t, os.WriteFile(exePath, []byte("new"), 0755))
	require.NoError(t, os.WriteFile(backupPath, []byte("old"), 0755))
	service := newTestSystemUpdateService("v1.1.0", "linux", "amd64", false, testSystemUpdateRelease("v1.1.0"))
	service.executableFn = func() (string, error) { return exePath, nil }
	service.evalSymlinksFn = func(path string) (string, error) { return path, nil }

	result, err := service.PerformRollback(context.Background(), nil, "")

	require.NoError(t, err)
	assert.True(t, result.RestartRequired)
	assertFileContent(t, exePath, "old")
	_, statErr := os.Stat(backupPath)
	assert.True(t, os.IsNotExist(statErr))
}

func TestPerformRollbackRequiresBackup(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "nexustok")
	require.NoError(t, os.WriteFile(exePath, []byte("new"), 0755))
	service := newTestSystemUpdateService("v1.1.0", "linux", "amd64", false, testSystemUpdateRelease("v1.1.0"))
	service.executableFn = func() (string, error) { return exePath, nil }
	service.evalSymlinksFn = func(path string) (string, error) { return path, nil }

	_, err := service.PerformRollback(context.Background(), nil, "")

	require.ErrorIs(t, err, ErrSystemRollbackDisabled)
}

func TestStartSystemUpdateTaskReturnsActiveTaskBeforeCheckingRelease(t *testing.T) {
	setupSystemTaskServiceTestDB(t)
	activeTask, err := model.CreateSystemTaskWithActiveKey(model.SystemTaskTypeSystemUpdate, systemUpdateActiveKey, nil, nil)
	require.NoError(t, err)

	task, err := StartSystemUpdateTask(context.Background())

	require.NoError(t, err)
	require.Equal(t, activeTask.TaskID, task.TaskID)
}

func TestStartSystemRollbackTaskReturnsActiveTaskBeforeCheckingBackup(t *testing.T) {
	setupSystemTaskServiceTestDB(t)
	activeTask, err := model.CreateSystemTaskWithActiveKey(model.SystemTaskTypeSystemUpdate, systemUpdateActiveKey, nil, nil)
	require.NoError(t, err)

	task, err := StartSystemRollbackTask()

	require.NoError(t, err)
	require.Equal(t, activeTask.TaskID, task.TaskID)
}

func TestRestartSystemServiceSchedulesLinuxExit(t *testing.T) {
	originalGOOS := systemRestartGOOS
	originalExit := systemRestartExit
	originalDelay := systemRestartDelay
	t.Cleanup(func() {
		systemRestartGOOS = originalGOOS
		systemRestartExit = originalExit
		systemRestartDelay = originalDelay
	})

	exitCodes := make(chan int, 1)
	systemRestartGOOS = func() string { return "linux" }
	systemRestartDelay = 0
	systemRestartExit = func(code int) {
		exitCodes <- code
	}

	result := restartSystemService()

	require.True(t, result.RestartSupported)
	require.True(t, result.RestartScheduled)
	select {
	case code := <-exitCodes:
		assert.Equal(t, 0, code)
	case <-time.After(time.Second):
		t.Fatal("restart exit was not called")
	}
}

func TestRestartSystemServiceRequiresManualRestartOutsideLinux(t *testing.T) {
	originalGOOS := systemRestartGOOS
	originalExit := systemRestartExit
	t.Cleanup(func() {
		systemRestartGOOS = originalGOOS
		systemRestartExit = originalExit
	})

	systemRestartGOOS = func() string { return "darwin" }
	systemRestartExit = func(int) {
		t.Fatal("non-linux restart must not exit the process")
	}

	result := restartSystemService()

	assert.False(t, result.RestartSupported)
	assert.False(t, result.RestartScheduled)
	assert.True(t, result.ManualRequired)
}

func newTestSystemUpdateService(currentVersion string, goos string, goarch string, isContainer bool, release *systemUpdateGitHubRelease) *SystemUpdateService {
	service := NewSystemUpdateService(&fakeSystemUpdateGitHubClient{release: release})
	service.currentVersionFn = func() string { return currentVersion }
	service.goosFn = func() string { return goos }
	service.goarchFn = func() string { return goarch }
	service.isContainerFn = func() bool { return isContainer }
	service.executableFn = func() (string, error) { return "", errors.New("not configured") }
	service.evalSymlinksFn = func(path string) (string, error) { return path, nil }
	service.dockerSocketPathFn = func() string { return filepath.Join(os.TempDir(), "nexustok-test-missing-docker.sock") }
	return service
}

func testSystemUpdateRelease(version string) *systemUpdateGitHubRelease {
	return &systemUpdateGitHubRelease{
		TagName:     version,
		Name:        version,
		HTMLURL:     "https://github.com/c1cadaBob/NexusTok/releases/tag/" + version,
		PublishedAt: "2026-08-01T00:00:00Z",
		Assets: []systemUpdateGitHubAsset{
			{Name: "nexustok-" + version, BrowserDownloadURL: "https://github.com/c1cadaBob/NexusTok/releases/download/" + version + "/nexustok-" + version, Size: 10},
			{Name: "nexustok-arm64-" + version, BrowserDownloadURL: "https://github.com/c1cadaBob/NexusTok/releases/download/" + version + "/nexustok-arm64-" + version, Size: 10},
			{Name: "nexustok-macos-" + version, BrowserDownloadURL: "https://github.com/c1cadaBob/NexusTok/releases/download/" + version + "/nexustok-macos-" + version, Size: 10},
			{Name: "nexustok-" + version + ".exe", BrowserDownloadURL: "https://github.com/c1cadaBob/NexusTok/releases/download/" + version + "/nexustok-" + version + ".exe", Size: 10},
			{Name: "checksums-linux.txt", BrowserDownloadURL: "https://github.com/c1cadaBob/NexusTok/releases/download/" + version + "/checksums-linux.txt", Size: 100},
			{Name: "checksums-macos.txt", BrowserDownloadURL: "https://github.com/c1cadaBob/NexusTok/releases/download/" + version + "/checksums-macos.txt", Size: 100},
			{Name: "checksums-windows.txt", BrowserDownloadURL: "https://github.com/c1cadaBob/NexusTok/releases/download/" + version + "/checksums-windows.txt", Size: 100},
		},
	}
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(data))
}
