// 包 auth - errors.go
// 该文件定义了认证流程中的特定错误类型。
// 包括项目选择错误（GCP 多项目场景）和邮箱必填错误等业务异常。
package auth

import (
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
)

// ProjectSelectionError 表示用户必须从多个 GCP 项目中选择一个。
// 当认证账户关联了多个项目时，调用方需要将项目列表展示给用户进行选择。
type ProjectSelectionError struct {
	Email    string                         // 用户邮箱地址
	Projects []interfaces.GCPProjectProjects // 可选的 GCP 项目列表
}

// Error 返回项目选择错误的描述信息。
func (e *ProjectSelectionError) Error() string {
	if e == nil {
		return "cliproxy auth: project selection required"
	}
	return fmt.Sprintf("cliproxy auth: project selection required for %s", e.Email)
}

// ProjectsDisplay 返回项目列表，供调用方展示给用户进行选择。
//
// 返回:
//   - []interfaces.GCPProjectProjects: 可选的 GCP 项目列表；nil 表示无可用项目
func (e *ProjectSelectionError) ProjectsDisplay() []interfaces.GCPProjectProjects {
	if e == nil {
		return nil
	}
	return e.Projects
}

// EmailRequiredError 表示调用上下文必须提供邮箱或别名信息。
// 当认证流程需要用户标识但调用方未提供时抛出此错误。
type EmailRequiredError struct {
	Prompt string // 提示信息，告知调用方需要提供什么
}

// Error 返回邮箱必填错误的描述信息。
func (e *EmailRequiredError) Error() string {
	if e == nil || e.Prompt == "" {
		return "cliproxy auth: email is required"
	}
	return e.Prompt
}
