// Package model - errors.go
// 该文件定义了数据模型层的通用错误常量
//
// 错误分类：
// - 数据库错误（ErrDatabase）
// - 用户认证错误（ErrInvalidCredentials、ErrUserEmptyCredentials）
// - Token 认证错误（ErrTokenNotProvided、ErrTokenInvalid）
// - 兑换错误（ErrRedeemFailed）
// - 两步验证错误（ErrTwoFANotEnabled）
package model

import "errors"

// 通用错误
var (
	// ErrDatabase 数据库操作错误
	ErrDatabase = errors.New("database error")
)

// 用户认证错误
var (
	// ErrInvalidCredentials 无效的用户名或密码
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUserEmptyCredentials 用户名或密码为空
	ErrUserEmptyCredentials = errors.New("empty credentials")
	// ErrLoginUserNotFound 表示密码登录时没有找到匹配的用户名或邮箱。
	//
	// 该错误只用于服务端诊断分类，控制器仍然对外返回统一的“用户名或密码错误”
	// 文案，避免通过错误差异枚举账号是否存在。
	ErrLoginUserNotFound = errors.New("login user not found")
	// ErrLoginPasswordMismatch 表示密码登录时 bcrypt 校验失败。
	ErrLoginPasswordMismatch = errors.New("login password mismatch")
	// ErrLoginUserDisabled 表示密码正确但账号已被禁用。
	ErrLoginUserDisabled = errors.New("login user disabled")
	// ErrLoginEmptyPasswordHash 表示目标账号没有可用于密码登录的哈希。
	ErrLoginEmptyPasswordHash = errors.New("login empty password hash")
	// ErrEmailAlreadyTaken 表示邮箱已被其他用户占用。
	// 该错误用于注册、邮箱绑定和 OAuth 写入前的模型层唯一性保护。
	ErrEmailAlreadyTaken = errors.New("email already taken")
	// ErrEmailNotFound 表示按规范化邮箱没有找到可用于当前操作的用户。
	// 密码重置等路径会用它区分“没有唯一目标用户”和数据库异常。
	ErrEmailNotFound = errors.New("email not found")
	// ErrEmailAmbiguous 表示同一个规范化邮箱匹配到多个历史用户。
	// 出现该错误时必须拒绝自动重置密码或绑定，避免误操作多个账号。
	ErrEmailAmbiguous = errors.New("email matches multiple users")
)

// Token 认证错误
var (
	// ErrTokenNotProvided 未提供 Token
	ErrTokenNotProvided = errors.New("token not provided")
	// ErrTokenInvalid 无效的 Token
	ErrTokenInvalid = errors.New("token invalid")
)

// 兑换错误
// ErrRedeemFailed 兑换码兑换失败
var ErrRedeemFailed = errors.New("redeem.failed")

// 两步验证错误
// ErrTwoFANotEnabled 两步验证未启用
var ErrTwoFANotEnabled = errors.New("2fa not enabled")
