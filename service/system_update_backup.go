package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

const (
	systemUpdateBackupDirEnv        = "SYSTEM_UPDATE_BACKUP_DIR"
	systemUpdateDataDirEnv          = "SYSTEM_UPDATE_DATA_DIR"
	systemUpdateBackupRetention     = 5
	systemUpdateBackupStatusReady   = "ready"
	systemUpdateBackupStatusFailed  = "failed"
	systemUpdateRestoreStatusNone   = "not_started"
	systemUpdateRestoreStatusReady  = "restored"
	systemUpdateRestoreStatusFailed = "data_restore_failed"
	systemUpdateBackupRetainMarker  = ".retain"
)

// SystemUpdateBackup 是更新任务可以安全展示的备份摘要，不包含备份路径或连接凭据。
type SystemUpdateBackup struct {
	ID                string `json:"backup_id"`
	Status            string `json:"backup_status"`
	RestoreStatus     string `json:"restore_status,omitempty"`
	DataRestoreFailed bool   `json:"data_restore_failed"`
}

type systemUpdateBackupManifest struct {
	Version    string                        `json:"version"`
	CreatedAt  string                        `json:"created_at"`
	Database   string                        `json:"database"`
	Components []systemUpdateBackupComponent `json:"components"`
}

type systemUpdateBackupComponent struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type systemUpdateBackupArchive struct {
	Root         string
	ArchivePath  string
	Component    string
	ExcludedRoot string
}

