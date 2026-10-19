package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/model"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

func setupSystemTaskServiceTestDB(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open("file:system_update_test_"+common.GetUUID()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})
}

type fakeSystemUpdateGitHubClient struct {
	release       *systemUpdateGitHubRelease
	releases      []*systemUpdateGitHubRelease
	fetchErr      error
	downloadData  []byte
	checksumData  []byte
	requestedRepo string
	fetchCount    int
}

type systemUpdateTestRoundTripper func(*http.Request) (*http.Response, error)

func (f systemUpdateTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func (c *fakeSystemUpdateGitHubClient) FetchLatestRelease(_ context.Context, repo string) (*systemUpdateGitHubRelease, error) {
	c.requestedRepo = repo
	c.fetchCount++
	if c.fetchErr != nil {
		return nil, c.fetchErr
	}
	return c.release, nil
}

func (c *fakeSystemUpdateGitHubClient) ListReleases(_ context.Context, repo string) ([]*systemUpdateGitHubRelease, error) {
	c.requestedRepo = repo
	c.fetchCount++
	if c.fetchErr != nil {
		return nil, c.fetchErr
	}
	if c.release == nil && len(c.releases) == 0 {
		return nil, ErrSystemUpdateNoRelease
	}
	if len(c.releases) > 0 {
		return c.releases, nil
	}
	return []*systemUpdateGitHubRelease{c.release}, nil
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

func TestSystemUpdateCheckMatchesPublishedReleaseAssetNames(t *testing.T) {
	release := &systemUpdateGitHubRelease{
		TagName:     "v0.2.1",
		PublishedAt: "2026-10-01T00:00:00Z",
		Assets: []systemUpdateGitHubAsset{
			{
				Name:               "nexustok-linux-amd64-v0.2.1",
				BrowserDownloadURL: "https://github.com/c1cadaBob/NexusTok/releases/download/v0.2.1/nexustok-linux-amd64-v0.2.1",
			},
			{
				Name:               "checksums-linux-v0.2.1.txt",
				BrowserDownloadURL: "https://github.com/c1cadaBob/NexusTok/releases/download/v0.2.1/checksums-linux-v0.2.1.txt",
			},
		},
	}
	service := newTestSystemUpdateService("v0.2.0", "linux", "amd64", false, release)

	info, err := service.CheckLatest(context.Background(), true)

	require.NoError(t, err)
	require.True(t, info.CanApply)
	assert.Equal(t, "nexustok-linux-amd64-v0.2.1", info.MatchedAsset.Name)
	assert.Equal(t, "checksums-linux-v0.2.1.txt", info.ChecksumAsset.Name)
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
	assert.False(t, info.HasUpdate)
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
	assert.Contains(t, info.Docker.OneTimeEnableCommand, "-p 3030:3030")
	assert.Contains(t, info.Docker.OneTimeEnableCommand, "-e PORT=3030")
}

func TestSystemUpdateDockerManualRunCommandDefaultsTo3030(t *testing.T) {
	command := minimalSystemUpdateDockerRunCommand(
		"c1cadabob/nexustok:latest",
		"nexustok",
		[]string{"/opt/nexustok/data:/data", "/opt/nexustok/logs:/app/logs"},
		nil,
		[]string{"TZ=Asia/Shanghai"},
	)

	assert.Contains(t, command, "-p 3030:3030")
	assert.Contains(t, command, "-e PORT=3030")
	assert.Contains(t, command, "-v /opt/nexustok/data:/data")
	assert.Contains(t, command, "c1cadabob/nexustok:latest")
}

func TestSystemUpdateTargetDockerImagePreservesDigestRepository(t *testing.T) {
	assert.Equal(
		t,
		"registry.example/nexustok:latest",
		systemUpdateTargetDockerImage("registry.example/nexustok@sha256:deadbeef"),
	)
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

func TestSystemUpdateCheckUsesCacheAndForceRefresh(t *testing.T) {
	client := &fakeSystemUpdateGitHubClient{release: testSystemUpdateRelease("v1.1.0")}
	service := NewSystemUpdateService(client)
	service.currentVersionFn = func() string { return "v1.0.0" }
	service.goosFn = func() string { return "linux" }
	service.goarchFn = func() string { return "amd64" }
	service.isContainerFn = func() bool { return false }
	service.executableFn = func() (string, error) { return "", errors.New("not configured") }

	now := time.Unix(100, 0)
	service.nowFn = func() time.Time { return now }

	first, err := service.CheckLatest(context.Background(), true)
	require.NoError(t, err)
	require.False(t, first.Cached)
	assert.Equal(t, 1, client.fetchCount)

	cached, err := service.CheckLatest(context.Background(), false)
	require.NoError(t, err)
	assert.True(t, cached.Cached)
	assert.Equal(t, 1, client.fetchCount)

	refreshed, err := service.CheckLatest(context.Background(), true)
	require.NoError(t, err)
	assert.False(t, refreshed.Cached)
	assert.Equal(t, 2, client.fetchCount)
}

func TestSystemUpdateCheckListsAvailableReleasesAndMarksPrerelease(t *testing.T) {
	current := testSystemUpdateRelease("v1.0.0")
	current.PublishedAt = "2026-10-01T00:00:00Z"
	latest := testSystemUpdateRelease("v1.2.0")
	latest.PublishedAt = "2026-10-03T00:00:00Z"
	prerelease := testSystemUpdateRelease("v1.1.0-rc.1")
	prerelease.Prerelease = true
	prerelease.PublishedAt = "2026-10-02T00:00:00Z"
	draft := testSystemUpdateRelease("v9.0.0")
	draft.Draft = true
	invalid := &systemUpdateGitHubRelease{TagName: "nightly", PublishedAt: "2026-10-04T00:00:00Z"}
	service := NewSystemUpdateService(&fakeSystemUpdateGitHubClient{
		releases: []*systemUpdateGitHubRelease{current, latest, prerelease, draft, invalid},
	})
	service.currentVersionFn = func() string { return "v1.0.0" }
	service.goosFn = func() string { return "linux" }
	service.goarchFn = func() string { return "amd64" }
	service.isContainerFn = func() bool { return false }

	info, err := service.CheckLatest(context.Background(), true)

	require.NoError(t, err)
	require.Len(t, info.AvailableReleases, 3)
	assert.Equal(t, "v1.2.0", info.AvailableReleases[0].TagName)
	assert.Equal(t, "v1.1.0-rc.1", info.AvailableReleases[1].TagName)
	assert.True(t, info.AvailableReleases[1].Prerelease)
	assert.True(t, info.AvailableReleases[2].Current)
	assert.True(t, info.RollbackAvailable)
	assert.Equal(t, "v1.2.0", info.LatestVersion)
}

func TestSystemUpdateCheckUsesTargetRepositoryTagForDockerRelease(t *testing.T) {
	current := testSystemUpdateRelease("v1.0.0")
	latest := testSystemUpdateRelease("v1.2.0")
	service := NewSystemUpdateService(&fakeSystemUpdateGitHubClient{
		releases: []*systemUpdateGitHubRelease{latest, current},
	})
	service.currentVersionFn = func() string { return "v1.0.0" }
	service.goosFn = func() string { return "linux" }
	service.goarchFn = func() string { return "amd64" }
	service.isContainerFn = func() bool { return true }
	service.dockerSocketPathFn = func() string { return filepath.Join(t.TempDir(), "missing.sock") }

	info, err := service.CheckLatest(context.Background(), true)

	require.NoError(t, err)
	require.Len(t, info.AvailableReleases, 2)
	assert.Equal(t, "c1cadabob/nexustok:v1.2.0", info.AvailableReleases[0].DockerTargetImage)
	assert.NotContains(t, info.AvailableReleases[0].DockerTargetImage, "latest")
}

func TestResolveTargetReleaseRejectsInvalidCurrentAndUnknownVersions(t *testing.T) {
	current := testSystemUpdateRelease("v1.0.0")
	target := testSystemUpdateRelease("v0.9.0")
	service := newTestSystemUpdateService("v1.0.0", "linux", "amd64", false, current)
	service.githubClient = &fakeSystemUpdateGitHubClient{releases: []*systemUpdateGitHubRelease{current, target}}
	info, err := service.CheckLatest(context.Background(), true)
	require.NoError(t, err)

	_, err = service.resolveTargetRelease(context.Background(), info, "not-semver")
	require.Error(t, err)
	_, err = service.resolveTargetRelease(context.Background(), info, "v1.0.0")
	require.Error(t, err)
	_, err = service.resolveTargetRelease(context.Background(), info, "v9.9.9")
	require.Error(t, err)
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
	assert.False(t, info.HasUpdate)
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
		{
			name:    "linux amd64",
			goos:    "linux",
			goarch:  "amd64",
			version: "v1.2.3",
			expected: []string{
				"nexustok-linux-amd64-v1.2.3",
				"nexustok-amd64-v1.2.3",
				"nexustok-v1.2.3",
				"nexustok-linux-amd64-1.2.3",
				"nexustok-amd64-1.2.3",
				"nexustok-1.2.3",
			},
		},
		{
			name:    "linux arm64",
			goos:    "linux",
			goarch:  "arm64",
			version: "v1.2.3",
			expected: []string{
				"nexustok-linux-arm64-v1.2.3",
				"nexustok-arm64-v1.2.3",
				"nexustok-v1.2.3",
				"nexustok-linux-arm64-1.2.3",
				"nexustok-arm64-1.2.3",
				"nexustok-1.2.3",
			},
		},
		{
			name:    "macos",
			goos:    "darwin",
			goarch:  "arm64",
			version: "v1.2.3",
			expected: []string{
				"nexustok-darwin-arm64-v1.2.3",
				"nexustok-macos-arm64-v1.2.3",
				"nexustok-macos-v1.2.3",
				"nexustok-darwin-arm64-1.2.3",
				"nexustok-macos-arm64-1.2.3",
				"nexustok-macos-1.2.3",
			},
		},
		{
			name:    "windows amd64",
			goos:    "windows",
			goarch:  "amd64",
			version: "v1.2.3",
			expected: []string{
				"nexustok-windows-amd64-v1.2.3.exe",
				"nexustok-amd64-v1.2.3.exe",
				"nexustok-v1.2.3.exe",
				"nexustok-windows-amd64-1.2.3.exe",
				"nexustok-amd64-1.2.3.exe",
				"nexustok-1.2.3.exe",
			},
		},
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
		"https://release-assets.githubusercontent.com/github-production-release-asset/file",
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

func TestSystemUpdateHTTPClientMapsGitHub404ToNoRelease(t *testing.T) {
	client := &systemUpdateHTTPClient{
		metadataClient: &http.Client{
			Transport: systemUpdateTestRoundTripper(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
		metadataURLValidator: validateSystemUpdateGitHubAPIURL,
	}

	_, err := client.FetchLatestRelease(context.Background(), systemUpdateDefaultGitHubRepo)

	require.ErrorIs(t, err, ErrSystemUpdateNoRelease)
}

func TestSystemUpdateHTTPClientListReleasesUsesPaginationAndFilters(t *testing.T) {
	var requests []string
	client := &systemUpdateHTTPClient{
		metadataClient: &http.Client{
			Transport: systemUpdateTestRoundTripper(func(request *http.Request) (*http.Response, error) {
				requests = append(requests, request.URL.RawQuery)
				var body string
				switch request.URL.Query().Get("page") {
				case "1":
					pageReleases := make([]map[string]any, 0, 100)
					pageReleases = append(pageReleases,
						map[string]any{
							"tag_name":     "v1.2.0",
							"draft":        false,
							"prerelease":   false,
							"published_at": "2026-10-03T00:00:00Z",
						},
						map[string]any{
							"tag_name":     "nightly",
							"draft":        false,
							"prerelease":   false,
							"published_at": "2026-10-03T00:00:00Z",
						},
						map[string]any{
							"tag_name":   "v9.0.0",
							"draft":      true,
							"prerelease": false,
						},
					)
					for index := len(pageReleases); index < 100; index++ {
						pageReleases = append(pageReleases, map[string]any{
							"tag_name":     fmt.Sprintf("build-%d", index),
							"draft":        false,
							"prerelease":   false,
							"published_at": "2026-10-01T00:00:00Z",
						})
					}
					data, marshalErr := common.Marshal(pageReleases)
					require.NoError(t, marshalErr)
					body = string(data)
				case "2":
					body = `[{"tag_name":"v1.1.0-rc.1","draft":false,"prerelease":true,"published_at":"2026-10-02T00:00:00Z"}]`
				default:
					body = `[]`
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		},
		metadataURLValidator: validateSystemUpdateGitHubAPIURL,
	}

	releases, err := client.ListReleases(context.Background(), systemUpdateDefaultGitHubRepo)

	require.NoError(t, err)
	require.Len(t, releases, 2)
	assert.Equal(t, []string{"page=1&per_page=100", "page=2&per_page=100"}, requests)
	assert.Equal(t, "v1.2.0", releases[0].TagName)
	assert.True(t, releases[1].Prerelease)
}

func TestSystemUpdateRedirectCheckerRejectsUntrustedAssetHost(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://evil.example/file", nil)
	require.NoError(t, err)
	checker := systemUpdateRedirectChecker(validateSystemUpdateDownloadURL)

	require.Error(t, checker(request, nil))

	request.URL, err = url.Parse("https://release-assets.githubusercontent.com/file")
	require.NoError(t, err)
	require.NoError(t, checker(request, nil))
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

func TestCreateSystemUpdateBackupCapturesDataLogsAndSQLiteSidecars(t *testing.T) {
	dataDir := t.TempDir()
	logDir := t.TempDir()
	backupDir := t.TempDir()
	sqlitePath := filepath.Join(t.TempDir(), "nexustok.db")
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "settings.json"), []byte(`{"version":1}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "app.log"), []byte("log"), 0600))
	require.NoError(t, os.WriteFile(sqlitePath, []byte("sqlite"), 0600))
	require.NoError(t, os.WriteFile(sqlitePath+"-wal", []byte("wal"), 0600))
	require.NoError(t, os.WriteFile(sqlitePath+"-shm", []byte("shm"), 0600))

	previousSQLitePath := common.SQLitePath
	previousLogDir := common.LogDir
	previousMainType := common.MainDatabaseType()
	t.Cleanup(func() {
		common.SQLitePath = previousSQLitePath
		common.LogDir = previousLogDir
		common.SetMainDatabaseType(previousMainType)
	})
	common.SQLitePath = sqlitePath
	common.LogDir = &logDir
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Setenv(systemUpdateDataDirEnv, dataDir)
	t.Setenv(systemUpdateBackupDirEnv, backupDir)

	backup, err := CreateSystemUpdateBackup(context.Background(), "v0.2.7")

	require.NoError(t, err)
	require.NotNil(t, backup)
	assert.Equal(t, systemUpdateBackupStatusReady, backup.Status)
	assert.Equal(t, 0700, int(fileModePerm(t, filepath.Join(backupDir, backup.ID))))
	assert.Equal(t, 0600, int(fileModePerm(t, filepath.Join(backupDir, backup.ID, "manifest.json"))))

	manifestData, err := os.ReadFile(filepath.Join(backupDir, backup.ID, "manifest.json"))
	require.NoError(t, err)
	var manifest systemUpdateBackupManifest
	require.NoError(t, common.Unmarshal(manifestData, &manifest))
	assert.Equal(t, "v0.2.7", manifest.Version)
	assert.ElementsMatch(t,
		[]string{"data", "logs", "sqlite", "sqlite-wal", "sqlite-shm"},
		backupComponentNames(manifest.Components),
	)
}

func TestSystemUpdateBackupExcludesBackupRootFromDataArchive(t *testing.T) {
	dataDir := t.TempDir()
	logDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "system-backups")
	sqlitePath := filepath.Join(t.TempDir(), "nexustok.db")
	require.NoError(t, os.MkdirAll(backupDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "settings.json"), []byte("settings"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(backupDir, "old.bin"), []byte("old"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "app.log"), []byte("log"), 0600))
	require.NoError(t, os.WriteFile(sqlitePath, []byte("db"), 0600))

	previousSQLitePath := common.SQLitePath
	previousLogDir := common.LogDir
	previousMainType := common.MainDatabaseType()
	t.Cleanup(func() {
		common.SQLitePath = previousSQLitePath
		common.LogDir = previousLogDir
		common.SetMainDatabaseType(previousMainType)
	})
	common.SQLitePath = sqlitePath
	common.LogDir = &logDir
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Setenv(systemUpdateDataDirEnv, dataDir)
	t.Setenv(systemUpdateBackupDirEnv, backupDir)

	backup, err := CreateSystemUpdateBackup(context.Background(), "v1.0.0")
	require.NoError(t, err)
	archive, err := os.Open(filepath.Join(backupDir, backup.ID, "data.tar.gz"))
	require.NoError(t, err)
	defer archive.Close()
	gzipReader, err := gzip.NewReader(archive)
	require.NoError(t, err)
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var names []string
	for {
		header, readErr := tarReader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
		names = append(names, header.Name)
	}
	assert.Contains(t, names, "settings.json")
	assert.NotContains(t, names, "system-backups/old.bin")
}

func TestRestoreSystemUpdateBackupRestoresFilesWithoutRedis(t *testing.T) {
	dataDir := t.TempDir()
	logDir := t.TempDir()
	backupDir := t.TempDir()
	sqlitePath := filepath.Join(t.TempDir(), "nexustok.db")
	dataPath := filepath.Join(dataDir, "settings.json")
	logPath := filepath.Join(logDir, "app.log")
	require.NoError(t, os.WriteFile(dataPath, []byte("before"), 0600))
	require.NoError(t, os.WriteFile(logPath, []byte("before-log"), 0600))
	require.NoError(t, os.WriteFile(sqlitePath, []byte("before-db"), 0600))

	previousSQLitePath := common.SQLitePath
	previousLogDir := common.LogDir
	previousMainType := common.MainDatabaseType()
	t.Cleanup(func() {
		common.SQLitePath = previousSQLitePath
		common.LogDir = previousLogDir
		common.SetMainDatabaseType(previousMainType)
	})
	common.SQLitePath = sqlitePath
	common.LogDir = &logDir
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Setenv(systemUpdateDataDirEnv, dataDir)
	t.Setenv(systemUpdateBackupDirEnv, backupDir)

	backup, err := CreateSystemUpdateBackup(context.Background(), "v0.2.7")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dataPath, []byte("after"), 0600))
	require.NoError(t, os.WriteFile(logPath, []byte("after-log"), 0600))
	require.NoError(t, os.WriteFile(sqlitePath, []byte("after-db"), 0600))

	require.NoError(t, RestoreSystemUpdateBackup(context.Background(), backup.ID))
	assertFileContent(t, dataPath, "before")
	assertFileContent(t, logPath, "before-log")
	assertFileContent(t, sqlitePath, "before-db")
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

func TestReplaceSystemUpdateExecutableAtomicallyInstallsTarget(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "nexustok")
	newPath := filepath.Join(dir, "new-nexustok")
	require.NoError(t, os.WriteFile(exePath, []byte("old"), 0755))
	require.NoError(t, os.WriteFile(newPath, []byte("new"), 0755))

	err := replaceSystemUpdateExecutableAtomically(exePath, newPath)

	require.NoError(t, err)
	assertFileContent(t, exePath, "new")
	_, statErr := os.Stat(exePath + ".backup")
	assert.True(t, os.IsNotExist(statErr))
}

func TestReplaceSystemUpdateExecutableRestoresOriginalWhenTargetMissing(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "nexustok")
	missingNewPath := filepath.Join(dir, "missing-nexustok")
	require.NoError(t, os.WriteFile(exePath, []byte("old"), 0755))

	err := replaceSystemUpdateExecutableAtomically(exePath, missingNewPath)

	require.Error(t, err)
	assertFileContent(t, exePath, "old")
	_, statErr := os.Stat(exePath + ".backup")
	assert.True(t, os.IsNotExist(statErr))
}

func TestReplaceSystemUpdateExecutablePreservesCurrentPermissions(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "nexustok")
	newPath := filepath.Join(dir, "new-nexustok")
	require.NoError(t, os.WriteFile(exePath, []byte("old"), 0710))
	require.NoError(t, os.WriteFile(newPath, []byte("new"), 0755))

	err := replaceSystemUpdateExecutableAtomically(exePath, newPath)

	require.NoError(t, err)
	info, err := os.Stat(exePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0710), info.Mode().Perm())
}

func TestPerformRollbackDownloadsSelectedRelease(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "nexustok")
	currentRelease := testSystemUpdateRelease("v1.1.0")
	targetRelease := testSystemUpdateRelease("v1.0.0")
	client := &fakeSystemUpdateGitHubClient{
		release:      currentRelease,
		releases:     []*systemUpdateGitHubRelease{currentRelease, targetRelease},
		downloadData: []byte("target"),
	}
	sum := sha256.Sum256(client.downloadData)
	client.checksumData = []byte(hex.EncodeToString(sum[:]) + "  nexustok-v1.0.0\n")
	service := NewSystemUpdateService(client)
	service.currentVersionFn = func() string { return "v1.1.0" }
	service.goosFn = func() string { return "linux" }
	service.goarchFn = func() string { return "amd64" }
	service.isContainerFn = func() bool { return false }
	service.executableFn = func() (string, error) { return exePath, nil }
	service.evalSymlinksFn = func(path string) (string, error) { return path, nil }
	service.backupFn = func(context.Context, string) (*SystemUpdateBackup, error) {
		return &SystemUpdateBackup{ID: "backup-test", Status: systemUpdateBackupStatusReady}, nil
	}
	require.NoError(t, os.WriteFile(exePath, []byte("current"), 0755))

	setupSystemTaskServiceTestDB(t)
	task, err := model.CreateSystemTask(
		model.SystemTaskTypeSystemRollback,
		SystemUpdateTaskPayload{TargetVersion: "v1.0.0"},
		nil,
	)
	require.NoError(t, err)
	task, claimed, err := model.ClaimSystemTask(
		task.ID,
		model.SystemTaskTypeSystemRollback,
		"runner",
		common.GetTimestamp()+60,
	)
	require.NoError(t, err)
	require.True(t, claimed)
	result, err := service.PerformRollback(context.Background(), task, "runner")

	require.NoError(t, err)
	assert.True(t, result.RestartRequired)
	assert.Equal(t, "v1.0.0", result.TargetVersion)
	assertFileContent(t, exePath, "target")
	_, statErr := os.Stat(exePath + ".backup")
	assert.True(t, os.IsNotExist(statErr))
	resultJSON, err := common.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(resultJSON), "current_executable")
}

func TestSystemUpdateDatabaseToolsUseDumpOutputAndRestoreInput(t *testing.T) {
	dir := t.TempDir()
	dumpScript := filepath.Join(dir, "pg_dump")
	restoreScript := filepath.Join(dir, "psql")
	require.NoError(t, os.WriteFile(dumpScript, []byte("#!/bin/sh\nprintf 'dumped sql'\n"), 0755))
	require.NoError(t, os.WriteFile(restoreScript, []byte("#!/bin/sh\ncat\n"), 0755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	dumpPath := filepath.Join(t.TempDir(), "postgres.sql")
	require.NoError(t, runSystemUpdateDatabaseTool(
		context.Background(),
		"pg_dump",
		nil,
		dumpPath,
		[]string{"PGPASSWORD=test-only-secret"},
	))
	assertFileContent(t, dumpPath, "dumped sql")

	require.NoError(t, runSystemUpdateDatabaseTool(
		context.Background(),
		"psql",
		nil,
		dumpPath,
		[]string{"PGPASSWORD=test-only-secret"},
	))
	assert.True(t, systemUpdateDatabaseToolWritesOutput("pg_dump"))
	assert.False(t, systemUpdateDatabaseToolWritesOutput("psql"))
}

func TestPerformRollbackDisablesRunningWindowsExecutable(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "nexustok.exe")
	require.NoError(t, os.WriteFile(exePath, []byte("new"), 0755))
	service := newTestSystemUpdateService("v1.1.0", "windows", "amd64", false, testSystemUpdateRelease("v1.1.0"))
	service.executableFn = func() (string, error) { return exePath, nil }
	service.evalSymlinksFn = func(path string) (string, error) { return path, nil }

	_, err := service.PerformRollback(context.Background(), nil, "")

	require.ErrorIs(t, err, ErrSystemUpdateDisabled)
	assertFileContent(t, exePath, "new")
}

func TestStartSystemUpdateTaskReturnsActiveTaskBeforeCheckingRelease(t *testing.T) {
	setupSystemTaskServiceTestDB(t)
	activeTask, err := model.CreateSystemTaskWithActiveKey(model.SystemTaskTypeSystemUpdate, systemUpdateActiveKey, nil, nil)
	require.NoError(t, err)

	task, err := StartSystemUpdateTask(context.Background())

	require.NoError(t, err)
	require.Equal(t, activeTask.TaskID, task.TaskID)
}

func TestStartSystemRollbackTaskReturnsActiveTaskBeforeCheckingRelease(t *testing.T) {
	setupSystemTaskServiceTestDB(t)
	activeTask, err := model.CreateSystemTaskWithActiveKey(model.SystemTaskTypeSystemUpdate, systemUpdateActiveKey, nil, nil)
	require.NoError(t, err)

	task, err := StartSystemRollbackTask(context.Background(), "v1.0.0")

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

type dockerTestRoundTripper func(*http.Request) (*http.Response, error)

func (f dockerTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type fakeDockerEngine struct {
	mu sync.Mutex

	containers           map[string]*dockerInspectContainer
	nextID               int
	pullStatuses         []int
	pullCalls            int
	healthByImage        map[string]string
	healthByNameImage    map[string]string
	failRenameTo         map[string]int
	failStartFor         map[string]int
	failStartImage       map[string]int
	failStartName        map[string]int
	failStartAll         int
	startStateError      string
	failStopFor          map[string]int
	failCreate           int
	enforcePortConflicts bool
	createRequests       []dockerCreateRecord
	startedNames         []string
	events               []string
}

type dockerCreateRecord struct {
	Name    string
	Request dockerCreateContainerRequest
}

func newFakeDockerEngine() *fakeDockerEngine {
	return &fakeDockerEngine{
		containers:        map[string]*dockerInspectContainer{},
		healthByImage:     map[string]string{},
		healthByNameImage: map[string]string{},
		failRenameTo:      map[string]int{},
		failStartFor:      map[string]int{},
		failStartImage:    map[string]int{},
		failStartName:     map[string]int{},
		failStopFor:       map[string]int{},
		events:            []string{},
	}
}

func (f *fakeDockerEngine) addContainer(container *dockerInspectContainer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if container.ID == "" {
		f.nextID++
		container.ID = fmt.Sprintf("container-%d", f.nextID)
	}
	if container.Name == "" {
		container.Name = "/" + container.ID
	}
	f.containers[container.ID] = container
	f.containers[cleanDockerContainerName(container.Name)] = container
}

func (f *fakeDockerEngine) findContainer(idOrName string) *dockerInspectContainer {
	idOrName = cleanDockerContainerName(idOrName)
	if container := f.containers[idOrName]; container != nil {
		return container
	}
	for _, container := range f.containers {
		if container.ID == idOrName || cleanDockerContainerName(container.Name) == idOrName {
			return container
		}
	}
	return nil
}

func (f *fakeDockerEngine) removeContainer(container *dockerInspectContainer) {
	if container == nil {
		return
	}
	delete(f.containers, container.ID)
	delete(f.containers, cleanDockerContainerName(container.Name))
}

func (f *fakeDockerEngine) hasContainerNamePrefix(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := map[string]struct{}{}
	for _, container := range f.containers {
		if _, exists := seen[container.ID]; exists {
			continue
		}
		seen[container.ID] = struct{}{}
		if strings.HasPrefix(cleanDockerContainerName(container.Name), prefix) {
			return true
		}
	}
	return false
}

func (f *fakeDockerEngine) runningPortConflict(candidate *dockerInspectContainer) (string, bool) {
	if candidate == nil {
		return "", false
	}
	candidatePorts := dockerTestPublishedPorts(candidate.HostConfig.PortBindings)
	if len(candidatePorts) == 0 {
		return "", false
	}
	seen := map[string]struct{}{}
	for _, container := range f.containers {
		if container == nil || container.ID == candidate.ID || !container.State.Running {
			continue
		}
		if _, exists := seen[container.ID]; exists {
			continue
		}
		seen[container.ID] = struct{}{}
		runningPorts := dockerTestPublishedPorts(container.HostConfig.PortBindings)
		for port := range candidatePorts {
			if _, exists := runningPorts[port]; exists {
				return port, true
			}
		}
	}
	return "", false
}

func dockerTestPublishedPorts(bindings map[string][]dockerPortBinding) map[string]struct{} {
	ports := map[string]struct{}{}
	for _, portBindings := range bindings {
		for _, binding := range portBindings {
			hostPort := strings.TrimSpace(binding.HostPort)
			if hostPort == "" {
				continue
			}
			hostIP := strings.TrimSpace(binding.HostIP)
			if hostIP == "" {
				hostIP = "0.0.0.0"
			}
			ports[hostIP+":"+hostPort] = struct{}{}
			if hostIP == "0.0.0.0" {
				ports[hostPort] = struct{}{}
			}
		}
	}
	return ports
}

func (f *fakeDockerEngine) client(t *testing.T) (*dockerEngineClient, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(f.handle))
	baseURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := newDockerEngineClient("unused")
	client.httpClient = &http.Client{
		Transport: dockerTestRoundTripper(func(request *http.Request) (*http.Response, error) {
			proxyRequest := request.Clone(request.Context())
			proxyRequest.URL.Scheme = baseURL.Scheme
			proxyRequest.URL.Host = baseURL.Host
			proxyRequest.Host = ""
			return http.DefaultTransport.RoundTrip(proxyRequest)
		}),
	}
	return client, server.Close
}

func (f *fakeDockerEngine) handle(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodPost && request.URL.Path == "/containers/create" {
		f.handleCreate(writer, request)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	if request.URL.Path == "/_ping" {
		writer.WriteHeader(http.StatusOK)
		return
	}
	if request.URL.Path == "/images/create" {
		f.pullCalls++
		statusCode := http.StatusOK
		if len(f.pullStatuses) >= f.pullCalls {
			statusCode = f.pullStatuses[f.pullCalls-1]
		}
		if statusCode != http.StatusOK {
			http.Error(writer, "temporary registry failure", statusCode)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"status":"Pulled"}`+"\n")
		return
	}
	if !strings.HasPrefix(request.URL.Path, "/containers/") {
		http.NotFound(writer, request)
		return
	}

	parts := strings.Split(strings.TrimPrefix(request.URL.Path, "/containers/"), "/")
	if len(parts) == 0 {
		http.NotFound(writer, request)
		return
	}
	idOrName, err := url.PathUnescape(parts[0])
	if err != nil {
		http.Error(writer, "bad container", http.StatusBadRequest)
		return
	}

	switch {
	case request.Method == http.MethodGet && len(parts) == 2 && parts[1] == "json":
		container := f.findContainer(idOrName)
		if container == nil {
			http.Error(writer, "No such container", http.StatusNotFound)
			return
		}
		writeDockerTestJSON(writer, container)
	case request.Method == http.MethodPost && len(parts) == 2 && parts[1] == "start":
		container := f.findContainer(idOrName)
		if container == nil {
			http.Error(writer, "No such container", http.StatusNotFound)
			return
		}
		if f.failStartAll > 0 {
			f.failStartAll--
			http.Error(writer, "start failed", http.StatusInternalServerError)
			return
		}
		if f.failStartFor[container.ID] > 0 {
			f.failStartFor[container.ID]--
			http.Error(writer, "start failed", http.StatusInternalServerError)
			return
		}
		containerName := cleanDockerContainerName(container.Name)
		if f.failStartName[containerName] > 0 {
			f.failStartName[containerName]--
			http.Error(writer, "start failed", http.StatusInternalServerError)
			return
		}
		if f.failStartImage[container.Config.Image] > 0 {
			f.failStartImage[container.Config.Image]--
			http.Error(writer, "start failed", http.StatusInternalServerError)
			return
		}
		if f.enforcePortConflicts {
			if hostPort, ok := f.runningPortConflict(container); ok {
				http.Error(writer, "failed to set up container networking: driver failed programming external connectivity on endpoint test: Bind for 0.0.0.0:"+hostPort+" failed: port is already allocated", http.StatusInternalServerError)
				return
			}
		}
		container.State.Running = true
		container.State.Status = "running"
		container.State.Error = f.startStateError
		if container.Config.Healthcheck != nil {
			healthStatus := f.healthByNameImage[containerName+"\x00"+container.Config.Image]
			if healthStatus == "" {
				healthStatus = f.healthByImage[container.Config.Image]
			}
			if healthStatus == "" {
				healthStatus = "healthy"
			}
			container.State.Health = &dockerContainerHealth{Status: healthStatus}
		}
		f.startedNames = append(f.startedNames, containerName)
		f.events = append(f.events, "start:"+container.Config.Image)
		writer.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodPost && len(parts) == 2 && parts[1] == "stop":
		container := f.findContainer(idOrName)
		if container == nil {
			http.Error(writer, "No such container", http.StatusNotFound)
			return
		}
		if f.failStopFor[container.ID] > 0 {
			f.failStopFor[container.ID]--
			http.Error(writer, "stop failed", http.StatusInternalServerError)
			return
		}
		container.State.Running = false
		container.State.Status = "exited"
		f.events = append(f.events, "stop:"+container.ID)
		writer.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodPost && len(parts) == 2 && parts[1] == "rename":
		container := f.findContainer(idOrName)
		if container == nil {
			http.Error(writer, "No such container", http.StatusNotFound)
			return
		}
		name := cleanDockerContainerName(request.URL.Query().Get("name"))
		if f.failRenameTo[name] > 0 {
			f.failRenameTo[name]--
			http.Error(writer, "rename failed", http.StatusInternalServerError)
			return
		}
		if existing := f.findContainer(name); existing != nil && existing.ID != container.ID {
			http.Error(writer, "name already in use", http.StatusConflict)
			return
		}
		delete(f.containers, cleanDockerContainerName(container.Name))
		container.Name = "/" + name
		f.containers[name] = container
		f.events = append(f.events, "rename:"+container.ID+":"+name)
		writer.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodDelete && len(parts) == 1:
		container := f.findContainer(idOrName)
		if container == nil {
			http.Error(writer, "No such container", http.StatusNotFound)
			return
		}
		f.removeContainer(container)
		writer.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(writer, request)
	}
}

func (f *fakeDockerEngine) handleCreate(writer http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.failCreate > 0 {
		f.failCreate--
		http.Error(writer, "create failed", http.StatusInternalServerError)
		return
	}

	var payload dockerCreateContainerRequest
	data, err := io.ReadAll(request.Body)
	if err != nil {
		http.Error(writer, "read failed", http.StatusBadRequest)
		return
	}
	if err := common.Unmarshal(data, &payload); err != nil {
		http.Error(writer, "decode failed", http.StatusBadRequest)
		return
	}
	name := cleanDockerContainerName(request.URL.Query().Get("name"))
	f.createRequests = append(f.createRequests, dockerCreateRecord{Name: name, Request: payload})
	f.nextID++
	container := &dockerInspectContainer{
		ID:    fmt.Sprintf("container-%d", f.nextID),
		Name:  "/" + name,
		Image: payload.Image,
		Config: dockerContainerConfig{
			Image:        payload.Image,
			Env:          payload.Env,
			Cmd:          payload.Cmd,
			Entrypoint:   payload.Entrypoint,
			WorkingDir:   payload.WorkingDir,
			User:         payload.User,
			Labels:       payload.Labels,
			ExposedPorts: payload.ExposedPorts,
			Healthcheck:  payload.Healthcheck,
		},
		HostConfig: payload.HostConfig,
		NetworkSettings: dockerContainerNetworkSettings{
			Networks: payload.NetworkingConfig.EndpointsConfig,
		},
		State: dockerContainerState{
			Status: "created",
		},
	}
	if container.Name == "/" {
		container.Name = "/" + container.ID
	}
	f.containers[container.ID] = container
	f.containers[cleanDockerContainerName(container.Name)] = container
	f.events = append(f.events, "create:"+container.Config.Image)
	writeDockerTestJSON(writer, dockerCreateContainerResponse{ID: container.ID})
}

func writeDockerTestJSON(writer http.ResponseWriter, value any) {
	data, err := common.Marshal(value)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(data)
}

func dockerTestService(fake *fakeDockerEngine, currentID *string) *SystemUpdateService {
	service := NewSystemUpdateService(nil)
	service.isContainerFn = func() bool { return true }
	service.dockerSocketPathFn = func() string { return "unused" }
	service.currentContainerIDFn = func() string { return *currentID }
	service.backupFn = func(context.Context, string) (*SystemUpdateBackup, error) {
		return &SystemUpdateBackup{ID: "docker-backup-test", Status: systemUpdateBackupStatusReady}, nil
	}
	service.restoreFn = func(context.Context, string) error { return nil }
	return service
}

func TestDockerPullRetriesTransientErrorsButNotPermanentErrors(t *testing.T) {
	fake := newFakeDockerEngine()
	fake.pullStatuses = []int{http.StatusBadGateway, http.StatusOK}
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	client.pullRetryDelays = []time.Duration{0}

	require.NoError(t, client.pullImage(context.Background(), "c1cadabob/nexustok:latest", nil))
	assert.Equal(t, 2, fake.pullCalls)

	fake = newFakeDockerEngine()
	fake.pullStatuses = []int{http.StatusNotFound, http.StatusOK}
	client, closeServer = fake.client(t)
	t.Cleanup(closeServer)
	client.pullRetryDelays = []time.Duration{0, 0}

	require.Error(t, client.pullImage(context.Background(), "missing/image:latest", nil))
	assert.Equal(t, 1, fake.pullCalls)
}

func TestDockerReadinessClassifiesHealthcheckStatesAndNoHealthcheck(t *testing.T) {
	originalInterval := systemUpdateDockerHealthPollInterval
	systemUpdateDockerHealthPollInterval = 0
	t.Cleanup(func() { systemUpdateDockerHealthPollInterval = originalInterval })

	fake := newFakeDockerEngine()
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)

	fake.addContainer(&dockerInspectContainer{
		ID:   "healthy",
		Name: "/healthy",
		Config: dockerContainerConfig{
			Image:       "image:healthy",
			Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
		},
		State: dockerContainerState{
			Running: true,
			Status:  "running",
			Health:  &dockerContainerHealth{Status: "healthy"},
		},
	})
	readiness, err := waitDockerContainerReady(context.Background(), client, "healthy", time.Second)
	require.NoError(t, err)
	assert.Equal(t, "healthy", readiness.Status)
	assert.True(t, readiness.HealthcheckAvailable)
	assert.False(t, readiness.HealthcheckDegraded)

	fake.addContainer(&dockerInspectContainer{
		ID:   "unhealthy",
		Name: "/unhealthy",
		Config: dockerContainerConfig{
			Image:       "image:unhealthy",
			Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "false"}},
		},
		State: dockerContainerState{
			Running: true,
			Status:  "running",
			Health:  &dockerContainerHealth{Status: "unhealthy"},
		},
	})
	_, err = waitDockerContainerReady(context.Background(), client, "unhealthy", time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unhealthy")

	fake.addContainer(&dockerInspectContainer{
		ID:     "without-healthcheck",
		Name:   "/without-healthcheck",
		Config: dockerContainerConfig{Image: "image:plain"},
		State:  dockerContainerState{Running: true, Status: "running"},
	})
	readiness, err = waitDockerContainerReady(context.Background(), client, "without-healthcheck", time.Second)
	require.NoError(t, err)
	assert.Equal(t, "running_without_healthcheck", readiness.Status)
	assert.False(t, readiness.HealthcheckAvailable)
	assert.True(t, readiness.HealthcheckDegraded)

	fake.addContainer(&dockerInspectContainer{
		ID:   "startup-error",
		Name: "/startup-error",
		Config: dockerContainerConfig{
			Image:       "image:startup-error",
			Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
		},
		State: dockerContainerState{
			Running: true,
			Status:  "running",
			Error:   "SESSION_SECRET=hidden-value",
		},
	})
	_, err = waitDockerContainerReady(context.Background(), client, "startup-error", time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SESSION_SECRET=***")
	assert.NotContains(t, err.Error(), "hidden-value")
}

func TestDockerUpdatedContainerStartErrorExplainsPortConflictAndMasksSecrets(t *testing.T) {
	err := dockerUpdatedContainerStartError(
		"start updated container failed",
		errors.New(`Docker Engine returned 500: {"message":"Bind for 0.0.0.0:3030 failed: port is already allocated","SESSION_SECRET":"hidden","SQL_DSN":"postgres://root:secret@example/nexustok","Authorization":"Bearer token-secret"}`),
	)

	require.Error(t, err)
	text := err.Error()
	assert.Contains(t, text, "Docker host port is already allocated")
	assert.Contains(t, text, "portless preflight")
	assert.Contains(t, text, "SESSION_SECRET")
	assert.NotContains(t, text, "hidden")
	assert.NotContains(t, text, "secret@example")
	assert.NotContains(t, text, "token-secret")
}

func TestDockerUpdateKeepsOnlyFormalTargetContainer(t *testing.T) {
	fake := newFakeDockerEngine()
	current := &dockerInspectContainer{
		ID:   "current-id",
		Name: "/nexustok",
		Config: dockerContainerConfig{
			Image: "c1cadabob/nexustok:old",
			Env:   []string{"PORT=3030", "SESSION_SECRET=not-for-output"},
			Healthcheck: &dockerHealthcheck{
				Test: []string{"CMD-SHELL", "true"},
			},
		},
		State: dockerContainerState{
			Running: true,
			Status:  "running",
			Health:  &dockerContainerHealth{Status: "healthy"},
		},
	}
	fake.addContainer(current)
	currentID := current.ID
	service := dockerTestService(fake, &currentID)
	originalFactory := systemUpdateDockerClientFactory
	t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

	result, err := service.performDockerUpdateInProcess(
		context.Background(),
		nil,
		"runner",
		dockerHelperOptions{
			TargetImage: "c1cadabob/nexustok:new",
			Mode:        systemUpdateDeploymentDockerRun,
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "c1cadabob/nexustok:new", fake.findContainer("nexustok").Config.Image)
	assert.Nil(t, fake.findContainer("nexustok.backup"))
	assert.False(t, result.RollbackAvailable)
	assert.Equal(t, "healthy", result.HealthStatus)
	startIndex := -1
	stopIndex := -1
	for index, event := range fake.events {
		if event == "start:c1cadabob/nexustok:new" && startIndex < 0 {
			startIndex = index
		}
		if event == "stop:current-id" && stopIndex < 0 {
			stopIndex = index
		}
	}
	assert.GreaterOrEqual(t, startIndex, 0)
	assert.Greater(t, stopIndex, startIndex)
	assert.False(t, fake.hasContainerNamePrefix("nexustok.staging-"))
	assert.False(t, fake.hasContainerNamePrefix("nexustok.failed-"))
}

func TestDockerUpdateUsesPortlessPreflightBeforeReplacingPortBoundContainer(t *testing.T) {
	fake := newFakeDockerEngine()
	fake.enforcePortConflicts = true
	current := &dockerInspectContainer{
		ID:   "current-id",
		Name: "/nexustok",
		Config: dockerContainerConfig{
			Image: "c1cadabob/nexustok:old",
			Env:   []string{"PORT=3030", "SESSION_SECRET=not-for-output"},
			Labels: map[string]string{
				"com.docker.compose.project":     "nexustok",
				"com.docker.compose.service":     "nexustok",
				"dev.c1cada.nexustok.keep-label": "true",
			},
			ExposedPorts: map[string]any{"3030/tcp": struct{}{}},
			Healthcheck:  &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
		},
		HostConfig: dockerHostConfig{
			PortBindings: map[string][]dockerPortBinding{
				"3030/tcp": {{HostIP: "0.0.0.0", HostPort: "3030"}},
			},
			RestartPolicy: dockerRestartPolicy{Name: "always"},
			NetworkMode:   "nexustok-network",
			Binds:         []string{"/opt/nexustok/data:/data"},
		},
		NetworkSettings: dockerContainerNetworkSettings{
			Networks: map[string]dockerEndpointSettings{
				"nexustok-network": {Aliases: []string{"nexustok", "nexustok-1"}},
			},
		},
		State: dockerContainerState{
			Running: true,
			Status:  "running",
			Health:  &dockerContainerHealth{Status: "healthy"},
		},
	}
	fake.addContainer(current)
	currentID := current.ID
	service := dockerTestService(fake, &currentID)
	originalFactory := systemUpdateDockerClientFactory
	t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

	result, err := service.performDockerUpdateInProcess(
		context.Background(),
		nil,
		"runner",
		dockerHelperOptions{TargetImage: "c1cadabob/nexustok:new"},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, fake.createRequests, 2)
	preflight := fake.createRequests[0]
	final := fake.createRequests[1]
	assert.Contains(t, preflight.Name, "nexustok.preflight-")
	assert.Contains(t, final.Name, "nexustok.staging-")
	assert.Empty(t, preflight.Request.HostConfig.PortBindings)
	assert.Equal(t, "no", preflight.Request.HostConfig.RestartPolicy.Name)
	assert.NotContains(t, preflight.Request.Labels, "com.docker.compose.project")
	assert.NotContains(t, preflight.Request.Labels, "com.docker.compose.service")
	assert.Equal(t, dockerEndpointSettings{}, preflight.Request.NetworkingConfig.EndpointsConfig["nexustok-network"])
	assert.Equal(t, current.HostConfig.PortBindings, final.Request.HostConfig.PortBindings)
	assert.Equal(t, "always", final.Request.HostConfig.RestartPolicy.Name)
	assert.Equal(t, "nexustok", final.Request.Labels["com.docker.compose.project"])
	assert.Equal(t, "nexustok", final.Request.Labels["com.docker.compose.service"])
	assert.Equal(t, []string{"nexustok", "nexustok-1"}, final.Request.NetworkingConfig.EndpointsConfig["nexustok-network"].Aliases)
	assert.Equal(t, "c1cadabob/nexustok:new", fake.findContainer("nexustok").Config.Image)
	assert.Equal(t, []string{preflight.Name, "nexustok"}, fake.startedNames)
	assert.False(t, fake.hasContainerNamePrefix("nexustok.preflight-"))
	assert.False(t, fake.hasContainerNamePrefix("nexustok.staging-"))
	assert.False(t, fake.hasContainerNamePrefix("nexustok.failed-"))
}

func TestReplaceComposeServiceImageOnlyChangesTargetService(t *testing.T) {
	var document yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(`
services:
  nexustok:
    image: c1cadabob/nexustok:old
  postgres:
    image: postgres:15-alpine
`), &document))

	require.True(t, replaceComposeServiceImage(&document, "nexustok", "c1cadabob/nexustok:v0.2.7"))
	data, err := yaml.Marshal(&document)
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "image: c1cadabob/nexustok:v0.2.7")
	assert.Contains(t, text, "image: postgres:15-alpine")
}

func TestPrepareComposeImageUpdatesAtomicallyAndCanRestore(t *testing.T) {
	dir := t.TempDir()
	absoluteComposePath := filepath.Join(dir, "compose.yml")
	original := []byte("services:\n  nexustok:\n    image: c1cadabob/nexustok:old\n")
	require.NoError(t, os.WriteFile(absoluteComposePath, original, 0640))
	composePath, err := filepath.Rel(dir, absoluteComposePath)
	require.NoError(t, err)
	current := &dockerInspectContainer{
		Config: dockerContainerConfig{
			Labels: map[string]string{
				"com.docker.compose.service":              "nexustok",
				"com.docker.compose.project.working_dir":  dir,
				"com.docker.compose.project.config_files": composePath,
			},
		},
	}

	backups, err := prepareSystemUpdateComposeImage(current, "c1cadabob/nexustok:v0.2.7")
	require.NoError(t, err)
	updated, err := os.ReadFile(absoluteComposePath)
	require.NoError(t, err)
	assert.Contains(t, string(updated), "c1cadabob/nexustok:v0.2.7")
	assert.NotEqual(t, string(original), string(updated))
	require.NoError(t, restoreSystemUpdateComposeFiles(backups))
	restored, err := os.ReadFile(absoluteComposePath)
	require.NoError(t, err)
	assert.Equal(t, string(original), string(restored))
	assert.Equal(t, os.FileMode(0640), fileModePerm(t, absoluteComposePath))
}

func TestPrepareComposeImageValidatesAllFilesBeforeUpdatingAny(t *testing.T) {
	directory := t.TempDir()
	composePath := filepath.Join(directory, "docker-compose.yml")
	original := []byte("services:\n  nexustok:\n    image: c1cadabob/nexustok:old\n")
	require.NoError(t, os.WriteFile(composePath, original, 0640))
	invalidPath := filepath.Join(directory, "invalid.yml")
	require.NoError(t, os.Mkdir(invalidPath, 0750))
	current := &dockerInspectContainer{Config: dockerContainerConfig{Labels: map[string]string{
		"com.docker.compose.service":              "nexustok",
		"com.docker.compose.project.working_dir":  directory,
		"com.docker.compose.project.config_files": "docker-compose.yml,invalid.yml",
	}}}

	_, err := prepareSystemUpdateComposeImage(current, "c1cadabob/nexustok:v0.2.10")
	require.Error(t, err)
	actual, readErr := os.ReadFile(composePath)
	require.NoError(t, readErr)
	assert.Equal(t, original, actual)
}

func TestDockerUpdateFailureRestoresCurrentAndRemovesStaging(t *testing.T) {
	fake := newFakeDockerEngine()
	current := &dockerInspectContainer{
		ID:   "current-id",
		Name: "/nexustok",
		Config: dockerContainerConfig{
			Image:       "c1cadabob/nexustok:old",
			Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
		},
		State: dockerContainerState{
			Running: true,
			Status:  "running",
			Health:  &dockerContainerHealth{Status: "healthy"},
		},
	}
	backup := &dockerInspectContainer{
		ID:     "backup-id",
		Name:   "/nexustok.backup",
		Config: dockerContainerConfig{Image: "c1cadabob/nexustok:previous"},
		State:  dockerContainerState{Status: "exited"},
	}
	fake.addContainer(current)
	fake.addContainer(backup)
	fake.failRenameTo["nexustok"] = 1
	currentID := current.ID
	service := dockerTestService(fake, &currentID)
	originalFactory := systemUpdateDockerClientFactory
	t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

	_, err := service.performDockerUpdateInProcess(
		context.Background(),
		nil,
		"runner",
		dockerHelperOptions{TargetImage: "c1cadabob/nexustok:new"},
	)

	require.Error(t, err)
	assert.Equal(t, "c1cadabob/nexustok:old", fake.findContainer("nexustok").Config.Image)
	assert.Nil(t, fake.findContainer("nexustok.backup"))
	assert.NotNil(t, fake.findContainer("nexustok"))
	assert.False(t, fake.hasContainerNamePrefix("nexustok.staging-"))
}

func TestDockerUpdateFailureBeforeReplacementKeepsCurrentAndBackup(t *testing.T) {
	tests := []struct {
		name  string
		setup func(fake *fakeDockerEngine, current *dockerInspectContainer)
	}{
		{
			name: "create failure",
			setup: func(fake *fakeDockerEngine, _ *dockerInspectContainer) {
				fake.failCreate = 1
			},
		},
		{
			name: "candidate start failure",
			setup: func(fake *fakeDockerEngine, _ *dockerInspectContainer) {
				fake.failStartImage["c1cadabob/nexustok:new"] = 1
			},
		},
		{
			name: "candidate health failure",
			setup: func(fake *fakeDockerEngine, _ *dockerInspectContainer) {
				fake.healthByImage["c1cadabob/nexustok:new"] = "unhealthy"
			},
		},
		{
			name: "current stop failure",
			setup: func(fake *fakeDockerEngine, current *dockerInspectContainer) {
				fake.failStopFor[current.ID] = 1
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeDockerEngine()
			current := &dockerInspectContainer{
				ID:   "current-id",
				Name: "/nexustok",
				Config: dockerContainerConfig{
					Image:       "c1cadabob/nexustok:old",
					Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
				},
				State: dockerContainerState{Running: true, Status: "running"},
			}
			backup := &dockerInspectContainer{
				ID:     "backup-id",
				Name:   "/nexustok.backup",
				Config: dockerContainerConfig{Image: "c1cadabob/nexustok:previous"},
				State:  dockerContainerState{Status: "exited"},
			}
			fake.addContainer(current)
			fake.addContainer(backup)
			tt.setup(fake, current)

			currentID := current.ID
			service := dockerTestService(fake, &currentID)
			originalFactory := systemUpdateDockerClientFactory
			t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
			client, closeServer := fake.client(t)
			t.Cleanup(closeServer)
			systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

			_, err := service.performDockerUpdateInProcess(
				context.Background(),
				nil,
				"runner",
				dockerHelperOptions{TargetImage: "c1cadabob/nexustok:new"},
			)

			require.Error(t, err)
			assert.Equal(t, "c1cadabob/nexustok:old", fake.findContainer("nexustok").Config.Image)
			assert.True(t, fake.findContainer("nexustok").State.Running)
			assert.Nil(t, fake.findContainer("nexustok.backup"))
			assert.False(t, fake.hasContainerNamePrefix("nexustok.staging-"))
			assert.False(t, fake.hasContainerNamePrefix("nexustok.failed-"))
		})
	}
}

func TestDockerUpdateFailureAfterFinalStartRestoresCurrentAndBackup(t *testing.T) {
	tests := []struct {
		name  string
		setup func(fake *fakeDockerEngine)
	}{
		{
			name: "final start failure",
			setup: func(fake *fakeDockerEngine) {
				fake.failStartName["nexustok"] = 1
			},
		},
		{
			name: "final health failure",
			setup: func(fake *fakeDockerEngine) {
				fake.healthByNameImage["nexustok\x00c1cadabob/nexustok:new"] = "unhealthy"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeDockerEngine()
			current := &dockerInspectContainer{
				ID:   "current-id",
				Name: "/nexustok",
				Config: dockerContainerConfig{
					Image:       "c1cadabob/nexustok:old",
					Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
				},
				HostConfig: dockerHostConfig{
					PortBindings: map[string][]dockerPortBinding{
						"3030/tcp": {{HostPort: "3030"}},
					},
					RestartPolicy: dockerRestartPolicy{Name: "always"},
				},
				State: dockerContainerState{Running: true, Status: "running"},
			}
			backup := &dockerInspectContainer{
				ID:     "backup-id",
				Name:   "/nexustok.backup",
				Config: dockerContainerConfig{Image: "c1cadabob/nexustok:previous"},
				State:  dockerContainerState{Status: "exited"},
			}
			fake.addContainer(current)
			fake.addContainer(backup)
			tt.setup(fake)

			currentID := current.ID
			service := dockerTestService(fake, &currentID)
			originalFactory := systemUpdateDockerClientFactory
			t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
			client, closeServer := fake.client(t)
			t.Cleanup(closeServer)
			systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

			_, err := service.performDockerUpdateInProcess(
				context.Background(),
				nil,
				"runner",
				dockerHelperOptions{TargetImage: "c1cadabob/nexustok:new"},
			)

			require.Error(t, err)
			assert.Equal(t, "c1cadabob/nexustok:old", fake.findContainer("nexustok").Config.Image)
			assert.True(t, fake.findContainer("nexustok").State.Running)
			assert.Nil(t, fake.findContainer("nexustok.backup"))
			assert.False(t, fake.hasContainerNamePrefix("nexustok.preflight-"))
			assert.False(t, fake.hasContainerNamePrefix("nexustok.staging-"))
			assert.False(t, fake.hasContainerNamePrefix("nexustok.failed-"))
		})
	}
}

func TestDockerUpdateDataBackupFailureKeepsCurrentContainerRunning(t *testing.T) {
	fake := newFakeDockerEngine()
	current := &dockerInspectContainer{
		ID:   "current-id",
		Name: "/nexustok",
		Config: dockerContainerConfig{
			Image:       "c1cadabob/nexustok:old",
			Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
		},
		State: dockerContainerState{Running: true, Status: "running"},
	}
	fake.addContainer(current)
	currentID := current.ID
	service := dockerTestService(fake, &currentID)
	service.backupFn = func(context.Context, string) (*SystemUpdateBackup, error) {
		return nil, errors.New("backup unavailable")
	}
	originalFactory := systemUpdateDockerClientFactory
	t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

	_, err := service.performDockerUpdateInProcess(
		context.Background(),
		nil,
		"runner",
		dockerHelperOptions{TargetImage: "c1cadabob/nexustok:new"},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create system update data backup failed")
	assert.True(t, fake.findContainer("nexustok").State.Running)
	assert.Equal(t, "c1cadabob/nexustok:old", fake.findContainer("nexustok").Config.Image)
	assert.NotEmpty(t, fake.createRequests)
	assert.Contains(t, fake.createRequests[0].Name, "nexustok.preflight-")
	assert.False(t, fake.hasContainerNamePrefix("nexustok.staging-"))
	assert.NotContains(t, fake.events, "stop:current-id")
}

func TestDockerUpdateDataRestoreFailureIsReportedAfterContainerSwitch(t *testing.T) {
	fake := newFakeDockerEngine()
	current := &dockerInspectContainer{
		ID:   "current-id",
		Name: "/nexustok",
		Config: dockerContainerConfig{
			Image:       "c1cadabob/nexustok:old",
			Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
		},
		State: dockerContainerState{Running: true, Status: "running"},
	}
	fake.addContainer(current)
	fake.healthByNameImage["nexustok\x00c1cadabob/nexustok:new"] = "unhealthy"
	currentID := current.ID
	service := dockerTestService(fake, &currentID)
	service.restoreFn = func(context.Context, string) error {
		return errors.New("restore unavailable")
	}
	originalFactory := systemUpdateDockerClientFactory
	t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

	_, err := service.performDockerUpdateInProcess(
		context.Background(),
		nil,
		"runner",
		dockerHelperOptions{TargetImage: "c1cadabob/nexustok:new"},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "data_restore_failed")
	assert.True(t, fake.findContainer("nexustok").State.Running)
	assert.Equal(t, "c1cadabob/nexustok:old", fake.findContainer("nexustok").Config.Image)
}

func TestDockerUpdateSkipsParallelPreflightForHostAndContainerNetworkModes(t *testing.T) {
	for _, networkMode := range []string{"host", "container:shared-network"} {
		t.Run(networkMode, func(t *testing.T) {
			fake := newFakeDockerEngine()
			current := &dockerInspectContainer{
				ID:   "current-id",
				Name: "/nexustok",
				Config: dockerContainerConfig{
					Image:       "c1cadabob/nexustok:old",
					Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
				},
				HostConfig: dockerHostConfig{
					NetworkMode: networkMode,
				},
				State: dockerContainerState{Running: true, Status: "running"},
			}
			fake.addContainer(current)
			currentID := current.ID
			service := dockerTestService(fake, &currentID)
			originalFactory := systemUpdateDockerClientFactory
			t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
			client, closeServer := fake.client(t)
			t.Cleanup(closeServer)
			systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

			result, err := service.performDockerUpdateInProcess(
				context.Background(),
				nil,
				"runner",
				dockerHelperOptions{TargetImage: "c1cadabob/nexustok:new"},
			)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, fake.createRequests, 1)
			assert.Contains(t, fake.createRequests[0].Name, "nexustok.staging-")
			assert.Equal(t, networkMode, fake.createRequests[0].Request.HostConfig.NetworkMode)
			assert.Equal(t, []string{"nexustok"}, fake.startedNames)
			assert.Equal(t, "c1cadabob/nexustok:new", fake.findContainer("nexustok").Config.Image)
		})
	}
}

func TestDockerUpdateIgnoresLegacyBackupContainer(t *testing.T) {
	fake := newFakeDockerEngine()
	current := &dockerInspectContainer{
		ID:   "current-id",
		Name: "/nexustok",
		Config: dockerContainerConfig{
			Image:       "c1cadabob/nexustok:old",
			Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
		},
		State: dockerContainerState{Running: true, Status: "running"},
	}
	backup := &dockerInspectContainer{
		ID:     "backup-id",
		Name:   "/nexustok.backup",
		Config: dockerContainerConfig{Image: "c1cadabob/nexustok:previous"},
		State:  dockerContainerState{Status: "exited"},
	}
	fake.addContainer(current)
	fake.addContainer(backup)
	currentID := current.ID
	service := dockerTestService(fake, &currentID)
	originalFactory := systemUpdateDockerClientFactory
	t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

	result, err := service.performDockerUpdateInProcess(
		context.Background(),
		nil,
		"runner",
		dockerHelperOptions{TargetImage: "c1cadabob/nexustok:new"},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "c1cadabob/nexustok:new", fake.findContainer("nexustok").Config.Image)
	assert.Nil(t, fake.findContainer("nexustok.backup"))
	assert.False(t, fake.hasContainerNamePrefix("nexustok.staging-"))
	assert.False(t, fake.hasContainerNamePrefix("nexustok.failed-"))
}

func TestDockerUpdateHealthcheckStartingTimesOutAndRestoresCurrent(t *testing.T) {
	originalInterval := systemUpdateDockerHealthPollInterval
	systemUpdateDockerHealthPollInterval = 0
	t.Cleanup(func() { systemUpdateDockerHealthPollInterval = originalInterval })

	fake := newFakeDockerEngine()
	current := &dockerInspectContainer{
		ID:   "current-id",
		Name: "/nexustok",
		Config: dockerContainerConfig{
			Image:       "c1cadabob/nexustok:old",
			Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
		},
		State: dockerContainerState{Running: true, Status: "running"},
	}
	fake.addContainer(current)
	fake.healthByImage["c1cadabob/nexustok:new"] = "starting"
	currentID := current.ID
	service := dockerTestService(fake, &currentID)
	originalFactory := systemUpdateDockerClientFactory
	t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, err := service.performDockerUpdateInProcess(
		ctx,
		nil,
		"runner",
		dockerHelperOptions{TargetImage: "c1cadabob/nexustok:new"},
	)

	require.Error(t, err)
	assert.Equal(t, "c1cadabob/nexustok:old", fake.findContainer("nexustok").Config.Image)
	assert.True(t, fake.findContainer("nexustok").State.Running)
	assert.False(t, fake.hasContainerNamePrefix("nexustok.staging-"))
}

func TestDockerRollbackUsesExplicitTargetImage(t *testing.T) {
	fake := newFakeDockerEngine()
	current := &dockerInspectContainer{
		ID:   "current-id",
		Name: "/nexustok",
		Config: dockerContainerConfig{
			Image:       "c1cadabob/nexustok:new",
			Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
		},
		State: dockerContainerState{
			Running: true,
			Status:  "running",
			Health:  &dockerContainerHealth{Status: "healthy"},
		},
	}
	fake.addContainer(current)
	currentID := current.ID
	service := dockerTestService(fake, &currentID)
	originalFactory := systemUpdateDockerClientFactory
	t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

	_, err := service.performDockerRollbackInProcess(
		context.Background(),
		nil,
		"runner",
		dockerHelperOptions{TargetVersion: "v0.2.7", TargetImage: "c1cadabob/nexustok:v0.2.7"},
	)
	require.NoError(t, err)
	assert.Equal(t, "c1cadabob/nexustok:v0.2.7", fake.findContainer("nexustok").Config.Image)
	assert.Nil(t, fake.findContainer("nexustok.backup"))
}

func TestDockerRollbackFailureRestoresCurrentContainer(t *testing.T) {
	tests := []struct {
		name  string
		setup func(fake *fakeDockerEngine, current *dockerInspectContainer)
	}{
		{
			name: "restore rename failure",
			setup: func(fake *fakeDockerEngine, _ *dockerInspectContainer) {
				fake.failRenameTo["nexustok"] = 1
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeDockerEngine()
			current := &dockerInspectContainer{
				ID:   "current-id",
				Name: "/nexustok",
				Config: dockerContainerConfig{
					Image:       "c1cadabob/nexustok:new",
					Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
				},
				State: dockerContainerState{Running: true, Status: "running"},
			}
			fake.addContainer(current)
			tt.setup(fake, current)

			currentID := current.ID
			service := dockerTestService(fake, &currentID)
			originalFactory := systemUpdateDockerClientFactory
			t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
			client, closeServer := fake.client(t)
			t.Cleanup(closeServer)
			systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }

			_, err := service.performDockerRollbackInProcess(
				context.Background(),
				nil,
				"runner",
				dockerHelperOptions{TargetVersion: "v0.2.7", TargetImage: "c1cadabob/nexustok:v0.2.7"},
			)

			require.Error(t, err)
			assert.Equal(t, "c1cadabob/nexustok:new", fake.findContainer("nexustok").Config.Image)
			assert.True(t, fake.findContainer("nexustok").State.Running)
			assert.Nil(t, fake.findContainer("nexustok.backup"))
			assert.False(t, fake.hasContainerNamePrefix("nexustok.failed-"))
		})
	}
}

func TestDockerStagingAndFailedContainerNamesAreUnique(t *testing.T) {
	stagingNames := map[string]struct{}{}
	failedNames := map[string]struct{}{}
	for range 3 {
		stagingNames[dockerStagingContainerName("nexustok")] = struct{}{}
		failedNames[dockerFailedContainerName("nexustok")] = struct{}{}
	}
	assert.Len(t, stagingNames, 3)
	assert.Len(t, failedNames, 3)
}

func TestDockerLifecycleNamesUseTheFormalContainerBase(t *testing.T) {
	assert.Equal(t, "nexustok", canonicalDockerContainerName("nexustok"))
	assert.Equal(t, "nexustok", canonicalDockerContainerName("/nexustok.backup"))
	assert.Equal(t, "nexustok", canonicalDockerContainerName("nexustok.staging-abc"))
	assert.Equal(t, "nexustok", canonicalDockerContainerName("nexustok-update-helper-abc"))
	assert.Contains(t, dockerFailedContainerName("nexustok.failed-abc"), "nexustok.failed-")
	assert.Contains(t, dockerPreflightContainerName("nexustok.preflight-abc"), "nexustok.preflight-")
	assert.Contains(t, dockerStagingContainerName("nexustok.staging-abc"), "nexustok.staging-")
}

func TestDockerHelperBindsConfiguredSocketWithoutExposingSensitiveEnv(t *testing.T) {
	inspect := &dockerInspectContainer{
		HostConfig: dockerHostConfig{
			Binds: []string{"/data:/data"},
		},
		Mounts: []dockerMount{
			{Source: "/logs", Destination: "/app/logs", RW: true},
		},
	}

	mounts, err := dockerHelperMountsForMode(inspect, "/run/custom-docker.sock", systemUpdateDeploymentDockerRun)
	require.NoError(t, err)

	assert.Contains(t, mounts.Binds, "/data:/data")
	assert.Contains(t, mounts.Binds, "/logs:/app/logs:rw")
	assert.Contains(t, mounts.Binds, "/run/custom-docker.sock:/run/custom-docker.sock")
	assert.NotContains(t, strings.Join(mounts.Binds, "\n"), "SESSION_SECRET")
}

func TestDockerHelperBindsComposeConfigDirectoriesReadWrite(t *testing.T) {
	workingDir := t.TempDir()
	configDir := filepath.Join(workingDir, "config")
	require.NoError(t, os.MkdirAll(configDir, 0755))
	for _, name := range []string{"docker-compose.yml", "docker-compose.override.yml"} {
		require.NoError(t, os.WriteFile(filepath.Join(workingDir, name), []byte("services: {}\n"), 0640))
	}
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "compose.extra.yaml"), []byte("services: {}\n"), 0640))
	inspect := &dockerInspectContainer{
		HostConfig: dockerHostConfig{Binds: []string{"/opt/nexustok/data:/data"}},
		Mounts:     []dockerMount{{Source: "/opt/nexustok/logs", Destination: "/app/logs", RW: true}},
		Config: dockerContainerConfig{Labels: map[string]string{
			"com.docker.compose.project.working_dir": workingDir,
			"com.docker.compose.project.config_files": strings.Join([]string{
				"docker-compose.yml",
				"./docker-compose.override.yml",
				"config/compose.extra.yaml",
			}, ","),
		}},
	}

	mounts, err := dockerHelperMountsForMode(inspect, "/run/custom-docker.sock", systemUpdateDeploymentDockerCompose)
	require.NoError(t, err)
	assert.Contains(t, mounts.Binds, "/opt/nexustok/data:/data")
	assert.Contains(t, mounts.Binds, "/opt/nexustok/logs:/app/logs:rw")
	assert.Contains(t, mounts.Binds, "/run/custom-docker.sock:/run/custom-docker.sock")
	assert.Contains(t, mounts.Mounts, dockerMountSpec{Type: "bind", Source: configDir, Target: configDir})
	assert.Contains(t, mounts.Mounts, dockerMountSpec{Type: "bind", Source: workingDir, Target: workingDir})
	assert.Equal(t, 1, countDockerHelperMountTarget(mounts.Mounts, workingDir))
	assert.Equal(t, 1, countDockerHelperMountTarget(mounts.Mounts, configDir))

	runModeMounts, err := dockerHelperMountsForMode(inspect, "/run/custom-docker.sock", systemUpdateDeploymentDockerRun)
	require.NoError(t, err)
	assert.Empty(t, runModeMounts.Mounts)
}

func countDockerHelperMountTarget(mounts []dockerMountSpec, target string) int {
	count := 0
	for _, mount := range mounts {
		if mount.Target == target {
			count++
		}
	}
	return count
}

func TestDockerHelperComposeMountsRequireResolvableMetadata(t *testing.T) {
	inspect := &dockerInspectContainer{Config: dockerContainerConfig{Labels: map[string]string{
		"com.docker.compose.project.config_files": "docker-compose.yml",
	}}}

	_, err := dockerHelperMountsForMode(inspect, "/run/custom-docker.sock", systemUpdateDeploymentDockerCompose)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "working directory")
}

