// Package model - main.go
// 该文件是数据模型层的核心文件
// 负责数据库初始化、连接管理、表结构迁移等
//
// 支持的数据库：
// - SQLite（默认，适合开发和小型部署）
// - MySQL（>= 5.7.8，适合生产环境）
// - PostgreSQL（>= 9.6，适合生产环境）
//
// 数据库兼容性说明：
// - 使用 GORM 抽象层，避免直接使用 SQL
// - 当需要 raw SQL 时，使用 commonGroupCol、commonKeyCol 等变量处理列名差异
// - 布尔值处理：PostgreSQL 使用 true/false，MySQL/SQLite 使用 1/0
package model

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"   // 公共工具包
	"github.com/c1cada/NexusTok/constant" // 常量定义

	"github.com/glebarez/sqlite" // SQLite 驱动
	"gorm.io/driver/clickhouse"  // ClickHouse 驱动（仅用于独立日志库）
	"gorm.io/driver/mysql"       // MySQL 驱动
	"gorm.io/driver/postgres"    // PostgreSQL 驱动
	"gorm.io/gorm"               // GORM ORM
)

// 数据库列名变量（根据数据库类型自动设置）
// PostgreSQL 使用双引号 "group"，MySQL/SQLite 使用反引号 `group`
var commonGroupCol string // group 列名（保留字需要特殊处理）
var commonKeyCol string   // key 列名（保留字需要特殊处理）
var commonTrueVal string  // 布尔真值（PostgreSQL: "true", MySQL/SQLite: "1"）
var commonFalseVal string // 布尔假值（PostgreSQL: "false", MySQL/SQLite: "0"）

// 日志数据库列名变量
var logKeyCol string   // 日志数据库的 key 列名
var logGroupCol string // 日志数据库的 group 列名

// initCol 初始化数据库列名
// 根据数据库类型设置不同的列名格式
// PostgreSQL 使用双引号，MySQL/SQLite 使用反引号
func initCol() {
	// 初始化公共列名
	if common.UsingPostgreSQL {
		// PostgreSQL 使用双引号包裹保留字
		commonGroupCol = `"group"`
		commonKeyCol = `"key"`
		commonTrueVal = "true"
		commonFalseVal = "false"
	} else {
		// MySQL/SQLite 使用反引号包裹保留字
		commonGroupCol = "`group`"
		commonKeyCol = "`key`"
		commonTrueVal = "1"
		commonFalseVal = "0"
	}

	// 初始化日志数据库列名
	if os.Getenv("LOG_SQL_DSN") != "" {
		// 日志数据库与主数据库不同
		switch common.LogSqlType {
		case common.DatabaseTypePostgreSQL:
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		default:
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	} else {
		// LOG_SQL_DSN 为空时，日志数据库与主数据库相同
		if common.UsingPostgreSQL {
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		} else {
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	}
}

// DB 主数据库连接实例
var DB *gorm.DB

// LOG_DB 日志数据库连接实例
// 可以与主数据库相同，也可以是独立的数据库
var LOG_DB *gorm.DB

// createRootAccountIfNeed 创建 Root 账户（如果需要）
// 当系统中没有任何用户时，自动创建默认的 Root 用户
//
// 默认 Root 用户信息：
// - 用户名：root
// - 密码：123456（首次登录后应立即修改）
// - 角色：Root 用户
// - 状态：启用
// - 配额：100000000（1亿）
//
// 返回值：
//   - error: 错误信息，成功返回 nil
func createRootAccountIfNeed() error {
	var user User

	// 检查是否存在用户
	if err := DB.First(&user).Error; err != nil {
		// 没有用户存在，创建 Root 用户
		common.SysLog("no user exists, create a root user for you: username is root, password is 123456")

		// 加密密码
		hashedPassword, err := common.Password2Hash("123456")
		if err != nil {
			return err
		}

		// 创建 Root 用户
		rootUser := User{
			Username:    "root",
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}

		// 插入数据库
		DB.Create(&rootUser)
	}

	return nil
}

// CheckSetup 检查系统设置状态
// 首次运行时初始化系统设置
func CheckSetup() {
	setup := GetSetup()
	if setup == nil {
		// No setup record exists, check if we have a root user
		if RootUserExists() {
			common.SysLog("system is not initialized, but root user exists")
			// Create setup record
			newSetup := Setup{
				Version:       common.Version,
				InitializedAt: time.Now().Unix(),
			}
			err := DB.Create(&newSetup).Error
			if err != nil {
				common.SysLog("failed to create setup record: " + err.Error())
			}
			constant.Setup = true
		} else {
			common.SysLog("system is not initialized and no root user exists")
			constant.Setup = false
		}
	} else {
		// Setup record exists, system is initialized
		common.SysLog("system is already initialized at: " + time.Unix(setup.InitializedAt, 0).String())
		constant.Setup = true
	}
}

// isClickHouseDSN 判断 DSN 是否使用 ClickHouse 常见连接协议。
//
// ClickHouse 通常使用 clickhouse/tcp/http/https 协议；这些协议不属于 NexusTok 主业务库
// 当前支持范围，因此主库遇到这类 DSN 会 fail-fast，日志库则进入独立日志库初始化路径。
func isClickHouseDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "clickhouse://") ||
		strings.HasPrefix(dsn, "tcp://") ||
		strings.HasPrefix(dsn, "http://") ||
		strings.HasPrefix(dsn, "https://")
}

