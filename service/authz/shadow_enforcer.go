package authz

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	"gorm.io/gorm"
)

var (
	errAuthzShadowEnforcerNotInitialized = errors.New("authz shadow enforcer is not initialized")

	shadowEnforcerMu sync.RWMutex
	shadowEnforcer   *casbin.SyncedEnforcer
)

const shadowModelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && r.obj == p.obj && r.act == p.act && p.eft == "allow"
`

// ShadowPolicyMismatch 描述 Casbin 影子运行时与 NexusTok 当前权限矩阵的差异。
//
// Expected 是当前 Can()/角色策略矩阵会看到的结果，Shadow 是同一条策略通过
// Casbin SyncedEnforcer 得到的结果。该结构只用于观测和测试，不参与真实授权。
type ShadowPolicyMismatch struct {
	RoleKey  string `json:"role_key"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Expected bool   `json:"expected"`
	Shadow   bool   `json:"shadow"`
}

// InitShadowEnforcer 初始化 Casbin 影子运行时。
//
// 影子 enforcer 只加载 casbin_rule，并关闭 AutoSave，确保不会向数据库写入策略。
// 当前真实授权仍由 Can()、持久角色策略快照和用户 override 决定。
func InitShadowEnforcer(db *gorm.DB) error {
	if db == nil {
		return errAuthzDatabaseNotInitialized
	}

	m, err := casbinmodel.NewModelFromString(shadowModelText)
	if err != nil {
		return err
	}
	enforcer, err := casbin.NewSyncedEnforcer(m, newShadowGormAdapter(db))
	if err != nil {
		return err
	}
	enforcer.EnableAutoSave(false)

	shadowEnforcerMu.Lock()
	shadowEnforcer = enforcer
	shadowEnforcerMu.Unlock()
	return nil
}

func currentShadowEnforcer() *casbin.SyncedEnforcer {
	shadowEnforcerMu.RLock()
	defer shadowEnforcerMu.RUnlock()
	return shadowEnforcer
}

// ReloadShadowPolicy 从数据库重新加载 Casbin 影子策略。
//
// 该函数只刷新影子运行时，不影响 persistentPolicySnapshot，也不改变 Can() 结果。
func ReloadShadowPolicy() error {
	enforcer := currentShadowEnforcer()
	if enforcer == nil {
		return errAuthzShadowEnforcerNotInitialized
	}
	return enforcer.LoadPolicy()
}

// StartShadowPolicySync 周期性刷新 Casbin 影子策略。
//
// 多节点环境中，角色策略可能由其它实例写入；影子运行时需要与持久策略快照一样
// 周期 reload。reload 失败只记录日志，不阻断主业务授权。
func StartShadowPolicySync(frequency int) {
	if frequency <= 0 {
		return
	}
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		if err := ReloadShadowPolicy(); err != nil {
			common.SysLog("authz: failed to reload shadow policy: " + err.Error())
		}
	}
}

// ShadowRoleAllows 使用 Casbin 影子运行时检查角色是否拥有指定权限。
//
// 返回值 ok 表示检查是否实际执行：权限未知、角色 key 为空或 shadow enforcer 未初始化
// 时 ok=false。调用方不能把该函数结果用于真实授权，只能用于观测和测试。
func ShadowRoleAllows(roleKey string, permission Permission) (allowed bool, ok bool, err error) {
	if roleKey == "" || !isKnownPermission(permission) {
		return false, false, nil
	}
	enforcer := currentShadowEnforcer()
	if enforcer == nil {
		return false, false, errAuthzShadowEnforcerNotInitialized
	}
	allowed, err = enforcer.Enforce(RoleSubject(roleKey), permission.Resource, permission.Action)
	return allowed, true, err
}

// CompareShadowRolePolicies 比较当前角色权限矩阵与 Casbin 影子运行时。
//
// 比较范围是数据库中已启用、非 superuser 的角色模板。内置 Admin 会按当前
// NexusTok 授权语义计算 expected：如果策略表缺失，会体现静态 fallback，从而暴露
// “直接切到 Casbin runtime 会改变授权结果”的风险。自定义角色只比较其模板策略，
// 不表示这些角色已经可分配给用户。
func CompareShadowRolePolicies() ([]ShadowPolicyMismatch, error) {
	if model.DB == nil {
		return nil, errAuthzDatabaseNotInitialized
	}
	enforcer := currentShadowEnforcer()
	if enforcer == nil {
		return nil, errAuthzShadowEnforcerNotInitialized
	}

	roles, err := shadowComparableRoles()
	if err != nil {
		return nil, err
	}

	mismatches := make([]ShadowPolicyMismatch, 0)
	for _, role := range roles {
		if role.Superuser || !role.Enabled {
			continue
		}
		for _, resource := range registry {
			for _, action := range resource.Actions {
				expected := role.Grants[resource.Resource][action.Action]
				shadowAllowed, enforceErr := enforcer.Enforce(
					RoleSubject(role.Key),
					resource.Resource,
					action.Action,
				)
				if enforceErr != nil {
					return nil, fmt.Errorf("check shadow policy role=%s resource=%s action=%s: %w", role.Key, resource.Resource, action.Action, enforceErr)
				}
				if expected == shadowAllowed {
					continue
				}
				mismatches = append(mismatches, ShadowPolicyMismatch{
					RoleKey:  role.Key,
					Resource: resource.Resource,
					Action:   action.Action,
					Expected: expected,
					Shadow:   shadowAllowed,
				})
			}
		}
	}
	return mismatches, nil
}

func shadowComparableRoles() ([]RolePolicyDescriptor, error) {
	var roles []model.AuthzRole
	if err := model.DB.Order("sort ASC, key ASC").Find(&roles).Error; err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return fallbackBuiltInRolePolicies(), nil
	}

	result := make([]RolePolicyDescriptor, 0, len(roles))
	for _, role := range roles {
		descriptor, err := describePersistentRole(model.DB, role)
		if err != nil {
			return nil, err
		}
		result = append(result, descriptor)
	}
	return result, nil
}