func TestDockerHelperComposeMountRejectsConflictingBind(t *testing.T) {
	workingDir := t.TempDir()
	composePath := filepath.Join(workingDir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(composePath, []byte("services: {}\n"), 0640))
	inspect := &dockerInspectContainer{
		HostConfig: dockerHostConfig{Binds: []string{"/unrelated/source:" + workingDir + ":rw"}},
		Config: dockerContainerConfig{Labels: map[string]string{
			"com.docker.compose.project.working_dir":  workingDir,
			"com.docker.compose.project.config_files": composePath,
		}},
	}

	_, err := dockerHelperMountsForMode(inspect, "/run/custom-docker.sock", systemUpdateDeploymentDockerCompose)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mounted incompatibly")
}

func TestDockerHelperComposeMountDoesNotRequireHostPathVisibleToApplication(t *testing.T) {
	composeDirectory := filepath.Join(t.TempDir(), "host-compose")
	inspect := &dockerInspectContainer{Config: dockerContainerConfig{Labels: map[string]string{
		"com.docker.compose.project.working_dir":  composeDirectory,
		"com.docker.compose.project.config_files": "docker-compose.yml",
	}}}

	mounts, err := dockerHelperMountsForMode(inspect, "/run/custom-docker.sock", systemUpdateDeploymentDockerCompose)
	require.NoError(t, err)
	assert.Contains(t, mounts.Mounts, dockerMountSpec{Type: "bind", Source: composeDirectory, Target: composeDirectory})
}

