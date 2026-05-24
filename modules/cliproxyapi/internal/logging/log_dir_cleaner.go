// logging - log_dir_cleaner.go
// 本文件实现了日志目录大小限制的自动清理机制。
// 定期检查日志目录总大小，当超过限制时按修改时间从旧到新删除日志文件。
package logging

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// logDirCleanerInterval 是日志目录清理器的检查间隔。
const logDirCleanerInterval = time.Minute

// logDirCleanerCancel 是用于停止清理器的取消函数。
var logDirCleanerCancel context.CancelFunc

// configureLogDirCleanerLocked 配置并启动日志目录清理器。
// 当日志目录总大小超过 maxTotalSizeMB 时，自动删除最旧的日志文件。
// protectedPath 指定的文件不会被删除。
func configureLogDirCleanerLocked(logDir string, maxTotalSizeMB int, protectedPath string) {
	stopLogDirCleanerLocked()

	if maxTotalSizeMB <= 0 {
		return
	}

	maxBytes := int64(maxTotalSizeMB) * 1024 * 1024
	if maxBytes <= 0 {
		return
	}

	dir := strings.TrimSpace(logDir)
	if dir == "" {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	logDirCleanerCancel = cancel
	go runLogDirCleaner(ctx, filepath.Clean(dir), maxBytes, strings.TrimSpace(protectedPath))
}

func stopLogDirCleanerLocked() {
	if logDirCleanerCancel == nil {
		return
	}
	logDirCleanerCancel()
	logDirCleanerCancel = nil
}

func runLogDirCleaner(ctx context.Context, logDir string, maxBytes int64, protectedPath string) {
	ticker := time.NewTicker(logDirCleanerInterval)
	defer ticker.Stop()

	cleanOnce := func() {
		deleted, errClean := enforceLogDirSizeLimit(logDir, maxBytes, protectedPath)
		if errClean != nil {
			log.WithError(errClean).Warn("logging: failed to enforce log directory size limit")
			return
		}
		if deleted > 0 {
			log.Debugf("logging: removed %d old log file(s) to enforce log directory size limit", deleted)
		}
	}

	cleanOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanOnce()
		}
	}
}

func enforceLogDirSizeLimit(logDir string, maxBytes int64, protectedPath string) (int, error) {
	if maxBytes <= 0 {
		return 0, nil
	}

	dir := strings.TrimSpace(logDir)
	if dir == "" {
		return 0, nil
	}
	dir = filepath.Clean(dir)

	entries, errRead := os.ReadDir(dir)
	if errRead != nil {
		if os.IsNotExist(errRead) {
			return 0, nil
		}
		return 0, errRead
	}

	protected := strings.TrimSpace(protectedPath)
	if protected != "" {
		protected = filepath.Clean(protected)
	}

	type logFile struct {
		path    string
		size    int64
		modTime time.Time
	}

	var (
		files []logFile
		total int64
	)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isLogFileName(name) {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(dir, name)
		files = append(files, logFile{
			path:    path,
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		total += info.Size()
	}

	if total <= maxBytes {
		return 0, nil
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	deleted := 0
	for _, file := range files {
		if total <= maxBytes {
			break
		}
		if protected != "" && filepath.Clean(file.path) == protected {
			continue
		}
		if errRemove := os.Remove(file.path); errRemove != nil {
			log.WithError(errRemove).Warnf("logging: failed to remove old log file: %s", filepath.Base(file.path))
			continue
		}
		total -= file.size
		deleted++
	}

	return deleted, nil
}

func isLogFileName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.HasSuffix(lower, ".log") || strings.HasSuffix(lower, ".log.gz")
}