// normalizeClickHouseDSN 规范化 ClickHouse HTTPS DSN。
//
// gorm ClickHouse driver 对 HTTPS 连接需要 `secure=true`。这里在打开连接前自动补齐，
// 让 `https://host:8443/db` 这类运维常见写法也能直接作为独立日志库配置使用。
func normalizeClickHouseDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "https" {
		return dsn
	}
	query := parsed.Query()
	if _, ok := query["secure"]; !ok {
		query.Set("secure", "true")
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

// clickHouseLogTTLDays 返回 ClickHouse 日志保留天数。
//
// 小于 0 的配置视为关闭 TTL，避免错误环境变量生成无意义的删除表达式。
func clickHouseLogTTLDays() int {
	ttlDays := common.GetEnvOrDefault("LOG_SQL_CLICKHOUSE_TTL_DAYS", 0)
	if ttlDays < 0 {
		return 0
	}
	return ttlDays
}

// clickHouseLogTTLExpression 生成 ClickHouse logs 表 TTL 表达式。
//
// 返回空字符串表示不启用 TTL；调用方必须先判断空值，避免拼出不完整 DDL。
func clickHouseLogTTLExpression(ttlDays int) string {
	if ttlDays <= 0 {
		return ""
	}
	return fmt.Sprintf("toDateTime(created_at) + INTERVAL %d DAY DELETE", ttlDays)
}

// clickHouseLogTTLClause 生成可直接拼到 ClickHouse 建表 SQL 后部的 TTL 子句。
func clickHouseLogTTLClause(ttlDays int) string {
	expression := clickHouseLogTTLExpression(ttlDays)
	if expression == "" {
		return ""
	}
	return "\nTTL " + expression
}

// clickHouseLogCreateTableSQL 返回 ClickHouse 日志库使用的 logs 表 DDL。
//
// ClickHouse 只作为独立日志库使用，不能承载主业务库。这里使用 MergeTree 按月份分区，
// 并用 created_at + request_id 做稳定排序：request_id 在写入前会自动补齐，因此不依赖
// 传统自增 id，也能让后台分页、日志清理和后续排障保持确定顺序。
func clickHouseLogCreateTableSQL(ttlDays int) string {
	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS logs (
	id Int64 DEFAULT 0,
	user_id Int32 DEFAULT 0,
	created_at Int64 DEFAULT 0,
	type Int32 DEFAULT 0,
	content String DEFAULT '',
	username String DEFAULT '',
	token_name String DEFAULT '',
	model_name String DEFAULT '',
	quota Int32 DEFAULT 0,
	prompt_tokens Int32 DEFAULT 0,
	completion_tokens Int32 DEFAULT 0,
	use_time Int32 DEFAULT 0,
	is_stream UInt8 DEFAULT 0,
	channel_id Int32 DEFAULT 0,
	token_id Int32 DEFAULT 0,
	`+"`group`"+` String DEFAULT '',
	ip String DEFAULT '',
	request_id String DEFAULT '',
	upstream_request_id String DEFAULT '',
	other String DEFAULT ''
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(toDateTime(created_at))
ORDER BY (created_at, request_id)%s`, clickHouseLogTTLClause(ttlDays))
}

// clickHouseCreateTableHasTTL 判断 ClickHouse 建表 SQL 中是否已经包含 TTL 子句。
func clickHouseCreateTableHasTTL(createTableSQL string) bool {
	upperSQL := strings.ToUpper(createTableSQL)
	return strings.Contains(upperSQL, "\nTTL ") || strings.Contains(upperSQL, " TTL ")
}

// syncClickHouseLogTTL 同步 ClickHouse logs 表的 TTL 配置。
//
// 运维可能会在升级后调整 LOG_SQL_CLICKHOUSE_TTL_DAYS：大于 0 时使用 MODIFY TTL 更新保留期；
// 等于 0 时表示关闭自动删除，如果旧表已经带 TTL，则显式 REMOVE TTL。ClickHouse 的 TTL 是
// 表级元数据，必须用 ALTER TABLE 同步，不能依赖 GORM AutoMigrate。
func syncClickHouseLogTTL(ttlDays int) error {
	expression := clickHouseLogTTLExpression(ttlDays)
	if expression != "" {
		return LOG_DB.Exec("ALTER TABLE logs MODIFY TTL " + expression).Error
	}

	hasTTL, err := clickHouseLogTableHasTTL()
	if err != nil {
		return err
	}
	if !hasTTL {
		return nil
	}
	return LOG_DB.Exec("ALTER TABLE logs REMOVE TTL").Error
}

// clickHouseLogTableHasTTL 查询 ClickHouse 当前建表语句，判断是否已经存在 TTL。
func clickHouseLogTableHasTTL() (bool, error) {
	var createTableSQL string
	if err := LOG_DB.Raw("SHOW CREATE TABLE logs").Scan(&createTableSQL).Error; err != nil {
		return false, err
	}
	return clickHouseCreateTableHasTTL(createTableSQL), nil
}

func chooseDB(envName string, isLog bool) (*gorm.DB, error) {
	defer func() {
		initCol()
	}()
	dsn := os.Getenv(envName)
	if dsn != "" {
		if isClickHouseDSN(dsn) {
			if !isLog {
				return nil, fmt.Errorf("%s does not support ClickHouse; use SQLite, MySQL, or PostgreSQL for the primary database", envName)
			}
			normalized := normalizeClickHouseDSN(dsn)
			common.SysLog("using ClickHouse as log database")
			common.SetLogDatabaseType(common.DatabaseTypeClickHouse)
			return gorm.Open(clickhouse.Open(normalized), &gorm.Config{
				// ClickHouse 写入日志时不需要 GORM prepared statement 缓存；关闭它可以减少
				// 长生命周期多节点部署中的语句缓存占用，也避免部分代理/网关对 prepared
				// statement 兼容性较差的问题。
				PrepareStmt: false,
			})
		}
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			// 使用 PostgreSQL 作为当前数据库。
			common.SysLog("using PostgreSQL as database")
			if !isLog {
				common.UsingPostgreSQL = true
			} else {
				common.SetLogDatabaseType(common.DatabaseTypePostgreSQL)
			}
			return gorm.Open(postgres.New(postgres.Config{
				DSN:                  dsn,
				PreferSimpleProtocol: true, // 禁用隐式 prepared statement，避免部分连接池兼容问题。
			}), &gorm.Config{
				PrepareStmt: true, // 预编译 SQL，提高重复查询性能。
			})
		}
		if strings.HasPrefix(dsn, "local") {
			common.SysLog("SQL_DSN not set, using SQLite as database")
			if !isLog {
				common.UsingSQLite = true
			} else {
				common.SetLogDatabaseType(common.DatabaseTypeSQLite)
			}
			return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
				PrepareStmt: true, // 预编译 SQL，提高重复查询性能。
			})
		}
		// 使用 MySQL 作为当前数据库。
		common.SysLog("using MySQL as database")
		// GORM 读取时间字段需要 parseTime，缺失时自动补齐以降低配置门槛。
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		if !isLog {
			common.UsingMySQL = true
		} else {
			common.SetLogDatabaseType(common.DatabaseTypeMySQL)
		}
		return gorm.Open(mysql.Open(dsn), &gorm.Config{
			PrepareStmt: true, // 预编译 SQL，提高重复查询性能。
		})
	}
	// 未显式配置 DSN 时使用 SQLite，保持开发和单机部署的默认体验。
	common.SysLog("SQL_DSN not set, using SQLite as database")
	common.UsingSQLite = true
	return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
		PrepareStmt: true, // 预编译 SQL，提高重复查询性能。
	})
}

func InitDB() (err error) {
	db, err := chooseDB("SQL_DSN", false)
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		DB = db
		// MySQL 启动时校验字符集/排序规则，确保能够保存中文内容。
		if common.UsingMySQL {
			if err := checkMySQLChineseSupport(DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		if common.UsingMySQL {
			//_, _ = sqlDB.Exec("ALTER TABLE channels MODIFY model_mapping TEXT;") // TODO: delete this line when most users have upgraded
		}
		common.SysLog("database migration started")
		err = migrateDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func InitLogDB() (err error) {
	if os.Getenv("LOG_SQL_DSN") == "" {
		LOG_DB = DB
		switch {
		case common.UsingPostgreSQL:
			common.SetLogDatabaseType(common.DatabaseTypePostgreSQL)
		case common.UsingMySQL:
			common.SetLogDatabaseType(common.DatabaseTypeMySQL)
		default:
			common.SetLogDatabaseType(common.DatabaseTypeSQLite)
		}
		initCol()
		if common.IsMasterNode {
			return migrateSyncedAccountLocalUsedQuota()
		}
		return
	}
	db, err := chooseDB("LOG_SQL_DSN", true)
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		LOG_DB = db
		// 日志库使用 MySQL 时同样校验字符集，确保中文审计日志和错误内容不会乱码。
		if common.LogSqlType == common.DatabaseTypeMySQL {
			if err := checkMySQLChineseSupport(LOG_DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := LOG_DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		common.SysLog("database migration started")
		if err = migrateLOGDB(); err != nil {
			return err
		}
		return migrateSyncedAccountLocalUsedQuota()
	} else {
		common.FatalLog(err)
	}
	return err
}

func migrateDB() error {
	// Migrate price_amount column from float/double to decimal for existing tables
	migrateSubscriptionPlanPriceAmount()
	// Migrate model_limits column from varchar to text for existing tables
	if err := migrateTokenModelLimitsToText(); err != nil {
		return err
	}

	err := DB.AutoMigrate(
		&Channel{},
		&ChannelAccount{},
		&AccountPoolGroup{},
		&PoolAccount{},
		&AccountPoolAuthFile{},
		&PoolAccountUsageLog{},
		&PoolAccountStateLog{},
		&PoolAccountCheckTask{},
		&Token{},
		&User{},
		&AuthzUserOverride{},
		&AuthzRole{},
		&CasbinRule{},
		&PasskeyCredential{},
		&Option{},
		&Redemption{},
		&Ability{},
		&Log{},
		&Midjourney{},
		&TopUp{},
		&QuotaData{},
		&Task{},
		&Model{},
		&Vendor{},
		&PrefillGroup{},
		&Setup{},
		&TwoFA{},
		&TwoFABackupCode{},
		&Checkin{},
		&SubscriptionOrder{},
		&UserSubscription{},
		&SubscriptionPreConsumeRecord{},
		&CustomOAuthProvider{},
		&UserOAuthBinding{},
		&PerfMetric{},
		&SystemInstance{},
		&SystemTask{},
		&SystemTaskLock{},
	)
	if err != nil {
		return err
	}
	if common.UsingSQLite {
		if err := ensureSubscriptionPlanTableSQLite(); err != nil {
			return err
		}
	} else {
		if err := DB.AutoMigrate(&SubscriptionPlan{}); err != nil {
			return err
		}
	}
	if err := migrateLegacyCLIProxyAccountPoolGroups(); err != nil {
		return err
	}
	if err := migrateSyncedAccountChannelTypes(); err != nil {
		return err
	}
	if err := migrateSyncedAccountAccessGroups(); err != nil {
		return err
	}
	if err := migrateSyncedAccountModels(); err != nil {
		return err
	}
	return nil
}

func migrateDBFast() error {

	var wg sync.WaitGroup

	migrations := []struct {
		model interface{}
		name  string
	}{
		{&Channel{}, "Channel"},
		{&ChannelAccount{}, "ChannelAccount"},
		{&AccountPoolGroup{}, "AccountPoolGroup"},
		{&PoolAccount{}, "PoolAccount"},
		{&AccountPoolAuthFile{}, "AccountPoolAuthFile"},
		{&PoolAccountUsageLog{}, "PoolAccountUsageLog"},
		{&PoolAccountStateLog{}, "PoolAccountStateLog"},
		{&PoolAccountCheckTask{}, "PoolAccountCheckTask"},
		{&Token{}, "Token"},
		{&User{}, "User"},
		{&AuthzUserOverride{}, "AuthzUserOverride"},
		{&AuthzRole{}, "AuthzRole"},
		{&CasbinRule{}, "CasbinRule"},
		{&PasskeyCredential{}, "PasskeyCredential"},
		{&Option{}, "Option"},
		{&Redemption{}, "Redemption"},
		{&Ability{}, "Ability"},
		{&Log{}, "Log"},
		{&Midjourney{}, "Midjourney"},
		{&TopUp{}, "TopUp"},
		{&QuotaData{}, "QuotaData"},
		{&Task{}, "Task"},
		{&Model{}, "Model"},
		{&Vendor{}, "Vendor"},
		{&PrefillGroup{}, "PrefillGroup"},
		{&Setup{}, "Setup"},
		{&TwoFA{}, "TwoFA"},
		{&TwoFABackupCode{}, "TwoFABackupCode"},
		{&Checkin{}, "Checkin"},
		{&SubscriptionOrder{}, "SubscriptionOrder"},
		{&UserSubscription{}, "UserSubscription"},
		{&SubscriptionPreConsumeRecord{}, "SubscriptionPreConsumeRecord"},
		{&CustomOAuthProvider{}, "CustomOAuthProvider"},
		{&UserOAuthBinding{}, "UserOAuthBinding"},
		{&PerfMetric{}, "PerfMetric"},
		{&SystemInstance{}, "SystemInstance"},
		{&SystemTask{}, "SystemTask"},
		{&SystemTaskLock{}, "SystemTaskLock"},
	}
	// 动态计算migration数量，确保errChan缓冲区足够大
	errChan := make(chan error, len(migrations))

	for _, m := range migrations {
		wg.Add(1)
		go func(model interface{}, name string) {
			defer wg.Done()
			if err := DB.AutoMigrate(model); err != nil {
				errChan <- fmt.Errorf("failed to migrate %s: %v", name, err)
			}
		}(m.model, m.name)
	}

	// Wait for all migrations to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	if common.UsingSQLite {
		if err := ensureSubscriptionPlanTableSQLite(); err != nil {
			return err
		}
	} else {
		if err := DB.AutoMigrate(&SubscriptionPlan{}); err != nil {
			return err
		}
	}
	if err := ensureAccountPoolAuthFileLinks(); err != nil {
		return err
	}
	if err := migrateLegacyCLIProxyAccountPoolGroups(); err != nil {
		return err
	}
	if err := migrateSyncedAccountChannelTypes(); err != nil {
		return err
	}
	if err := migrateSyncedAccountAccessGroups(); err != nil {
		return err
	}
	if err := migrateSyncedAccountModels(); err != nil {
		return err
	}
	common.SysLog("database migrated")
	return nil
}

func migrateSyncedAccountChannelTypes() error {
	var channels []Channel
	if err := DB.Select("id", "type", "settings").
		Where("settings LIKE ?", "%upstream_account_sync%").
		Find(&channels).Error; err != nil {
		return err
	}
	updated := 0
	for _, channel := range channels {
		nextType := SyncedAccountPlatformChannelType(channel.UpstreamAccountSyncPlatform())
		if nextType == 0 || channel.Type == nextType {
			continue
		}
		if err := DB.Model(&Channel{}).
			Where("id = ?", channel.Id).
			Update("type", nextType).Error; err != nil {
			return err
		}
		updated++
	}
	if updated > 0 {
		common.SysLog(fmt.Sprintf("migrated %d upstream account synced channel type(s)", updated))
	}
	return nil
}

// migrateSyncedAccountAccessGroups 为历史上游同步账号回填 NexusTok 可访问用户组。
//
// 新字段 `access_groups` 承担实际路由权限含义，而同步账号原有 `group` 字段已经被
// 用作“上游密钥分组”。旧数据升级时如果不回填，能力重建会认为这些 key 不允许任何
// 下游用户组，导致同步渠道不可路由。这里只处理带 upstream_account_sync 元数据的渠道，
// 并用渠道当前 group 作为兼容默认值；普通多 Key 渠道仍继续使用原有 group 字段。
func migrateSyncedAccountAccessGroups() error {
	if DB == nil || !DB.Migrator().HasColumn(&ChannelAccount{}, "access_groups") {
		return nil
	}
	var channels []Channel
	if err := DB.Select("id", "group", "settings").
		Where("settings LIKE ?", "%upstream_account_sync%").
		Find(&channels).Error; err != nil {
		return err
	}
	updated := int64(0)
	for _, channel := range channels {
		if !channel.HasUpstreamAccountSyncMetadata() {
			continue
		}
		if syncedAccountMigrationDone(channel.OtherSettings, "access_groups_backfilled") {
			continue
		}
		groups := strings.TrimSpace(channel.Group)
		if groups == "" {
			groups = "default"
		}
		result := DB.Model(&ChannelAccount{}).
			Where("channel_id = ? AND (access_groups IS NULL OR access_groups = '')", channel.Id).
			Update("access_groups", groups)
		if result.Error != nil {
			return result.Error
		}
		updated += result.RowsAffected
		nextSettings, err := markSyncedAccountMigrationDone(channel.Id, channel.OtherSettings, "access_groups_backfilled")
		if err != nil {
			return err
		}
		channel.OtherSettings = nextSettings
	}
	if updated > 0 {
		common.SysLog(fmt.Sprintf("migrated %d upstream synced account access group(s)", updated))
	}
	return nil
}

// migrateSyncedAccountModels 为历史上游同步账号回填模型白名单。
//
// 现在同步账号的空 models 表示“该密钥不参与模型路由”。升级旧数据时，如果不把已有
// 渠道聚合模型回填到账号级 models，历史同步渠道会在下一次能力重建后失去可用模型。
// 该迁移同样只执行一次，管理员后续显式清空某个密钥模型时不会被重启再次回填。
func migrateSyncedAccountModels() error {
	if DB == nil {
		return nil
	}
	var channels []Channel
	if err := DB.Select("id", "models", "settings").
		Where("settings LIKE ?", "%upstream_account_sync%").
		Find(&channels).Error; err != nil {
		return err
	}
	updated := int64(0)
	for _, channel := range channels {
		if !channel.HasUpstreamAccountSyncMetadata() {
			continue
		}
		if syncedAccountMigrationDone(channel.OtherSettings, "models_backfilled") {
			continue
		}
		models := strings.TrimSpace(channel.Models)
		if models != "" {
			result := DB.Model(&ChannelAccount{}).
				Where("channel_id = ? AND (models IS NULL OR models = '')", channel.Id).
				Update("models", models)
			if result.Error != nil {
				return result.Error
			}
			updated += result.RowsAffected
		}
		nextSettings, err := markSyncedAccountMigrationDone(channel.Id, channel.OtherSettings, "models_backfilled")
		if err != nil {
			return err
		}
		channel.OtherSettings = nextSettings
	}
	if updated > 0 {
		common.SysLog(fmt.Sprintf("migrated %d upstream synced account model list(s)", updated))
	}
	return nil
}

type channelQuotaSum struct {
	ChannelID int   `gorm:"column:channel_id"`
	Quota     int64 `gorm:"column:quota"`
}

// migrateSyncedAccountLocalUsedQuota 修复历史同步渠道的本地已用额度口径。
//
// 早期上游账号同步和余额刷新会把上游账号维度的 used_usd 覆盖到 Channel.used_quota，
// 而该字段在 Relay 结算链路中承担“本地经该渠道产生的消费累计”。这里只处理带
// upstream_account_sync 元数据且尚未标记完成的渠道，从消费日志按 channel_id 回算
// 现存 LogTypeConsume 的 quota 总和，并写回主库。查询 LOG_DB 后再更新 DB，避免主库
// 与日志库分离或 ClickHouse 独立日志库时做跨库 join。
func migrateSyncedAccountLocalUsedQuota() error {
	if DB == nil || LOG_DB == nil {
		return nil
	}
	var channels []Channel
	if err := DB.Select("id", "settings").
		Where("settings LIKE ?", "%upstream_account_sync%").
		Find(&channels).Error; err != nil {
		return err
	}
	targets := make([]Channel, 0, len(channels))
	channelIDs := make([]int, 0, len(channels))
	for _, channel := range channels {
		if !channel.HasUpstreamAccountSyncMetadata() {
			continue
		}
		if syncedAccountMigrationDone(channel.OtherSettings, "local_used_quota_rebuilt") {
			continue
		}
		targets = append(targets, channel)
		channelIDs = append(channelIDs, channel.Id)
	}
	if len(targets) == 0 {
		return nil
	}

	quotaByChannel := make(map[int]int64, len(targets))
	const batchSize = 500
	for start := 0; start < len(channelIDs); start += batchSize {
		end := start + batchSize
		if end > len(channelIDs) {
			end = len(channelIDs)
		}
		var rows []channelQuotaSum
		if err := LOG_DB.Table("logs").
			Select("channel_id, COALESCE(sum(quota), 0) as quota").
			Where("type = ? AND channel_id IN ?", LogTypeConsume, channelIDs[start:end]).
			Group("channel_id").
			Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			quotaByChannel[row.ChannelID] = row.Quota
		}
	}

	updated := 0
	for _, channel := range targets {
		localUsedQuota := quotaByChannel[channel.Id]
		if err := DB.Model(&Channel{}).
			Where("id = ?", channel.Id).
			Update("used_quota", localUsedQuota).Error; err != nil {
			return err
		}
		if _, err := markSyncedAccountMigrationDone(channel.Id, channel.OtherSettings, "local_used_quota_rebuilt"); err != nil {
			return err
		}
		updated++
	}
	if updated > 0 {
		common.SysLog(fmt.Sprintf("rebuilt local used_quota for %d upstream synced channel(s)", updated))
	}
	return nil
}

func syncedAccountMigrationDone(settings string, key string) bool {
	var data map[string]any
	if strings.TrimSpace(settings) == "" {
		return false
	}
	if err := common.UnmarshalJsonStr(settings, &data); err != nil {
		return false
	}
	metadata, ok := data["upstream_account_sync"].(map[string]any)
	if !ok {
		return false
	}
	migrations, ok := metadata["migrations"].(map[string]any)
	if !ok {
		return false
	}
	done, _ := migrations[key].(bool)
	return done
}

func markSyncedAccountMigrationDone(channelID int, settings string, key string) (string, error) {
	var data map[string]any
	if strings.TrimSpace(settings) != "" {
		_ = common.UnmarshalJsonStr(settings, &data)
	}
	if data == nil {
		data = map[string]any{}
	}
	metadata, _ := data["upstream_account_sync"].(map[string]any)
	if metadata == nil {
		metadata = map[string]any{}
	}
	migrations, _ := metadata["migrations"].(map[string]any)
	if migrations == nil {
		migrations = map[string]any{}
	}
	migrations[key] = true
	metadata["migrations"] = migrations
	data["upstream_account_sync"] = metadata
	bytes, err := common.Marshal(data)
	if err != nil {
		return settings, err
	}
	nextSettings := string(bytes)
	if err := DB.Model(&Channel{}).Where("id = ?", channelID).Update("settings", nextSettings).Error; err != nil {
		return settings, err
	}
	return nextSettings, nil
}

// ensureAccountPoolAuthFileLinks 回填旧版认证文件与池账号之间的反向来源 ID。
// 早期版本只在 AccountPoolAuthFile.pool_account_id 上保存“主账号”指针，PoolAccount
// 自身不知道来源认证文件。现在一个凭证可以分配到多个账号组，每个组内调度实例都需要
// auth_file_id 来做去重、删除保护和凭证列表聚合；这里仅修复已有主账号，不改变运行统计。
func ensureAccountPoolAuthFileLinks() error {
	if DB == nil || !DB.Migrator().HasColumn(&PoolAccount{}, "auth_file_id") {
		return nil
	}
	var authFiles []AccountPoolAuthFile
	if err := DB.Select("id", "pool_account_id").
		Where("pool_account_id > ?", 0).
		Find(&authFiles).Error; err != nil {
		return err
	}
	for _, authFile := range authFiles {
		if authFile.Id <= 0 || authFile.PoolAccountId <= 0 {
			continue
		}
		if err := DB.Model(&PoolAccount{}).
			Where("id = ? AND auth_file_id = ?", authFile.PoolAccountId, 0).
			Update("auth_file_id", authFile.Id).Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateLOGDB() error {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return migrateClickHouseLogDB()
	}
	var err error
	if err = LOG_DB.AutoMigrate(&Log{}); err != nil {
		return err
	}
	if err = LOG_DB.AutoMigrate(&PoolAccountUsageLog{}); err != nil {
		return err
	}
	if err = LOG_DB.AutoMigrate(&PoolAccountStateLog{}); err != nil {
		return err
	}
	return nil
}

// migrateClickHouseLogDB 初始化 ClickHouse 独立日志库。
//
// ClickHouse 只承载高频消费日志，不迁移账号池使用/状态日志，后两者仍保留在主业务库或
// 普通 LOG_DB 中。这样做可以先把最大写入压力从主库拆出去，同时避免把具有事务语义的
// 管理状态日志放进最终一致的分析库。
func migrateClickHouseLogDB() error {
	ttlDays := clickHouseLogTTLDays()
	if err := LOG_DB.Exec(clickHouseLogCreateTableSQL(ttlDays)).Error; err != nil {
		return err
	}
	return syncClickHouseLogTTL(ttlDays)
}

type sqliteColumnDef struct {
	Name string
	DDL  string
}

func ensureSubscriptionPlanTableSQLite() error {
	if !common.UsingSQLite {
		return nil
	}
	tableName := "subscription_plans"
	if !DB.Migrator().HasTable(tableName) {
		createSQL := `CREATE TABLE ` + "`" + tableName + "`" + ` (
` + "`id`" + ` integer,
` + "`title`" + ` varchar(128) NOT NULL,
` + "`subtitle`" + ` varchar(255) DEFAULT '',
` + "`price_amount`" + ` decimal(10,6) NOT NULL,
` + "`currency`" + ` varchar(8) NOT NULL DEFAULT 'USD',
` + "`duration_unit`" + ` varchar(16) NOT NULL DEFAULT 'month',
` + "`duration_value`" + ` integer NOT NULL DEFAULT 1,
` + "`custom_seconds`" + ` bigint NOT NULL DEFAULT 0,
` + "`enabled`" + ` numeric DEFAULT 1,
` + "`sort_order`" + ` integer DEFAULT 0,
` + "`allow_balance_pay`" + ` numeric DEFAULT 1,
` + "`allow_wallet_overflow`" + ` numeric DEFAULT 1,
` + "`stripe_price_id`" + ` varchar(128) DEFAULT '',
` + "`creem_product_id`" + ` varchar(128) DEFAULT '',
` + "`waffo_pancake_product_id`" + ` varchar(128) DEFAULT '',
` + "`max_purchase_per_user`" + ` integer DEFAULT 0,
` + "`upgrade_group`" + ` varchar(64) DEFAULT '',
` + "`downgrade_group`" + ` varchar(64) DEFAULT '',
` + "`total_amount`" + ` bigint NOT NULL DEFAULT 0,
` + "`quota_reset_period`" + ` varchar(16) DEFAULT 'never',
` + "`quota_reset_custom_seconds`" + ` bigint DEFAULT 0,
` + "`created_at`" + ` bigint,
` + "`updated_at`" + ` bigint,
PRIMARY KEY (` + "`id`" + `)
)`
		return DB.Exec(createSQL).Error
	}
	var cols []struct {
		Name string `gorm:"column:name"`
	}
	if err := DB.Raw("PRAGMA table_info(`" + tableName + "`)").Scan(&cols).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		existing[c.Name] = struct{}{}
	}
	required := []sqliteColumnDef{
		{Name: "title", DDL: "`title` varchar(128) NOT NULL"},
		{Name: "subtitle", DDL: "`subtitle` varchar(255) DEFAULT ''"},
		{Name: "price_amount", DDL: "`price_amount` decimal(10,6) NOT NULL"},
		{Name: "currency", DDL: "`currency` varchar(8) NOT NULL DEFAULT 'USD'"},
		{Name: "duration_unit", DDL: "`duration_unit` varchar(16) NOT NULL DEFAULT 'month'"},
		{Name: "duration_value", DDL: "`duration_value` integer NOT NULL DEFAULT 1"},
		{Name: "custom_seconds", DDL: "`custom_seconds` bigint NOT NULL DEFAULT 0"},
		{Name: "enabled", DDL: "`enabled` numeric DEFAULT 1"},
		{Name: "sort_order", DDL: "`sort_order` integer DEFAULT 0"},
		{Name: "allow_balance_pay", DDL: "`allow_balance_pay` numeric DEFAULT 1"},
		{Name: "allow_wallet_overflow", DDL: "`allow_wallet_overflow` numeric DEFAULT 1"},
		{Name: "stripe_price_id", DDL: "`stripe_price_id` varchar(128) DEFAULT ''"},
		{Name: "creem_product_id", DDL: "`creem_product_id` varchar(128) DEFAULT ''"},
		{Name: "waffo_pancake_product_id", DDL: "`waffo_pancake_product_id` varchar(128) DEFAULT ''"},
		{Name: "max_purchase_per_user", DDL: "`max_purchase_per_user` integer DEFAULT 0"},
		{Name: "upgrade_group", DDL: "`upgrade_group` varchar(64) DEFAULT ''"},
		{Name: "downgrade_group", DDL: "`downgrade_group` varchar(64) DEFAULT ''"},
		{Name: "total_amount", DDL: "`total_amount` bigint NOT NULL DEFAULT 0"},
		{Name: "quota_reset_period", DDL: "`quota_reset_period` varchar(16) DEFAULT 'never'"},
		{Name: "quota_reset_custom_seconds", DDL: "`quota_reset_custom_seconds` bigint DEFAULT 0"},
		{Name: "created_at", DDL: "`created_at` bigint"},
		{Name: "updated_at", DDL: "`updated_at` bigint"},
	}
	for _, col := range required {
		if _, ok := existing[col.Name]; ok {
			continue
		}
		if err := DB.Exec("ALTER TABLE `" + tableName + "` ADD COLUMN " + col.DDL).Error; err != nil {
			return err
		}
	}
	return nil
}

// migrateTokenModelLimitsToText migrates model_limits column from varchar(1024) to text
// This is safe to run multiple times - it checks the column type first
func migrateTokenModelLimitsToText() error {
	// SQLite uses type affinity, so TEXT and VARCHAR are effectively the same — no migration needed
	if common.UsingSQLite {
		return nil
	}

	tableName := "tokens"
	columnName := "model_limits"

	if !DB.Migrator().HasTable(tableName) {
		return nil
	}

	if !DB.Migrator().HasColumn(&Token{}, columnName) {
		return nil
	}

	var alterSQL string
	if common.UsingPostgreSQL {
		var dataType string
		if err := DB.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&dataType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if dataType == "text" {
			return nil
		}
		alterSQL = fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE text`, tableName, columnName)
	} else if common.UsingMySQL {
		var columnType string
		if err := DB.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&columnType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if strings.ToLower(columnType) == "text" {
			return nil
		}
		alterSQL = fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s text", tableName, columnName)
	} else {
		return nil
	}

	if alterSQL != "" {
		if err := DB.Exec(alterSQL).Error; err != nil {
			return fmt.Errorf("failed to migrate %s.%s to text: %w", tableName, columnName, err)
		}
		common.SysLog(fmt.Sprintf("Successfully migrated %s.%s to text", tableName, columnName))
	}
	return nil
}