func TestSystemUpdateErrorPreservesComposePathAndMasksSecrets(t *testing.T) {
	message := SystemUpdateErrorMessage(errors.New(
		`stat Docker Compose configuration failed: lstat /opt/NexusTok/docker-compose.yml: no such file or directory; SQL_DSN=dsn-placeholder Authorization=Bearer auth-placeholder upstream URL https://provider.example/v1`,
	))

	assert.Contains(t, message, "/opt/NexusTok/docker-compose.yml")
	assert.NotContains(t, message, "dsn-placeholder")
	assert.NotContains(t, message, "auth-placeholder")
	assert.NotContains(t, message, "https://provider.example")
	assert.Contains(t, message, "SQL_DSN=***")
	assert.Contains(t, message, "Authorization=***")
}

func TestDockerHelperCleanupRemovesContainerWithoutDatabase(t *testing.T) {
	fake := newFakeDockerEngine()
	helper := &dockerInspectContainer{
		ID:     "helper-id",
		Name:   "/nexustok-update-helper-test",
		Config: dockerContainerConfig{Image: "c1cadabob/nexustok:v0.2.7"},
		State:  dockerContainerState{Running: true, Status: "running"},
	}
	fake.addContainer(helper)

	originalFactory := systemUpdateDockerClientFactory
	t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }
	t.Setenv(systemUpdateHelperEnvContainerName, "nexustok-update-helper-test")

	CleanupSystemUpdateDockerHelper(context.Background())

	assert.Nil(t, fake.findContainer("nexustok-update-helper-test"))
}

