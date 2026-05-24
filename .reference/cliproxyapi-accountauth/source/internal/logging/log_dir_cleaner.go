// 包 logging - log_dir_cleaner.go
// 该文件提供了日志目录大小限制的自动清理功能。
// 后台定期检查日志目录总大小，超过限制时自动删除最旧的日志文件。
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

// logDirCleanerInterval 定义日志目录清理器的检查间隔。
const logDirCleanerInterval = time.Minute

// logDirCleanerCancel 是用于停止后台清理器的取消函数。
var logDirCleanerCancel context.CancelFunc

// configureLogDirCleanerLocked 配置并启动日志目录清理器。
// 调用前必须持有 writerMu 锁。当 maxTotalSizeMB <= 0 时禁用清理。
//
// 参数：
//   - logDir: 日志目录路径
//   - maxTotalSizeMB: 日志目录最大总大小（MB），0 或负数表示不限制
//   - protectedPath: 受保护的日志文件路径（不会被清理删除）
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

// stopLogDirCleanerLocked 停止正在运行的日志目录清理器。
// 调用前必须持有 writerMu 锁。
func stopLogDirCleanerLocked() {
	if logDirCleanerCancel == nil {
		return
	}
	logDirCleanerCancel()
	logDirCleanerCancel = nil
}

// runLogDirCleaner 是日志目录清理器的主循环。
// 启动时立即执行一次清理，之后按固定间隔定期检查。
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

// enforceLogDirSizeLimit 执行一次日志目录大小限制检查。
// 按修改时间从旧到新排序，依次删除最旧的日志文件直到总大小在限制范围内。
// 受保护的文件不会被删除。
//
// 参数：
//   - logDir: 日志目录路径
//   - maxBytes: 最大总字节数
//   - protectedPath: 受保护的文件路径
//
// 返回：
//   - int: 删除的文件数量
//   - error: 操作失败时返回错误
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

// isLogFileName 检查文件名是否为日志文件（以 .log 或 .log.gz 结尾）。
func isLogFileName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.HasSuffix(lower, ".log") || strings.HasSuffix(lower, ".log.gz")
}
