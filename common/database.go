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
	DatabaseTypeMySQL      = "mysql"    // MySQL 数据库类型
	DatabaseTypeSQLite     = "sqlite"   // SQLite 数据库类型
	DatabaseTypePostgreSQL = "postgres" // PostgreSQL 数据库类型
)

var UsingSQLite = false     // 是否使用 SQLite 数据库
var UsingPostgreSQL = false // 是否使用 PostgreSQL 数据库
var LogSqlType = DatabaseTypeSQLite // 日志 SQL 类型（默认 SQLite，用于日志记录的数据库类型）
var UsingMySQL = false      // 是否使用 MySQL 数据库
var UsingClickHouse = false // 是否使用 ClickHouse 数据库（用于日志分析）

var SQLitePath = "nexustok.db?_busy_timeout=30000" // SQLite 数据库文件路径（带超时参数）
