// auth - errors.go
// 该文件定义了认证过程中的错误类型。
// 包括项目选择错误（GCP 多项目场景）和邮箱必填错误。

package auth

import (
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
)

// ProjectSelectionError 表示用户需要选择特定的 GCP 项目 ID。
// 当 GCP 账户关联多个项目时，需要用户明确指定使用哪个项目。
type ProjectSelectionError struct {
	// Email 用户的邮箱地址
	Email    string
	// Projects 可选的 GCP 项目列表
	Projects []interfaces.GCPProjectProjects
}

// Error 实现 error 接口，返回项目选择错误的描述信息。
func (e *ProjectSelectionError) Error() string {
	if e == nil {
		return "cliproxy auth: project selection required"
	}
	return fmt.Sprintf("cliproxy auth: project selection required for %s", e.Email)
}

// ProjectsDisplay 返回项目列表，供调用方展示给用户选择。
func (e *ProjectSelectionError) ProjectsDisplay() []interfaces.GCPProjectProjects {
	if e == nil {
		return nil
	}
	return e.Projects
}

// EmailRequiredError 表示调用上下文必须提供邮箱或别名。
type EmailRequiredError struct {
	// Prompt 提示信息
	Prompt string
}

// Error 实现 error 接口，返回邮箱必填错误的描述信息。
func (e *EmailRequiredError) Error() string {
	if e == nil || e.Prompt == "" {
		return "cliproxy auth: email is required"
	}
	return e.Prompt
}
