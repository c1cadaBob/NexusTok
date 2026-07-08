package model

import (
	"github.com/c1cada/NexusTok/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// lockForUpdate 让紧随其后的查询在支持的主数据库上生成 SELECT ... FOR UPDATE。
//
// GORM v2 不再识别 GORM v1 的旧 query_option 注入方式，因此支付回调、
// 订阅预消费、兑换码等事务热点必须通过 clause.Locking 显式声明行锁。
// SQLite 不支持 FOR UPDATE 语法；这里跳过锁子句，继续依赖 SQLite 单写者事务模型处理写冲突。
func lockForUpdate(tx *gorm.DB) *gorm.DB {
	if common.UsingSQLite {
		return tx
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}