func TestDockerHelperWritesSuccessTerminalState(t *testing.T) {
	setupSystemTaskServiceTestDB(t)
	fake := newFakeDockerEngine()
	current := &dockerInspectContainer{
		ID:   "current-id",
		Name: "/nexustok",
		Config: dockerContainerConfig{
			Image:       "c1cadabob/nexustok:old",
			Healthcheck: &dockerHealthcheck{Test: []string{"CMD-SHELL", "true"}},
		},
		State: dockerContainerState{Running: true, Status: "running"},
	}
	fake.addContainer(current)
	currentID := current.ID
	task, err := model.CreateSystemTask(model.SystemTaskTypeSystemUpdate, nil, nil)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(
		task.ID,
		model.SystemTaskTypeSystemUpdate,
		"helper-runner",
		common.GetTimestamp()+60,
	)
	require.NoError(t, err)
	require.True(t, ok)

	originalFactory := systemUpdateDockerClientFactory
	originalService := defaultSystemUpdateService
	t.Cleanup(func() {
		systemUpdateDockerClientFactory = originalFactory
		defaultSystemUpdateService = originalService
	})
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }
	defaultSystemUpdateService = dockerTestService(fake, &currentID)

	t.Setenv(systemUpdateHelperEnvTaskID, claimed.TaskID)
	t.Setenv(systemUpdateHelperEnvRunnerID, "helper-runner")
	t.Setenv(systemUpdateHelperEnvAction, systemUpdateDockerHelperActionUpdate)
	t.Setenv(systemUpdateHelperEnvMode, systemUpdateDeploymentDockerRun)
	t.Setenv(systemUpdateHelperEnvCurrentContainerID, current.ID)
	t.Setenv(systemUpdateHelperEnvTargetImage, "c1cadabob/nexustok:new")

	require.NoError(t, RunSystemUpdateDockerHelper(context.Background()))

	reloaded, err := model.GetSystemTaskByTaskID(claimed.TaskID)
	require.NoError(t, err)
	require.NotNil(t, reloaded)
	assert.Equal(t, model.SystemTaskStatusSucceeded, reloaded.Status)
	assert.Equal(t, "helper-runner", reloaded.LockedBy)
	assert.Equal(t, "c1cadabob/nexustok:new", fake.findContainer("nexustok").Config.Image)
	assert.True(t, fake.findContainer("nexustok").State.Running)
}