// migrateSubscriptionPlanPriceAmount migrates price_amount column from float/double to decimal(10,6)
// This is safe to run multiple times - it checks the column type first
func migrateSubscriptionPlanPriceAmount() {
	// SQLite doesn't support ALTER COLUMN, and its type affinity handles this automatically
	// Skip early to avoid GORM parsing the existing table DDL which may cause issues
	if common.UsingSQLite {
		return
	}

	tableName := "subscription_plans"
	columnName := "price_amount"

	// Check if table exists first
	if !DB.Migrator().HasTable(tableName) {
		return
	}

	// Check if column exists
	if !DB.Migrator().HasColumn(&SubscriptionPlan{}, columnName) {
		return
	}

	var alterSQL string
	if common.UsingPostgreSQL {
		// PostgreSQL: Check if already decimal/numeric
		var dataType string
		if err := DB.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&dataType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if dataType == "numeric" {
			return // Already decimal/numeric
		}
		alterSQL = fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE decimal(10,6) USING %s::decimal(10,6)`,
			tableName, columnName, columnName)
	} else if common.UsingMySQL {
		// MySQL: Check if already decimal
		var columnType string
		if err := DB.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&columnType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if strings.HasPrefix(strings.ToLower(columnType), "decimal") {
			return // Already decimal
		}
		alterSQL = fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s decimal(10,6) NOT NULL DEFAULT 0",
			tableName, columnName)
	} else {
		return
	}

	if alterSQL != "" {
		if err := DB.Exec(alterSQL).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to migrate %s.%s to decimal: %v", tableName, columnName, err))
		} else {
			common.SysLog(fmt.Sprintf("Successfully migrated %s.%s to decimal(10,6)", tableName, columnName))
		}
	}
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	return err
}

func CloseDB() error {
	if LOG_DB != DB {
		err := closeDB(LOG_DB)
		if err != nil {
			return err
		}
	}
	return closeDB(DB)
}

// checkMySQLChineseSupport ensures the MySQL connection and current schema
// default charset/collation can store Chinese characters. It allows common
// Chinese-capable charsets (utf8mb4, utf8, gbk, big5, gb18030) and panics otherwise.
func checkMySQLChineseSupport(db *gorm.DB) error {
	// 仅检测：当前库默认字符集/排序规则 + 各表的排序规则（隐含字符集）

	// Read current schema defaults
	var schemaCharset, schemaCollation string
	err := db.Raw("SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = DATABASE()").Row().Scan(&schemaCharset, &schemaCollation)
	if err != nil {
		return fmt.Errorf("读取当前库默认字符集/排序规则失败 / Failed to read schema default charset/collation: %v", err)
	}

	toLower := func(s string) string { return strings.ToLower(s) }
	// Allowed charsets that can store Chinese text
	allowedCharsets := map[string]string{
		"utf8mb4": "utf8mb4_",
		"utf8":    "utf8_",
		"gbk":     "gbk_",
		"big5":    "big5_",
		"gb18030": "gb18030_",
	}
	isChineseCapable := func(cs, cl string) bool {
		csLower := toLower(cs)
		clLower := toLower(cl)
		if prefix, ok := allowedCharsets[csLower]; ok {
			if clLower == "" {
				return true
			}
			return strings.HasPrefix(clLower, prefix)
		}
		// 如果仅提供了排序规则，尝试按排序规则前缀判断
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(clLower, prefix) {
				return true
			}
		}
		return false
	}

	// 1) 当前库默认值必须支持中文
	if !isChineseCapable(schemaCharset, schemaCollation) {
		return fmt.Errorf("当前库默认字符集/排序规则不支持中文：schema(%s/%s)。请将库设置为 utf8mb4/utf8/gbk/big5/gb18030 / Schema default charset/collation is not Chinese-capable: schema(%s/%s). Please set to utf8mb4/utf8/gbk/big5/gb18030",
			schemaCharset, schemaCollation, schemaCharset, schemaCollation)
	}

	// 2) 所有物理表的排序规则（隐含字符集）必须支持中文
	type tableInfo struct {
		Name      string
		Collation *string
	}
	var tables []tableInfo
	if err := db.Raw("SELECT TABLE_NAME, TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'").Scan(&tables).Error; err != nil {
		return fmt.Errorf("读取表排序规则失败 / Failed to read table collations: %v", err)
	}

	var badTables []string
	for _, t := range tables {
		// NULL 或空表示继承库默认设置，已在上面校验库默认，视为通过
		if t.Collation == nil || *t.Collation == "" {
			continue
		}
		cl := *t.Collation
		// 仅凭排序规则判断是否中文可用
		ok := false
		lower := strings.ToLower(cl)
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(lower, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			badTables = append(badTables, fmt.Sprintf("%s(%s)", t.Name, cl))
		}
	}

	if len(badTables) > 0 {
		// 限制输出数量以避免日志过长
		maxShow := 20
		shown := badTables
		if len(shown) > maxShow {
			shown = shown[:maxShow]
		}
		return fmt.Errorf(
			"存在不支持中文的表，请修复其排序规则/字符集。示例（最多展示 %d 项）：%v / Found tables not Chinese-capable. Please fix their collation/charset. Examples (showing up to %d): %v",
			maxShow, shown, maxShow, shown,
		)
	}
	return nil
}

var (
	lastPingTime time.Time
	pingMutex    sync.Mutex
)

func PingDB() error {
	pingMutex.Lock()
	defer pingMutex.Unlock()

	if time.Since(lastPingTime) < time.Second*10 {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Error getting sql.DB from GORM: %v", err)
		return err
	}

	err = sqlDB.Ping()
	if err != nil {
		log.Printf("Error pinging DB: %v", err)
		return err
	}

	lastPingTime = time.Now()
	common.SysLog("Database pinged successfully")
	return nil
}
