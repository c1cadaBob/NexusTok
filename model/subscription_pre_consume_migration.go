package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	subscriptionPreConsumeRequestIDIndex       = "idx_subscription_pre_consume_records_request_id"
	subscriptionPreConsumeRequestIDConstraint  = "uni_subscription_pre_consume_records_request_id"
	subscriptionPreConsumeRequestIDPostgresKey = "subscription_pre_consume_records_request_id_key"
	subscriptionPreConsumeRequestIDColumn      = "request_id"
)

type subscriptionPreConsumeUniqueConstraint struct {
	Name        string
	Definition  string
	ColumnCount int
	Deferrable  bool
	Validated   bool
}

type subscriptionPreConsumeUniqueIndex struct {
	Name             string
	Definition       string
	ColumnCount      int
	Partial          bool
	Expression       bool
	ConstraintBacked bool
}

func inspectSubscriptionPreConsumeUniqueness(db *gorm.DB, tableName string) ([]subscriptionPreConsumeUniqueConstraint, []subscriptionPreConsumeUniqueIndex, error) {
	var constraints []subscriptionPreConsumeUniqueConstraint
	if err := db.Raw(`
SELECT constraint_meta.conname AS name,
       pg_get_constraintdef(constraint_meta.oid) AS definition,
       cardinality(constraint_meta.conkey) AS column_count,
       constraint_meta.condeferrable AS deferrable,
       constraint_meta.convalidated AS validated
FROM pg_catalog.pg_constraint AS constraint_meta
WHERE constraint_meta.conrelid = to_regclass(?)
  AND constraint_meta.contype = 'u'
  AND EXISTS (
      SELECT 1
      FROM unnest(constraint_meta.conkey) AS constraint_key(attnum)
      JOIN pg_catalog.pg_attribute AS attribute_meta
        ON attribute_meta.attrelid = constraint_meta.conrelid
       AND attribute_meta.attnum = constraint_key.attnum
      WHERE attribute_meta.attname = ?
  )
ORDER BY constraint_meta.conname`, tableName, subscriptionPreConsumeRequestIDColumn).Scan(&constraints).Error; err != nil {
		return nil, nil, fmt.Errorf("检查订阅预消费唯一约束失败: %w", err)
	}

	var indexes []subscriptionPreConsumeUniqueIndex
	if err := db.Raw(`
SELECT index_class.relname AS name,
       pg_get_indexdef(index_class.oid) AS definition,
       index_meta.indnatts AS column_count,
       index_meta.indpred IS NOT NULL AS partial,
       index_meta.indexprs IS NOT NULL AS expression,
       EXISTS (
           SELECT 1
           FROM pg_catalog.pg_constraint AS constraint_meta
           WHERE constraint_meta.conindid = index_class.oid
       ) AS constraint_backed
FROM pg_catalog.pg_index AS index_meta
JOIN pg_catalog.pg_class AS index_class
  ON index_class.oid = index_meta.indexrelid
WHERE index_meta.indrelid = to_regclass(?)
  AND index_meta.indisunique
  AND NOT index_meta.indisprimary
  AND (
      EXISTS (
          SELECT 1
          FROM unnest(index_meta.indkey) AS index_key(attnum)
          JOIN pg_catalog.pg_attribute AS attribute_meta
            ON attribute_meta.attrelid = index_meta.indrelid
           AND attribute_meta.attnum = index_key.attnum
          WHERE attribute_meta.attname = ?
      )
      OR (
          index_meta.indexprs IS NOT NULL
          AND lower(pg_get_indexdef(index_class.oid)) LIKE lower(?)
      )
  )
ORDER BY index_class.relname`, tableName, subscriptionPreConsumeRequestIDColumn, "%"+subscriptionPreConsumeRequestIDColumn+"%").Scan(&indexes).Error; err != nil {
		return nil, nil, fmt.Errorf("检查订阅预消费唯一索引失败: %w", err)
	}
	return constraints, indexes, nil
}

func isKnownSubscriptionPreConsumeUniquenessName(name string) bool {
	switch name {
	case subscriptionPreConsumeRequestIDIndex,
		subscriptionPreConsumeRequestIDConstraint,
		subscriptionPreConsumeRequestIDPostgresKey:
		return true
	default:
		return false
	}
}