// CreateSystemUpdateBackup 创建一次更新前数据备份。失败时不返回可用备份。
func CreateSystemUpdateBackup(ctx context.Context, version string) (*SystemUpdateBackup, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := systemUpdateBackupRoot()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create system update backup directory failed: %w", err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		return nil, fmt.Errorf("secure system update backup directory failed: %w", err)
	}
	id := fmt.Sprintf("%d-%s", time.Now().UnixNano(), strings.ToLower(strings.TrimSpace(common.GetRandomString(8))))
	id = strings.Trim(id, "-")
	if id == "" || strings.ContainsAny(id, `/\`) {
		return nil, errors.New("invalid system update backup id")
	}
	backupRoot := filepath.Join(root, id)
	if err := os.Mkdir(backupRoot, 0700); err != nil {
		return nil, fmt.Errorf("create system update backup failed: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(backupRoot)
	}

	manifest := systemUpdateBackupManifest{
		Version:    strings.TrimSpace(version),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		Database:   string(common.MainDatabaseType()),
		Components: make([]systemUpdateBackupComponent, 0, 3),
	}
	dataDir, err := systemUpdateDataDir()
	if err != nil {
		cleanup()
		return nil, err
	}
	logDir := systemUpdateLogDir()

	archives := []systemUpdateBackupArchive{
		{
			Root:         dataDir,
			ArchivePath:  filepath.Join(backupRoot, "data.tar.gz"),
			Component:    "data",
			ExcludedRoot: root,
		},
		{
			Root:         logDir,
			ArchivePath:  filepath.Join(backupRoot, "logs.tar.gz"),
			Component:    "logs",
			ExcludedRoot: backupRoot,
		},
	}
	for _, archive := range archives {
		if err := createSystemUpdateArchive(ctx, archive.Root, archive.ArchivePath, archive.ExcludedRoot); err != nil {
			cleanup()
			return nil, fmt.Errorf("backup %s failed: %w", archive.Component, err)
		}
		component, err := systemUpdateBackupComponentFromFile(archive.Component, archive.ArchivePath)
		if err != nil {
			cleanup()
			return nil, err
		}
		manifest.Components = append(manifest.Components, component)
	}

	switch common.MainDatabaseType() {
	case common.DatabaseTypeSQLite:
		components, err := backupSQLiteFiles(ctx, backupRoot)
		if err != nil {
			cleanup()
			return nil, err
		}
		manifest.Components = append(manifest.Components, components...)
	case common.DatabaseTypePostgreSQL:
		component, err := dumpPostgreSQL(ctx, backupRoot)
		if err != nil {
			cleanup()
			return nil, err
		}
		manifest.Components = append(manifest.Components, component)
	case common.DatabaseTypeMySQL:
		component, err := dumpMySQL(ctx, backupRoot)
		if err != nil {
			cleanup()
			return nil, err
		}
		manifest.Components = append(manifest.Components, component)
	}

	manifestPath := filepath.Join(backupRoot, "manifest.json")
	data, err := common.Marshal(manifest)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("encode system update backup manifest failed: %w", err)
	}
	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		cleanup()
		return nil, fmt.Errorf("write system update backup manifest failed: %w", err)
	}
	if err := os.Chmod(manifestPath, 0600); err != nil {
		cleanup()
		return nil, fmt.Errorf("secure system update backup manifest failed: %w", err)
	}
	if err := pruneSystemUpdateBackups(root, id); err != nil {
		cleanup()
		return nil, err
	}
	return &SystemUpdateBackup{
		ID:            id,
		Status:        systemUpdateBackupStatusReady,
		RestoreStatus: systemUpdateRestoreStatusNone,
	}, nil
}

// RestoreSystemUpdateBackup 恢复文件数据和主数据库；Redis 始终不参与恢复。
func RestoreSystemUpdateBackup(ctx context.Context, backupID string) (restoreErr error) {
	backupRoot, err := systemUpdateBackupPath(backupID)
	if err != nil {
		return err
	}
	defer func() {
		if restoreErr != nil {
			_ = os.WriteFile(filepath.Join(backupRoot, systemUpdateBackupRetainMarker), nil, 0600)
		}
	}()
	dataDir, err := systemUpdateDataDir()
	if err != nil {
		return err
	}
	logDir := systemUpdateLogDir()
	if err := extractSystemUpdateArchive(ctx, filepath.Join(backupRoot, "data.tar.gz"), dataDir); err != nil {
		return fmt.Errorf("restore data failed: %w", err)
	}
	if err := extractSystemUpdateArchive(ctx, filepath.Join(backupRoot, "logs.tar.gz"), logDir); err != nil {
		return fmt.Errorf("restore logs failed: %w", err)
	}
	switch common.MainDatabaseType() {
	case common.DatabaseTypeSQLite:
		if err := restoreSQLiteFiles(backupRoot); err != nil {
			return err
		}
	case common.DatabaseTypePostgreSQL:
		if err := restorePostgreSQL(ctx, backupRoot); err != nil {
			return err
		}
	case common.DatabaseTypeMySQL:
		if err := restoreMySQL(ctx, backupRoot); err != nil {
			return err
		}
	}
	return nil
}

func systemUpdateBackupRoot() (string, error) {
	if value := strings.TrimSpace(os.Getenv(systemUpdateBackupDirEnv)); value != "" {
		return absoluteSystemUpdatePath(value)
	}
	if common.IsRunningInContainer() {
		return "/data/system-backups", nil
	}
	dataDir, err := systemUpdateDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "system-backups"), nil
}

func systemUpdateDataDir() (string, error) {
	if value := strings.TrimSpace(os.Getenv(systemUpdateDataDirEnv)); value != "" {
		return absoluteSystemUpdatePath(value)
	}
	if common.IsRunningInContainer() {
		return "/data", nil
	}
	return absoluteSystemUpdatePath("data")
}

func systemUpdateLogDir() string {
	if common.LogDir == nil || strings.TrimSpace(*common.LogDir) == "" {
		path, err := filepath.Abs("logs")
		if err != nil {
			return "logs"
		}
		return path
	}
	path, err := filepath.Abs(filepath.Clean(*common.LogDir))
	if err != nil {
		return filepath.Clean(*common.LogDir)
	}
	return path
}

func absoluteSystemUpdatePath(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return "", errors.New("system update path is empty")
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

func systemUpdateBackupPath(id string) (string, error) {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\`) {
		return "", errors.New("invalid system update backup id")
	}
	root, err := systemUpdateBackupRoot()
	if err != nil {
		return "", err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return "", fmt.Errorf("system update backup root unavailable: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", errors.New("system update backup root is not a directory")
	}
	path := filepath.Join(root, id)
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", errors.New("invalid system update backup path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("system update backup not found: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("system update backup is not a directory")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("system update backup must not be a symbolic link")
	}
	return path, nil
}

func createSystemUpdateArchive(ctx context.Context, sourceDir string, archivePath string, excludedRoot string) error {
	sourceDir, err := absoluteSystemUpdatePath(sourceDir)
	if err != nil {
		return err
	}
	archivePath, err = absoluteSystemUpdatePath(archivePath)
	if err != nil {
		return err
	}
	excludedRoot, err = absoluteSystemUpdatePath(excludedRoot)
	if err != nil {
		return err
	}
	info, err := os.Stat(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return createEmptySystemUpdateArchive(archivePath)
		}
		return err
	}
	if !info.IsDir() {
		return errors.New("backup source is not a directory")
	}
	if rootInfo, err := os.Lstat(sourceDir); err != nil || rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("backup source directory must not be a symbolic link")
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(archivePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	walkErr := filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == excludedRoot || strings.HasPrefix(path, excludedRoot+string(os.PathSeparator)) {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup source contains unsupported symbolic link")
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(fileInfo, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, input)
		_ = input.Close()
		return copyErr
	})
	closeTarErr := tarWriter.Close()
	closeGzipErr := gzipWriter.Close()
	if walkErr != nil {
		return walkErr
	}
	if closeTarErr != nil {
		return closeTarErr
	}
	if closeGzipErr != nil {
		return closeGzipErr
	}
	return file.Close()
}

func createEmptySystemUpdateArchive(archivePath string) error {
	if err := os.MkdirAll(filepath.Dir(archivePath), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(archivePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	tarErr := tarWriter.Close()
	gzipErr := gzipWriter.Close()
	closeErr := file.Close()
	if tarErr != nil {
		return tarErr
	}
	if gzipErr != nil {
		return gzipErr
	}
	return closeErr
}

func extractSystemUpdateArchive(ctx context.Context, archivePath string, destination string) error {
	destination, err := absoluteSystemUpdatePath(destination)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(destination); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("restore destination must not be a symbolic link")
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzipReader.Close() }()
	tarReader := tar.NewReader(gzipReader)
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(filepath.FromSlash(header.Name))
		if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			return errors.New("backup archive contains an unsafe path")
		}
		target := filepath.Join(destination, name)
		if err := ensureSystemUpdatePathHasNoSymlink(destination, target); err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(output, tarReader)
			closeErr := output.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("backup archive contains an unsupported entry")
		}
	}
}

func backupSQLiteFiles(ctx context.Context, backupRoot string) ([]systemUpdateBackupComponent, error) {
	path := strings.TrimSpace(strings.SplitN(common.SQLitePath, "?", 2)[0])
	if path == "" || path == ":memory:" {
		return nil, errors.New("SQLite database path is not available")
	}
	if !filepath.IsAbs(path) {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
	}
	components := make([]systemUpdateBackupComponent, 0, 3)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		source := path + suffix
		destination := filepath.Join(backupRoot, "sqlite"+suffix)
		err := copyBackupFile(ctx, source, destination)
		if err != nil {
			if suffix != "" && os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("backup SQLite file failed: %w", err)
		}
		component, err := systemUpdateBackupComponentFromFile("sqlite"+suffix, destination)
		if err != nil {
			return nil, err
		}
		components = append(components, component)
	}
	return components, nil
}

func restoreSQLiteFiles(backupRoot string) error {
	path := strings.TrimSpace(strings.SplitN(common.SQLitePath, "?", 2)[0])
	if path == "" || path == ":memory:" {
		return errors.New("SQLite database path is not available")
	}
	if !filepath.IsAbs(path) {
		path, _ = filepath.Abs(path)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		source := filepath.Join(backupRoot, "sqlite"+suffix)
		err := copyBackupFile(context.Background(), source, path+suffix)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("restore SQLite file failed: %w", err)
		}
		if os.IsNotExist(err) {
			if removeErr := os.Remove(path + suffix); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("remove stale SQLite file failed: %w", removeErr)
			}
		}
	}
	return nil
}

func copyBackupFile(ctx context.Context, source string, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	if info, err := input.Stat(); err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		if err != nil {
			return err
		}
		return errors.New("backup source is not a regular file")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	if err := ensureSystemUpdatePathHasNoSymlink(filepath.Dir(destination), destination); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func dumpPostgreSQL(ctx context.Context, backupRoot string) (systemUpdateBackupComponent, error) {
	dsn := strings.TrimSpace(os.Getenv("SQL_DSN"))
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Hostname() == "" || parsed.Path == "" {
		return systemUpdateBackupComponent{}, errors.New("PostgreSQL backup requires a valid SQL_DSN")
	}
	outputPath := filepath.Join(backupRoot, "postgres.sql")
	args := []string{"--no-owner", "--no-privileges", "--host", parsed.Hostname()}
	if parsed.Port() != "" {
		args = append(args, "--port", parsed.Port())
	}
	if parsed.User != nil {
		args = append(args, "--username", parsed.User.Username())
	}
	args = append(args, "--dbname", strings.TrimPrefix(parsed.Path, "/"))
	if err := runSystemUpdateDatabaseTool(ctx, "pg_dump", args, outputPath, []string{"PGPASSWORD=" + parsedPassword(parsed)}); err != nil {
		return systemUpdateBackupComponent{}, err
	}
	return systemUpdateBackupComponentFromFile("postgresql", outputPath)
}

func dumpMySQL(ctx context.Context, backupRoot string) (systemUpdateBackupComponent, error) {
	dsn := strings.TrimSpace(os.Getenv("SQL_DSN"))
	if dsn == "" {
		return systemUpdateBackupComponent{}, errors.New("MySQL backup requires SQL_DSN")
	}
	config, err := mysqlDriver.ParseDSN(dsn)
	if err != nil || config.DBName == "" {
		return systemUpdateBackupComponent{}, errors.New("MySQL backup requires a valid SQL_DSN")
	}
	args := []string{"--single-transaction", "--skip-lock-tables"}
	if config.Net == "unix" {
		args = append(args, "--socket", config.Addr)
	} else {
		host, port, err := net.SplitHostPort(config.Addr)
		if err != nil {
			host = config.Addr
		} else if port != "" {
			args = append(args, "--port", port)
		}
		args = append(args, "--host", host)
	}
	args = append(args, "--user", config.User, config.DBName)
	outputPath := filepath.Join(backupRoot, "mysql.sql")
	if err := runSystemUpdateDatabaseTool(ctx, "mysqldump", args, outputPath, []string{"MYSQL_PWD=" + config.Passwd}); err != nil {
		return systemUpdateBackupComponent{}, err
	}
	return systemUpdateBackupComponentFromFile("mysql", outputPath)
}

func restorePostgreSQL(ctx context.Context, backupRoot string) error {
	dsn := strings.TrimSpace(os.Getenv("SQL_DSN"))
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Hostname() == "" || parsed.Path == "" {
		return errors.New("PostgreSQL restore requires a valid SQL_DSN")
	}
	args := []string{"--host", parsed.Hostname()}
	if parsed.Port() != "" {
		args = append(args, "--port", parsed.Port())
	}
	if parsed.User != nil {
		args = append(args, "--username", parsed.User.Username())
	}
	args = append(args, "--dbname", strings.TrimPrefix(parsed.Path, "/"))
	return runSystemUpdateDatabaseTool(ctx, "psql", args, filepath.Join(backupRoot, "postgres.sql"), []string{"PGPASSWORD=" + parsedPassword(parsed)})
}

func restoreMySQL(ctx context.Context, backupRoot string) error {
	dsn := strings.TrimSpace(os.Getenv("SQL_DSN"))
	if dsn == "" || !strings.Contains(dsn, "@") {
		return errors.New("MySQL restore requires a valid SQL_DSN")
	}
	config, err := mysqlDriver.ParseDSN(dsn)
	if err != nil || config.DBName == "" {
		return errors.New("MySQL restore database is unavailable")
	}
	args := []string{}
	if config.Net == "unix" {
		args = append(args, "--socket", config.Addr)
	} else {
		host, port, splitErr := net.SplitHostPort(config.Addr)
		if splitErr != nil {
			host = config.Addr
		} else if port != "" {
			args = append(args, "--port", port)
		}
		args = append(args, "--host", host)
	}
	args = append(args, "--user", config.User, config.DBName)
	return runSystemUpdateDatabaseTool(ctx, "mysql", args, filepath.Join(backupRoot, "mysql.sql"), []string{"MYSQL_PWD=" + config.Passwd})
}

func runSystemUpdateDatabaseTool(ctx context.Context, tool string, args []string, outputPath string, extraEnv []string) error {
	if common.IsRunningInContainer() {
		image := systemUpdateDatabaseToolImage(tool)
		if image == "" {
			return errors.New("system update database tool image is not configured")
		}
		client := systemUpdateDockerClient(systemUpdateDockerSocketPath())
		if err := client.pullImage(ctx, image, nil); err != nil {
			return fmt.Errorf("pull Docker database tool image failed: %w", err)
		}
		return runSystemUpdateDatabaseToolContainer(ctx, image, tool, args, outputPath, extraEnv)
	}
	path, err := exec.LookPath(tool)
	if err != nil {
		return fmt.Errorf("%s is not available", tool)
	}
	if systemUpdateDatabaseToolWritesOutput(tool) {
		return runSystemUpdateDatabaseDump(ctx, path, args, outputPath, extraEnv)
	}
	return runSystemUpdateDatabaseCommand(ctx, path, args, outputPath, extraEnv)
}

func systemUpdateDatabaseToolWritesOutput(tool string) bool {
	switch tool {
	case "pg_dump", "mysqldump":
		return true
	default:
		return false
	}
}

func systemUpdateDatabaseToolImage(tool string) string {
	envName := "SYSTEM_UPDATE_DATABASE_TOOL_IMAGE"
	defaultImage := "postgres:16-alpine"
	if tool == "mysqldump" || tool == "mysql" {
		envName = "SYSTEM_UPDATE_MYSQL_TOOL_IMAGE"
		defaultImage = "mysql:8.4"
	}
	return strings.TrimSpace(common.GetEnvOrDefaultString(envName, defaultImage))
}

func runSystemUpdateDatabaseToolContainer(
	ctx context.Context,
	image string,
	tool string,
	args []string,
	outputPath string,
	extraEnv []string,
) error {
	client := systemUpdateDockerClient(systemUpdateDockerSocketPath())
	if err := client.ping(ctx); err != nil {
		return fmt.Errorf("Docker database tool requires Docker Engine: %w", err)
	}
	current, err := client.inspectContainer(ctx, systemUpdateCurrentContainerID())
	if err != nil {
		return fmt.Errorf("inspect current container for database tool failed: %w", err)
	}
	mount, toolPath, err := dockerDatabaseToolPathMapping(current, outputPath)
	if err != nil {
		return err
	}
	inputPath := ""
	if tool == "mysql" {
		inputPath = toolPath
	}

	command := databaseToolContainerCommand(tool, args, toolPath)
	name := "nexustok-system-db-tool-" + dockerUniqueSuffix()
	request := dockerCreateContainerRequest{
		Image: image,
		Cmd:   command,
		Env:   appendDatabaseToolInputEnv(extraEnv, inputPath),
		HostConfig: dockerHostConfig{
			Binds: []string{mount},
		},
		NetworkingConfig: dockerCreateNetworkingConfig{
			EndpointsConfig: dockerEndpointConfigForCreate(current, false),
		},
	}
	if strings.HasPrefix(strings.TrimSpace(strings.ToLower(current.HostConfig.NetworkMode)), "container:") ||
		strings.EqualFold(strings.TrimSpace(current.HostConfig.NetworkMode), "host") {
		request.HostConfig.NetworkMode = current.HostConfig.NetworkMode
		request.NetworkingConfig = dockerCreateNetworkingConfig{}
	}

	containerID, err := client.createContainer(ctx, name, request)
	if err != nil {
		return fmt.Errorf("create Docker database tool container failed: %w", err)
	}
	defer func() {
		recoveryCtx, cancel := systemUpdateDockerRecoveryContext()
		defer cancel()
		_ = client.removeContainer(recoveryCtx, containerID, true)
	}()
	if err := client.startContainer(ctx, containerID); err != nil {
		return fmt.Errorf("start Docker database tool container failed: %w", err)
	}
	statusCode, waitErr := client.waitContainer(ctx, containerID)
	logData, logsErr := client.readContainerLogs(ctx, containerID)
	if waitErr != nil {
		return fmt.Errorf("Docker database tool wait failed: %w", waitErr)
	}
	if logsErr != nil {
		return fmt.Errorf("read Docker database tool logs failed: %w", logsErr)
	}
	if statusCode != 0 {
		return fmt.Errorf(
			"Docker database tool %s failed with exit code %d: %s",
			tool,
			statusCode,
			maskSystemUpdateSensitiveInfo(strings.TrimSpace(string(decodeDockerContainerLogStream(logData)))),
		)
	}
	return nil
}

func appendDatabaseToolInputEnv(env []string, inputPath string) []string {
	if inputPath == "" {
		return env
	}
	return append(env, "MYSQL_INPUT_FILE="+inputPath)
}

func databaseToolContainerCommand(tool string, args []string, path string) []string {
	commandArgs := make([]string, 0, len(args)+1)
	commandArgs = append(commandArgs, args...)
	switch tool {
	case "pg_dump", "pg_restore", "psql":
		commandArgs = append(commandArgs, "--file="+path)
		return append([]string{"sh", "-c", "exec \"$0\" \"$@\"", tool}, commandArgs...)
	case "mysqldump":
		commandArgs = append(commandArgs, "--result-file="+path)
		return append([]string{"sh", "-c", "exec \"$0\" \"$@\"", tool}, commandArgs...)
	case "mysql":
		return append([]string{
			"sh",
			"-c",
			"exec \"$0\" \"$@\" < \"$MYSQL_INPUT_FILE\"",
			tool,
		}, commandArgs...)
	default:
		return append([]string{"sh", "-c", "exec \"$0\" \"$@\"", tool}, commandArgs...)
	}
}

func dockerDatabaseToolPathMapping(current *dockerInspectContainer, path string) (string, string, error) {
	if current == nil {
		return "", "", errors.New("current Docker container is unavailable")
	}
	target := filepath.Clean(path)
	type candidate struct {
		mount dockerMount
	}
	candidates := make([]candidate, 0, len(current.Mounts))
	for _, mount := range current.Mounts {
		destination := filepath.Clean(strings.TrimSpace(mount.Destination))
		if destination == "" {
			continue
		}
		relative, err := filepath.Rel(destination, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			continue
		}
		candidates = append(candidates, candidate{mount: mount})
	}
	if len(candidates) == 0 {
		return "", "", fmt.Errorf("Docker database tool path %q is not mounted into the application container", path)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i].mount.Destination) > len(candidates[j].mount.Destination)
	})
	mount := candidates[0].mount
	relative, _ := filepath.Rel(filepath.Clean(mount.Destination), target)
	if mount.Source == "" && mount.Name == "" {
		return "", "", fmt.Errorf("Docker database tool mount for %q has no source", path)
	}
	source := mount.Source
	if source == "" {
		source = mount.Name
	}
	toolRoot := "/nexustok-mounted"
	toolPath := filepath.Join(toolRoot, relative)
	mode := "rw"
	if !mount.RW {
		mode = "ro"
	}
	return source + ":" + toolRoot + ":" + mode, filepath.ToSlash(toolPath), nil
}

func decodeDockerContainerLogStream(data []byte) []byte {
	if len(data) < 8 {
		return data
	}
	var output strings.Builder
	for len(data) >= 8 {
		size := int(data[4])<<24 | int(data[5])<<16 | int(data[6])<<8 | int(data[7])
		if size < 0 || size > len(data)-8 {
			return data
		}
		output.Write(data[8 : 8+size])
		data = data[8+size:]
	}
	if len(data) > 0 {
		return []byte(output.String() + string(data))
	}
	return []byte(output.String())
}

func runSystemUpdateDatabaseDump(ctx context.Context, name string, args []string, outputPath string, extraEnv []string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = append(os.Environ(), extraEnv...)
	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = output.Close() }()
	command.Stdout = output
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s failed", name)
	}
	return nil
}

func runSystemUpdateDatabaseCommand(ctx context.Context, name string, args []string, inputPath string, extraEnv []string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = append(os.Environ(), extraEnv...)
	input, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	command.Stdin = input
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s failed", name)
	}
	return nil
}

func parsedPassword(parsed *url.URL) string {
	if parsed == nil || parsed.User == nil {
		return ""
	}
	password, _ := parsed.User.Password()
	return password
}

func systemUpdateBackupComponentFromFile(name string, path string) (systemUpdateBackupComponent, error) {
	info, err := os.Stat(path)
	if err != nil {
		return systemUpdateBackupComponent{}, err
	}
	hash, err := sha256File(path)
	if err != nil {
		return systemUpdateBackupComponent{}, err
	}
	return systemUpdateBackupComponent{Name: name, Size: info.Size(), SHA256: hash}, nil
}

func pruneSystemUpdateBackups(root string, currentID string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	type backupEntry struct {
		name string
		time time.Time
	}
	backups := make([]backupEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == currentID {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backupEntry{name: entry.Name(), time: info.ModTime()})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].time.After(backups[j].time) })
	keep := systemUpdateBackupRetention - 1
	if keep < 0 {
		keep = 0
	}
	for i := keep; i < len(backups); i++ {
		if _, err := os.Stat(filepath.Join(root, backups[i].name, systemUpdateBackupRetainMarker)); err == nil {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, backups[i].name)); err != nil {
			return err
		}
	}
	return nil
}

func ensureSystemUpdatePathHasNoSymlink(root string, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return errors.New("backup path escapes its destination")
	}
	current := root
	parts := strings.Split(relative, string(os.PathSeparator))
	for _, part := range parts {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("backup path contains a symbolic link")
		}
	}
	return nil
}
