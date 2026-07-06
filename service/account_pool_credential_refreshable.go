package service

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
)

// poolAccountCredentialAllowsRefresh 判断账号凭据是否允许走自动刷新。
//
// 原生 OAuth 登录得到的账号通常没有显式 refreshable 标记，但凭据中包含 refresh_token，
// 因此默认保持允许刷新；Sub2api 等外部来源可能只导出当前 access_token，并明确写入
// credential_mode=access_token_only 或 refreshable=false，这类账号可以短期调用上游，
// 但不能被检测任务或自动刷新任务当作可刷新 OAuth 凭据处理。
func poolAccountCredentialAllowsRefresh(account *model.PoolAccount) bool {
	if account == nil || account.AuthType != model.AccountPoolAuthTypeOfficialOAuth {
		return false
	}
	for _, raw := range []string{account.CredentialAttrs, account.CredentialMetadata} {
		if credentialJSONModeIsAccessTokenOnly(raw) {
			return false
		}
		if refreshable, ok := credentialJSONRefreshable(raw); ok && !refreshable {
			return false
		}
	}
	return true
}

// credentialJSONModeIsAccessTokenOnly 判断凭据元数据是否明确声明为只读 access token 模式。
// 该模式代表账号没有可用 refresh_token，检测和后台任务不能尝试刷新。
func credentialJSONModeIsAccessTokenOnly(raw string) bool {
	value, ok := credentialJSONString(raw, "credential_mode", "credentialMode")
	return ok && strings.EqualFold(value, "access_token_only")
}

// credentialJSONRefreshable 从凭据元数据中读取 refreshable 标记。
// 返回值第二项表示字段是否存在且能被识别，避免把缺失字段误判为不可刷新。
func credentialJSONRefreshable(raw string) (bool, bool) {
	value, ok := credentialJSONValue(raw, "refreshable")
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.EqualFold(trimmed, "true") {
			return true, true
		}
		if strings.EqualFold(trimmed, "false") {
			return false, true
		}
	default:
		text := strings.TrimSpace(fmt.Sprintf("%v", typed))
		if strings.EqualFold(text, "true") {
			return true, true
		}
		if strings.EqualFold(text, "false") {
			return false, true
		}
	}
	return false, false
}

// credentialJSONString 从凭据元数据中读取字符串字段。
// 外部导入工具有时会把布尔或数字写入 JSON，因此这里允许转成字符串后再判断。
func credentialJSONString(raw string, keys ...string) (string, bool) {
	value, ok := credentialJSONValue(raw, keys...)
	if !ok {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed, trimmed != ""
	default:
		text := strings.TrimSpace(fmt.Sprintf("%v", typed))
		return text, text != "" && text != "<nil>"
	}
}

// credentialJSONValue 以宽松方式读取 JSON 对象里的任意一个候选字段。
// 元数据解析失败时直接视为字段不存在，保证异常元数据不会阻断账号池主流程。
func credentialJSONValue(raw string, keys ...string) (any, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	payload := map[string]any{}
	if err := common.UnmarshalJsonStr(raw, &payload); err != nil {
		return nil, false
	}
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			return value, true
		}
	}
	return nil, false
}