func validateSubscriptionPreConsumeUniqueness(
	constraints []subscriptionPreConsumeUniqueConstraint,
	indexes []subscriptionPreConsumeUniqueIndex,
) error {
	for _, constraint := range constraints {
		if constraint.ColumnCount != 1 {
			return fmt.Errorf("订阅预消费 request_id 存在复合唯一约束 %q，无法自动迁移", constraint.Name)
		}
		if !isKnownSubscriptionPreConsumeUniquenessName(constraint.Name) {
			return fmt.Errorf("订阅预消费 request_id 存在未知唯一约束 %q，未修改数据库", constraint.Name)
		}
		if constraint.Deferrable || !constraint.Validated ||
			strings.Contains(strings.ToUpper(constraint.Definition), "NULLS NOT DISTINCT") {
			return fmt.Errorf("订阅预消费 request_id 唯一约束 %q 的定义不受支持，未修改数据库", constraint.Name)
		}
	}
	for _, index := range indexes {
		if index.ConstraintBacked {
			continue
		}
		if index.ColumnCount != 1 || index.Partial || index.Expression {
			return fmt.Errorf("订阅预消费 request_id 存在复合、部分或表达式唯一索引 %q，无法自动迁移", index.Name)
		}
		if !isKnownSubscriptionPreConsumeUniquenessName(index.Name) {
			return fmt.Errorf("订阅预消费 request_id 存在未知唯一索引 %q，未修改数据库", index.Name)
		}
	}
	return nil
}

// migrateSubscriptionPreConsumeUniqueness 将旧版 PostgreSQL 唯一约束转换为独立唯一索引。
func migrateSubscriptionPreConsumeUniqueness(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("迁移订阅预消费唯一性失败: database is nil")
	}
	if db.Dialector.Name() != "postgres" {
		return nil
	}

	statement := &gorm.Statement{DB: db}
	if err := statement.Parse(&SubscriptionPreConsumeRecord{}); err != nil {
		return fmt.Errorf("解析订阅预消费表结构失败: %w", err)
	}
	tableName := statement.Schema.Table
	constraints, indexes, err := inspectSubscriptionPreConsumeUniqueness(db, tableName)
	if err != nil {
		return err
	}
	if len(constraints) == 0 && len(indexes) == 0 {
		return nil
	}
	if err := validateSubscriptionPreConsumeUniqueness(constraints, indexes); err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		migrator := tx.Migrator()
		if !migrator.HasTable(&SubscriptionPreConsumeRecord{}) {
			return nil
		}
		if err := tx.Exec(
			"LOCK TABLE ? IN ACCESS EXCLUSIVE MODE",
			clause.Table{Name: tableName},
		).Error; err != nil {
			return fmt.Errorf("锁定订阅预消费表失败: %w", err)
		}

		constraints, indexes, err := inspectSubscriptionPreConsumeUniqueness(tx, tableName)
		if err != nil {
			return err
		}
		if len(constraints) == 0 && len(indexes) == 0 {
			return nil
		}
		if err := validateSubscriptionPreConsumeUniqueness(constraints, indexes); err != nil {
			return err
		}

		for _, constraint := range constraints {
			if err := migrator.DropConstraint(&SubscriptionPreConsumeRecord{}, constraint.Name); err != nil {
				return fmt.Errorf("删除订阅预消费历史唯一约束 %q 失败: %w", constraint.Name, err)
			}
		}
		for _, index := range indexes {
			if index.ConstraintBacked || index.Name == subscriptionPreConsumeRequestIDIndex {
				continue
			}
			if err := migrator.DropIndex(&SubscriptionPreConsumeRecord{}, index.Name); err != nil {
				return fmt.Errorf("删除订阅预消费历史唯一索引 %q 失败: %w", index.Name, err)
			}
		}

		_, indexes, err = inspectSubscriptionPreConsumeUniqueness(tx, tableName)
		if err != nil {
			return err
		}
		targetExists := false
		for _, index := range indexes {
			if index.Name == subscriptionPreConsumeRequestIDIndex &&
				!index.ConstraintBacked &&
				index.ColumnCount == 1 &&
				!index.Partial &&
				!index.Expression {
				targetExists = true
				break
			}
		}
		if !targetExists {
			if err := migrator.CreateIndex(&SubscriptionPreConsumeRecord{}, subscriptionPreConsumeRequestIDIndex); err != nil {
				return fmt.Errorf("创建订阅预消费唯一索引失败: %w", err)
			}
		}
		return nil
	})
}
