// Package common - database.go
// 该文件定义了数据库类型常量和数据库连接相关的全局变量
//
// 系统支持三种数据库：
// - MySQL: 生产环境推荐，支持高并发
// - SQLite: 开发和测试环境，轻量级
// - PostgreSQL: 功能丰富，支持 JSON 查询等高级特性
//
// 这些变量在数据库初始化阶段设置，用于后续的数据库特定逻辑分支
package common

const (
	DatabaseTypeMySQL      = "mysql"      // MySQL 数据库类型
	DatabaseTypeSQLite     = "sqlite"     // SQLite 数据库类型
	DatabaseTypePostgreSQL = "postgres"   // PostgreSQL 数据库类型
	DatabaseTypeClickHouse = "clickhouse" // ClickHouse 日志数据库类型（仅用于独立日志库准备层）
)

var UsingSQLite = false             // 是否使用 SQLite 数据库
var UsingPostgreSQL = false         // 是否使用 PostgreSQL 数据库
var LogSqlType = DatabaseTypeSQLite // 日志 SQL 类型（默认 SQLite，用于日志记录的数据库类型）
var UsingMySQL = false              // 是否使用 MySQL 数据库
var UsingClickHouse = false         // 是否使用 ClickHouse 数据库（用于日志分析）

var SQLitePath = "nexustok.db?_busy_timeout=30000" // SQLite 数据库文件路径（带超时参数）

// LogDatabaseType 返回当前日志数据库类型。
//
// 该函数封装历史全局变量 LogSqlType，便于日志查询、迁移和测试在不直接依赖全局变量
// 细节的情况下判断日志库方言。当前正式支持 SQLite/MySQL/PostgreSQL；ClickHouse 只作为
// 独立日志库准备层标记，真实 driver 接入需单独评审。
func LogDatabaseType() string {
	return LogSqlType
}

// SetLogDatabaseType 设置日志数据库类型，并同步维护 ClickHouse 兼容标记。
//
// 测试和后续日志库初始化会通过该函数切换日志库方言。调用方传入空字符串时回退到 SQLite，
// 避免不完整配置让日志查询进入未知状态。
func SetLogDatabaseType(databaseType string) {
	if databaseType == "" {
		databaseType = DatabaseTypeSQLite
	}
	LogSqlType = databaseType
	UsingClickHouse = databaseType == DatabaseTypeClickHouse
}

// UsingLogDatabase 判断当前日志库是否为指定类型。
func UsingLogDatabase(databaseType string) bool {
	return LogSqlType == databaseType
}