func TestDockerHelperWritesFailureTerminalStateForInvalidAction(t *testing.T) {
	setupSystemTaskServiceTestDB(t)
	task, err := model.CreateSystemTask(model.SystemTaskTypeSystemUpdate, nil, nil)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(
		task.ID,
		model.SystemTaskTypeSystemUpdate,
		"helper-runner",
		common.GetTimestamp()+60,
	)
	require.NoError(t, err)
	require.True(t, ok)

	t.Setenv(systemUpdateHelperEnvTaskID, claimed.TaskID)
	t.Setenv(systemUpdateHelperEnvRunnerID, "helper-runner")
	t.Setenv(systemUpdateHelperEnvAction, "invalid")

	err = RunSystemUpdateDockerHelper(context.Background())

	require.Error(t, err)
	reloaded, queryErr := model.GetSystemTaskByTaskID(claimed.TaskID)
	require.NoError(t, queryErr)
	require.NotNil(t, reloaded)
	assert.Equal(t, model.SystemTaskStatusFailed, reloaded.Status)
	assert.Contains(t, reloaded.Error, "action is invalid")
}

func TestDockerHelperStartupFailureReturnsTaskLeaseToOriginalRunner(t *testing.T) {
	setupSystemTaskServiceTestDB(t)
	fake := newFakeDockerEngine()
	fake.failStartAll = 1
	composeDirectory := t.TempDir()
	composePath := filepath.Join(composeDirectory, "docker-compose.yml")
	require.NoError(t, os.WriteFile(composePath, []byte("services: {}\n"), 0640))
	current := &dockerInspectContainer{
		ID:   "current-id",
		Name: "/nexustok",
		Config: dockerContainerConfig{
			Image: "c1cadabob/nexustok:old",
			Labels: map[string]string{
				"com.docker.compose.project.working_dir":  composeDirectory,
				"com.docker.compose.project.config_files": composePath,
			},
		},
		State: dockerContainerState{Running: true, Status: "running"},
	}
	fake.addContainer(current)
	currentID := current.ID
	task, err := model.CreateSystemTask(model.SystemTaskTypeSystemUpdate, nil, nil)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(
		task.ID,
		model.SystemTaskTypeSystemUpdate,
		"runner-a",
		common.GetTimestamp()+60,
	)
	require.NoError(t, err)
	require.True(t, ok)

	originalFactory := systemUpdateDockerClientFactory
	t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }
	service := dockerTestService(fake, &currentID)

	err = service.startDockerHelper(
		context.Background(),
		claimed,
		"runner-a",
		dockerHelperOptions{
			Action: systemUpdateDockerHelperActionUpdate,
			Mode:   systemUpdateDeploymentDockerCompose,
		},
	)

	require.Error(t, err)
	require.Len(t, fake.createRequests, 1)
	assert.Contains(t, fake.createRequests[0].Request.HostConfig.Mounts, dockerMountSpec{
		Type:   "bind",
		Source: composeDirectory,
		Target: composeDirectory,
	})
	reloaded, queryErr := model.GetSystemTaskByTaskID(claimed.TaskID)
	require.NoError(t, queryErr)
	require.NotNil(t, reloaded)
	assert.Equal(t, model.SystemTaskStatusRunning, reloaded.Status)
	assert.Equal(t, "runner-a", reloaded.LockedBy)
	var lock model.SystemTaskLock
	require.NoError(t, model.DB.Where("task_id = ?", claimed.TaskID).First(&lock).Error)
	assert.Equal(t, "runner-a", lock.LockedBy)
	assert.False(t, fake.hasContainerNamePrefix("nexustok-update-helper-"))
}

