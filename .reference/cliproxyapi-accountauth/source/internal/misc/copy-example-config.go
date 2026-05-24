// 包 misc - copy-example-config.go
// 该文件提供了配置模板文件的复制功能。
package misc

import (
	"io"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

// CopyConfigTemplate 将配置模板文件从源路径复制到目标路径。
// 自动创建目标目录，文件权限为 0600。
//
// 参数：
//   - src: 源文件路径
//   - dst: 目标文件路径
//
// 返回：
//   - error: 复制失败时返回错误
func CopyConfigTemplate(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if errClose := in.Close(); errClose != nil {
			log.WithError(errClose).Warn("failed to close source config file")
		}
	}()

	if err = os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if errClose := out.Close(); errClose != nil {
			log.WithError(errClose).Warn("failed to close destination config file")
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
