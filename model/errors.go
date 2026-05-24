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