func TestDockerHelperStartupStateErrorReturnsTaskLeaseToOriginalRunner(t *testing.T) {
	setupSystemTaskServiceTestDB(t)
	fake := newFakeDockerEngine()
	fake.startStateError = "SESSION_SECRET=hidden-value"
	current := &dockerInspectContainer{
		ID:   "current-id",
		Name: "/nexustok",
		Config: dockerContainerConfig{
			Image: "c1cadabob/nexustok:old",
		},
		State: dockerContainerState{Running: true, Status: "running"},
	}
	fake.addContainer(current)
	currentID := current.ID
	task, err := model.CreateSystemTask(model.SystemTaskTypeSystemUpdate, nil, nil)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(
		task.ID,
		model.SystemTaskTypeSystemUpdate,
		"runner-a",
		common.GetTimestamp()+60,
	)
	require.NoError(t, err)
	require.True(t, ok)

	originalFactory := systemUpdateDockerClientFactory
	t.Cleanup(func() { systemUpdateDockerClientFactory = originalFactory })
	client, closeServer := fake.client(t)
	t.Cleanup(closeServer)
	systemUpdateDockerClientFactory = func(string) *dockerEngineClient { return client }
	service := dockerTestService(fake, &currentID)

	err = service.startDockerHelper(
		context.Background(),
		claimed,
		"runner-a",
		dockerHelperOptions{
			Action: systemUpdateDockerHelperActionUpdate,
			Mode:   systemUpdateDeploymentDockerRun,
		},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "SESSION_SECRET=***")
	assert.NotContains(t, err.Error(), "hidden-value")
	reloaded, queryErr := model.GetSystemTaskByTaskID(claimed.TaskID)
	require.NoError(t, queryErr)
	require.NotNil(t, reloaded)
	assert.Equal(t, model.SystemTaskStatusRunning, reloaded.Status)
	assert.Equal(t, "runner-a", reloaded.LockedBy)
	var lock model.SystemTaskLock
	require.NoError(t, model.DB.Where("task_id = ?", claimed.TaskID).First(&lock).Error)
	assert.Equal(t, "runner-a", lock.LockedBy)
	assert.False(t, fake.hasContainerNamePrefix("nexustok-update-helper-"))
}

func newTestSystemUpdateService(currentVersion string, goos string, goarch string, isContainer bool, release *systemUpdateGitHubRelease) *SystemUpdateService {
	service := NewSystemUpdateService(&fakeSystemUpdateGitHubClient{release: release})
	service.currentVersionFn = func() string { return currentVersion }
	service.goosFn = func() string { return goos }
	service.goarchFn = func() string { return goarch }
	service.isContainerFn = func() bool { return isContainer }
	service.executableFn = func() (string, error) { return "", errors.New("not configured") }
	service.evalSymlinksFn = func(path string) (string, error) { return path, nil }
	service.backupFn = func(context.Context, string) (*SystemUpdateBackup, error) {
		return &SystemUpdateBackup{ID: "binary-backup-test", Status: systemUpdateBackupStatusReady}, nil
	}
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

func fileModePerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}

func backupComponentNames(components []systemUpdateBackupComponent) []string {
	names := make([]string, 0, len(components))
	for _, component := range components {
		names = append(names, component.Name)
	}
	return names
}
