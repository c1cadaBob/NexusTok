// Package model - db_time.go
// 该文件提供了从数据库获取时间戳的辅助函数
//
// 核心功能：
// - GetDBTimestamp：从数据库获取当前 UNIX 时间戳
//
// 数据库兼容性：
// - PostgreSQL：使用 EXTRACT(EPOCH FROM NOW())::bigint
// - SQLite：使用 strftime('%s','now')
// - MySQL：使用 UNIX_TIMESTAMP()
// - 获取失败时回退到应用服务器时间
package model

import (
	"github.com/c1cada/NexusTok/common"
	"gorm.io/gorm"
)

// GetDBTimestamp 从数据库获取当前 UNIX 时间戳
// 优先使用数据库时间以保证时间一致性，失败时回退到应用服务器时间
func GetDBTimestamp() int64 {
	return getDBTimestampTx(DB)
}

// getDBTimestampTx 使用传入的 GORM 连接读取数据库时间。
//
// 事务内必须复用同一个 tx，否则 SQLite 单连接测试环境和生产低连接池场景下，
// 在事务持有连接时再通过全局 DB 发起查询会等待新连接，造成自锁。
func getDBTimestampTx(tx *gorm.DB) int64 {
	if tx == nil {
		tx = DB
	}
	var ts int64
	var err error
	switch {
	case common.UsingPostgreSQL:
		err = tx.Raw("SELECT EXTRACT(EPOCH FROM NOW())::bigint").Scan(&ts).Error
	case common.UsingSQLite:
		err = tx.Raw("SELECT strftime('%s','now')").Scan(&ts).Error
	default:
		err = tx.Raw("SELECT UNIX_TIMESTAMP()").Scan(&ts).Error
	}
	if err != nil || ts <= 0 {
		return common.GetTimestamp()
	}
	return ts
}
