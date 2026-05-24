// Package model - passkey.go
// 该文件定义了 WebAuthn/Passkey 认证数据模型及相关操作
//
// 主要结构体：
// - PasskeyCredential：Passkey 凭据存储
//
// 核心功能：
// - Passkey 凭据的注册、查询、删除
// - WebAuthn 协议的用户和凭据实现
// - 支持多种设备和浏览器的 Passkey 认证
package model

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"
)

var (
	ErrPasskeyNotFound         = errors.New("passkey credential not found")
	ErrFriendlyPasskeyNotFound = errors.New("Passkey 验证失败，请重试或联系管理员")
)

// PasskeyCredential Passkey 凭据存储
// 存储 WebAuthn/Passkey 认证所需的凭据信息
type PasskeyCredential struct {
	ID              int            `json:"id" gorm:"primaryKey"`                                   // 凭据 ID
	UserID          int            `json:"user_id" gorm:"uniqueIndex;not null"`                    // 用户 ID
	CredentialID    string         `json:"credential_id" gorm:"type:varchar(512);uniqueIndex;not null"` // 凭据 ID（Base64 编码）
	PublicKey       string         `json:"public_key" gorm:"type:text;not null"`                   // 公钥（Base64 编码）
	AttestationType string         `json:"attestation_type" gorm:"type:varchar(255)"`              // 认证类型
	AAGUID          string         `json:"aaguid" gorm:"type:varchar(512)"`                        // AAGUID（Base64 编码）
	SignCount       uint32         `json:"sign_count" gorm:"default:0"`                            // 签名计数
	CloneWarning    bool           `json:"clone_warning"`                                          // 克隆警告
	UserPresent     bool           `json:"user_present"`                                           // 用户存在标志
	UserVerified    bool           `json:"user_verified"`                                          // 用户验证标志
	BackupEligible  bool           `json:"backup_eligible"`                                        // 可备份标志
	BackupState     bool           `json:"backup_state"`                                           // 备份状态
	Transports      string         `json:"transports" gorm:"type:text"`                            // 传输方式（JSON 数组）
	Attachment      string         `json:"attachment" gorm:"type:varchar(32)"`                     // 附件类型
	LastUsedAt      *time.Time     `json:"last_used_at"`                                           // 最后使用时间
	CreatedAt       time.Time      `json:"created_at"`                                             // 创建时间
	UpdatedAt       time.Time      `json:"updated_at"`                                             // 更新时间
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`                                         // 软删除时间
}

// TransportList 获取传输方式列表
// 将 JSON 格式的传输方式字符串转换为 WebAuthn 协议的传输方式列表
//
// 返回值：
//   - []protocol.AuthenticatorTransport: 传输方式列表
func (p *PasskeyCredential) TransportList() []protocol.AuthenticatorTransport {
	if p == nil || strings.TrimSpace(p.Transports) == "" {
		return nil
	}
	var transports []string
	if err := json.Unmarshal([]byte(p.Transports), &transports); err != nil {
		return nil
	}
	result := make([]protocol.AuthenticatorTransport, 0, len(transports))
	for _, transport := range transports {
		result = append(result, protocol.AuthenticatorTransport(transport))
	}
	return result
}

// SetTransports 设置传输方式
// 将 WebAuthn 协议的传输方式列表转换为 JSON 格式存储
//
// 参数：
//   - list: 传输方式列表
func (p *PasskeyCredential) SetTransports(list []protocol.AuthenticatorTransport) {
	if len(list) == 0 {
		p.Transports = ""
		return
	}
	stringList := make([]string, len(list))
	for i, transport := range list {
		stringList[i] = string(transport)
	}
	encoded, err := json.Marshal(stringList)
	if err != nil {
		return
	}
	p.Transports = string(encoded)
}

// ToWebAuthnCredential 将 PasskeyCredential 转换为 WebAuthn 协议的 Credential
//
// 返回值：
//   - webauthn.Credential: WebAuthn 协议的凭据对象
func (p *PasskeyCredential) ToWebAuthnCredential() webauthn.Credential {
	flags := webauthn.CredentialFlags{
		UserPresent:    p.UserPresent,
		UserVerified:   p.UserVerified,
		BackupEligible: p.BackupEligible,
		BackupState:    p.BackupState,
	}

	credID, _ := base64.StdEncoding.DecodeString(p.CredentialID)
	pubKey, _ := base64.StdEncoding.DecodeString(p.PublicKey)
	aaguid, _ := base64.StdEncoding.DecodeString(p.AAGUID)

	return webauthn.Credential{
		ID:              credID,
		PublicKey:       pubKey,
		AttestationType: p.AttestationType,
		Transport:       p.TransportList(),
		Flags:           flags,
		Authenticator: webauthn.Authenticator{
			AAGUID:       aaguid,
			SignCount:    p.SignCount,
			CloneWarning: p.CloneWarning,
			Attachment:   protocol.AuthenticatorAttachment(p.Attachment),
		},
	}
}

// NewPasskeyCredentialFromWebAuthn 从 WebAuthn 协议的 Credential 创建 PasskeyCredential
//
// 参数：
//   - userID: 用户 ID
//   - credential: WebAuthn 协议的凭据对象
//
// 返回值：
//   - *PasskeyCredential: Passkey 凭据对象，credential 为 nil 时返回 nil
func NewPasskeyCredentialFromWebAuthn(userID int, credential *webauthn.Credential) *PasskeyCredential {
	if credential == nil {
		return nil
	}
	passkey := &PasskeyCredential{
		UserID:          userID,
		CredentialID:    base64.StdEncoding.EncodeToString(credential.ID),
		PublicKey:       base64.StdEncoding.EncodeToString(credential.PublicKey),
		AttestationType: credential.AttestationType,
		AAGUID:          base64.StdEncoding.EncodeToString(credential.Authenticator.AAGUID),
		SignCount:       credential.Authenticator.SignCount,
		CloneWarning:    credential.Authenticator.CloneWarning,
		UserPresent:     credential.Flags.UserPresent,
		UserVerified:    credential.Flags.UserVerified,
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
		Attachment:      string(credential.Authenticator.Attachment),
	}
	passkey.SetTransports(credential.Transport)
	return passkey
}

// ApplyValidatedCredential 应用已验证的凭据信息
// 更新 PasskeyCredential 的所有字段为 WebAuthn 凭据的值
//
// 参数：
//   - credential: WebAuthn 协议的凭据对象
func (p *PasskeyCredential) ApplyValidatedCredential(credential *webauthn.Credential) {
	if credential == nil || p == nil {
		return
	}
	p.CredentialID = base64.StdEncoding.EncodeToString(credential.ID)
	p.PublicKey = base64.StdEncoding.EncodeToString(credential.PublicKey)
	p.AttestationType = credential.AttestationType
	p.AAGUID = base64.StdEncoding.EncodeToString(credential.Authenticator.AAGUID)
	p.SignCount = credential.Authenticator.SignCount
	p.CloneWarning = credential.Authenticator.CloneWarning
	p.UserPresent = credential.Flags.UserPresent
	p.UserVerified = credential.Flags.UserVerified
	p.BackupEligible = credential.Flags.BackupEligible
	p.BackupState = credential.Flags.BackupState
	p.Attachment = string(credential.Authenticator.Attachment)
	p.SetTransports(credential.Transport)
}

// GetPasskeyByUserID 根据用户 ID 获取 Passkey 凭据
//
// 参数：
//   - userID: 用户 ID
//
// 返回值：
//   - *PasskeyCredential: Passkey 凭据对象
//   - error: 查询失败时返回错误（未找到返回 ErrPasskeyNotFound）
func GetPasskeyByUserID(userID int) (*PasskeyCredential, error) {
	if userID == 0 {
		common.SysLog("GetPasskeyByUserID: empty user ID")
		return nil, ErrFriendlyPasskeyNotFound
	}
	var credential PasskeyCredential
	if err := DB.Where("user_id = ?", userID).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 未找到记录是正常情况（用户未绑定），返回 ErrPasskeyNotFound 而不记录日志
			return nil, ErrPasskeyNotFound
		}
		// 只有真正的数据库错误才记录日志
		common.SysLog(fmt.Sprintf("GetPasskeyByUserID: database error for user %d: %v", userID, err))
		return nil, ErrFriendlyPasskeyNotFound
	}
	return &credential, nil
}

// GetPasskeyByCredentialID 根据凭据 ID 获取 Passkey 凭据
//
// 参数：
//   - credentialID: 凭据 ID（字节数组）
//
// 返回值：
//   - *PasskeyCredential: Passkey 凭据对象
//   - error: 查询失败时返回错误
func GetPasskeyByCredentialID(credentialID []byte) (*PasskeyCredential, error) {
	if len(credentialID) == 0 {
		common.SysLog("GetPasskeyByCredentialID: empty credential ID")
		return nil, ErrFriendlyPasskeyNotFound
	}

	credIDStr := base64.StdEncoding.EncodeToString(credentialID)
	var credential PasskeyCredential
	if err := DB.Where("credential_id = ?", credIDStr).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysLog(fmt.Sprintf("GetPasskeyByCredentialID: passkey not found for credential ID length %d", len(credentialID)))
			return nil, ErrFriendlyPasskeyNotFound
		}
		common.SysLog(fmt.Sprintf("GetPasskeyByCredentialID: database error for credential ID: %v", err))
		return nil, ErrFriendlyPasskeyNotFound
	}

	return &credential, nil
}

// UpsertPasskeyCredential 创建或更新 Passkey 凭据
// 使用事务保证原子性：先删除用户已有凭据，再创建新凭据
//
// 参数：
//   - credential: Passkey 凭据对象
//
// 返回值：
//   - error: 操作失败时返回错误
func UpsertPasskeyCredential(credential *PasskeyCredential) error {
	if credential == nil {
		common.SysLog("UpsertPasskeyCredential: nil credential provided")
		return fmt.Errorf("Passkey 保存失败，请重试")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		// 使用Unscoped()进行硬删除，避免唯一索引冲突
		if err := tx.Unscoped().Where("user_id = ?", credential.UserID).Delete(&PasskeyCredential{}).Error; err != nil {
			common.SysLog(fmt.Sprintf("UpsertPasskeyCredential: failed to delete existing credential for user %d: %v", credential.UserID, err))
			return fmt.Errorf("Passkey 保存失败，请重试")
		}
		if err := tx.Create(credential).Error; err != nil {
			common.SysLog(fmt.Sprintf("UpsertPasskeyCredential: failed to create credential for user %d: %v", credential.UserID, err))
			return fmt.Errorf("Passkey 保存失败，请重试")
		}
		return nil
	})
}

// DeletePasskeyByUserID 根据用户 ID 删除 Passkey 凭据
// 使用硬删除（Unscoped）避免唯一索引冲突
//
// 参数：
//   - userID: 用户 ID
//
// 返回值：
//   - error: 删除失败时返回错误
func DeletePasskeyByUserID(userID int) error {
	if userID == 0 {
		common.SysLog("DeletePasskeyByUserID: empty user ID")
		return fmt.Errorf("删除失败，请重试")
	}
	// 使用Unscoped()进行硬删除，避免唯一索引冲突
	if err := DB.Unscoped().Where("user_id = ?", userID).Delete(&PasskeyCredential{}).Error; err != nil {
		common.SysLog(fmt.Sprintf("DeletePasskeyByUserID: failed to delete passkey for user %d: %v", userID, err))
		return fmt.Errorf("删除失败，请重试")
	}
	return nil
}
