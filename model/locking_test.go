package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

// TestLockForUpdateEmitsRowLock 验证模型层统一行锁 helper 的三库行为。
//
// 这里使用 GORM dummy dialector 的 dry-run 模式，只检查 SQL 生成结果：
// MySQL/PostgreSQL 标记下应出现 FOR UPDATE，SQLite 标记下必须跳过该语法。
func TestLockForUpdateEmitsRowLock(t *testing.T) {
	dummyDB, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	require.NoError(t, err)

	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	t.Cleanup(func() {
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
	})

	buildSQL := func() string {
		var rows []Redemption
		return lockForUpdate(dummyDB).Where("id = ?", 1).Find(&rows).Statement.SQL.String()
	}

	common.UsingSQLite = false
	common.UsingMySQL = true
	common.UsingPostgreSQL = false
	assert.Contains(t, buildSQL(), "FOR UPDATE")

	common.UsingSQLite = false
	common.UsingMySQL = false
	common.UsingPostgreSQL = true
	assert.Contains(t, buildSQL(), "FOR UPDATE")

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	assert.NotContains(t, buildSQL(), "FOR UPDATE")
}
