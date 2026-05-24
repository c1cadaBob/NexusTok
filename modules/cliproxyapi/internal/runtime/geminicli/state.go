// Package geminicli 管理 Gemini CLI 的共享凭据状态和虚拟凭据。
// 提供多项目 Gemini CLI 登录的 OAuth 元数据管理和凭据解析功能。
package geminicli

import (
	"strings"
	"sync"
)

// SharedCredential 保存多项目 Gemini CLI 登录的规范 OAuth 元数据。
// 支持多个项目共享同一凭据，并提供线程安全的元数据访问。
//
// SharedCredential keeps canonical OAuth metadata for a multi-project Gemini CLI login.
type SharedCredential struct {
	primaryID  string
	email      string
	metadata   map[string]any
	projectIDs []string
	mu         sync.RWMutex
}

// NewSharedCredential 为给定的主条目构建共享凭据容器。
//
// 参数:
//   - primaryID: 主凭据标识符
//   - email: 关联的账户邮箱
//   - metadata: OAuth 元数据
//   - projectIDs: 项目 ID 列表
//
// NewSharedCredential builds a shared credential container for the given primary entry.
func NewSharedCredential(primaryID, email string, metadata map[string]any, projectIDs []string) *SharedCredential {
	return &SharedCredential{
		primaryID:  strings.TrimSpace(primaryID),
		email:      strings.TrimSpace(email),
		metadata:   cloneMap(metadata),
		projectIDs: cloneStrings(projectIDs),
	}
}

// PrimaryID returns the owning credential identifier.
func (s *SharedCredential) PrimaryID() string {
	if s == nil {
		return ""
	}
	return s.primaryID
}

// Email returns the associated account email.
func (s *SharedCredential) Email() string {
	if s == nil {
		return ""
	}
	return s.email
}

// ProjectIDs returns a snapshot of the configured project identifiers.
func (s *SharedCredential) ProjectIDs() []string {
	if s == nil {
		return nil
	}
	return cloneStrings(s.projectIDs)
}

// MetadataSnapshot returns a deep copy of the stored OAuth metadata.
func (s *SharedCredential) MetadataSnapshot() map[string]any {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneMap(s.metadata)
}

// MergeMetadata merges the provided fields into the shared metadata and returns an updated copy.
func (s *SharedCredential) MergeMetadata(values map[string]any) map[string]any {
	if s == nil {
		return nil
	}
	if len(values) == 0 {
		return s.MetadataSnapshot()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata == nil {
		s.metadata = make(map[string]any, len(values))
	}
	for k, v := range values {
		if v == nil {
			delete(s.metadata, k)
			continue
		}
		s.metadata[k] = v
	}
	return cloneMap(s.metadata)
}

// SetProjectIDs updates the stored project identifiers.
func (s *SharedCredential) SetProjectIDs(ids []string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.projectIDs = cloneStrings(ids)
	s.mu.Unlock()
}

// VirtualCredential 跟踪每个项目的虚拟认证条目，复用主凭据。
// 用于在多个项目之间共享同一 OAuth 凭据。
//
// VirtualCredential tracks a per-project virtual auth entry that reuses a primary credential.
type VirtualCredential struct {
	ProjectID string
	Parent    *SharedCredential
}

// NewVirtualCredential 创建绑定到共享父级的虚拟凭据描述符。
//
// 参数:
//   - projectID: 项目 ID
//   - parent: 共享父凭据
//
// NewVirtualCredential creates a virtual credential descriptor bound to the shared parent.
func NewVirtualCredential(projectID string, parent *SharedCredential) *VirtualCredential {
	return &VirtualCredential{ProjectID: strings.TrimSpace(projectID), Parent: parent}
}

// ResolveSharedCredential 返回支持提供的运行时负载的共享凭据。
//
// 参数:
//   - runtime: 运行时负载（可以是 SharedCredential 或 VirtualCredential）
//
// 返回值:
//   - *SharedCredential: 共享凭据实例，如果无法解析则返回 nil
//
// ResolveSharedCredential returns the shared credential backing the provided runtime payload.
func ResolveSharedCredential(runtime any) *SharedCredential {
	switch typed := runtime.(type) {
	case *SharedCredential:
		return typed
	case *VirtualCredential:
		return typed.Parent
	default:
		return nil
	}
}

// IsVirtual 报告运行时负载是否表示虚拟凭据。
//
// 参数:
//   - runtime: 运行时负载
//
// 返回值:
//   - true: 如果是虚拟凭据
//   - false: 如果不是或为 nil
//
// IsVirtual reports whether the runtime payload represents a virtual credential.
func IsVirtual(runtime any) bool {
	if runtime == nil {
		return false
	}
	_, ok := runtime.(*VirtualCredential)
	return ok
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
