// codex_credential_refresh.go
// 本文件实现了 Codex 渠道凭据的刷新逻辑，
// 包括解析 OAuth Key JSON、调用 OAuth Token 刷新接口、
// 更新数据库中的渠道凭据信息，以及缓存重置等功能。

package service

import (
	// 标准库
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
)

// CodexCredentialRefreshOptions 表示凭据刷新操作的选项
type CodexCredentialRefreshOptions struct {
	ResetCaches bool // 刷新成功后是否重置渠道缓存和代理客户端缓存
}

// CodexOAuthKey 表示 Codex 渠道的 OAuth 凭据信息
// 以 JSON 格式存储在渠道的 Key 字段中
type CodexOAuthKey struct {
	IDToken      string `json:"id_token,omitempty"`      // OpenID Connect ID Token
	AccessToken  string `json:"access_token,omitempty"`  // OAuth 访问令牌
	RefreshToken string `json:"refresh_token,omitempty"` // OAuth 刷新令牌，用于获取新的 AccessToken

	AccountID   string `json:"account_id,omitempty"`   // Codex 账户 ID
	LastRefresh string `json:"last_refresh,omitempty"` // 上次刷新时间（RFC3339 格式）
	Email       string `json:"email,omitempty"`        // 账户邮箱
	Type        string `json:"type,omitempty"`         // 凭据类型（如 "codex"）
	Expired     string `json:"expired,omitempty"`      // 访问令牌过期时间（RFC3339 格式）
}

// parseCodexOAuthKey 将渠道 Key 字段的 JSON 字符串解析为 CodexOAuthKey 结构体
// 参数:
//   - raw: 渠道 Key 字段的原始 JSON 字符串
// 返回值:
//   - *CodexOAuthKey: 解析后的 OAuth 凭据结构体
//   - error: JSON 为空或解析失败时返回错误
func parseCodexOAuthKey(raw string) (*CodexOAuthKey, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("codex channel: empty oauth key")
	}
	var key CodexOAuthKey
	if err := common.Unmarshal([]byte(raw), &key); err != nil {
		return nil, errors.New("codex channel: invalid oauth key json")
	}
	return &key, nil
}

// RefreshCodexChannelCredential 刷新指定 Codex 渠道的 OAuth 凭据
// 执行流程：
// 1. 从数据库获取渠道信息并验证类型
// 2. 解析渠道 Key 中的 OAuth 凭据
// 3. 使用 refresh_token 调用 OAuth Token 刷新接口
// 4. 更新凭据信息（AccessToken、RefreshToken、过期时间等）
// 5. 从 JWT 中提取账户 ID 和邮箱（如果缺失）
// 6. 将更新后的凭据写回数据库
// 7. 可选：重置渠道缓存和代理客户端缓存
// 参数:
//   - ctx: 上下文，用于控制请求超时和取消
//   - channelID: 渠道 ID
//   - opts: 刷新选项（如是否重置缓存）
// 返回值:
//   - *CodexOAuthKey: 更新后的 OAuth 凭据
//   - *model.Channel: 渠道信息
//   - error: 刷新失败时返回错误
func RefreshCodexChannelCredential(ctx context.Context, channelID int, opts CodexCredentialRefreshOptions) (*CodexOAuthKey, *model.Channel, error) {
	ch, err := model.GetChannelById(channelID, true)
	if err != nil {
		return nil, nil, err
	}
	if ch == nil {
		return nil, nil, fmt.Errorf("channel not found")
	}
	if ch.Type != constant.ChannelTypeCodex {
		return nil, nil, fmt.Errorf("channel type is not Codex")
	}

	oauthKey, err := parseCodexOAuthKey(strings.TrimSpace(ch.Key))
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(oauthKey.RefreshToken) == "" {
		return nil, nil, fmt.Errorf("codex channel: refresh_token is required to refresh credential")
	}

	// 设置 10 秒超时，防止刷新操作阻塞过久
	refreshCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 调用 OAuth Token 刷新接口获取新的令牌
	res, err := RefreshCodexOAuthTokenWithProxy(refreshCtx, oauthKey.RefreshToken, ch.GetSetting().Proxy)
	if err != nil {
		return nil, nil, err
	}

	// 更新 OAuth 凭据字段
	oauthKey.AccessToken = res.AccessToken
	oauthKey.RefreshToken = res.RefreshToken
	oauthKey.LastRefresh = time.Now().Format(time.RFC3339)
	oauthKey.Expired = res.ExpiresAt.Format(time.RFC3339)
	if strings.TrimSpace(oauthKey.Type) == "" {
		oauthKey.Type = "codex" // 默认凭据类型
	}

	// 如果账户 ID 缺失，尝试从 JWT 中提取
	if strings.TrimSpace(oauthKey.AccountID) == "" {
		if accountID, ok := ExtractCodexAccountIDFromJWT(oauthKey.AccessToken); ok {
			oauthKey.AccountID = accountID
		}
	}
	// 如果邮箱缺失，尝试从 JWT 中提取
	if strings.TrimSpace(oauthKey.Email) == "" {
		if email, ok := ExtractEmailFromJWT(oauthKey.AccessToken); ok {
			oauthKey.Email = email
		}
	}

	// 将更新后的 OAuth Key 序列化为 JSON 并写回数据库
	encoded, err := common.Marshal(oauthKey)
	if err != nil {
		return nil, nil, err
	}

	// 更新渠道的 Key 字段
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("key", string(encoded)).Error; err != nil {
		return nil, nil, err
	}

	// 根据选项决定是否重置缓存
	if opts.ResetCaches {
		model.InitChannelCache()  // 重置渠道缓存
		ResetProxyClientCache()   // 重置代理客户端缓存
	}

	return oauthKey, ch, nil
}
