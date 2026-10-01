package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func testSubscriptionPreConsumeMigrationNonPostgreSQL(t *testing.T, db *gorm.DB) {
	t.Helper()
	tableName := fmt.Sprintf("subscription_pre_consume_migration_%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = db.Migrator().DropTable(tableName) })

	tableDB := db.Table(tableName)
	require.NoError(t, tableDB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	require.NoError(t, migrateSubscriptionPreConsumeUniqueness(db))
	require.NoError(t, tableDB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
}

func TestMigrateSubscriptionPreConsumeUniquenessSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	testSubscriptionPreConsumeMigrationNonPostgreSQL(t, db)
}

func TestMigrateSubscriptionPreConsumeUniquenessMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	testSubscriptionPreConsumeMigrationNonPostgreSQL(t, db)
}

func requireSubscriptionPreConsumeIndex(t *testing.T, db *gorm.DB, tableName, indexName string) {
	t.Helper()
	var count int64
	require.NoError(t, db.Raw(`
SELECT count(*)
FROM pg_catalog.pg_class AS index_class
JOIN pg_catalog.pg_index AS index_meta
  ON index_meta.indexrelid = index_class.oid
WHERE index_meta.indrelid = to_regclass(?)
  AND index_class.relname = ?
  AND index_meta.indisunique
  AND NOT index_meta.indisprimary`, tableName, indexName).Scan(&count).Error)
	assert.EqualValues(t, 1, count)
}

func requireSubscriptionPreConsumeConstraint(t *testing.T, db *gorm.DB, tableName, constraintName string) {
	t.Helper()
	var count int64
	require.NoError(t, db.Raw(`
SELECT count(*)
FROM pg_catalog.pg_constraint
WHERE conrelid = to_regclass(?)
  AND conname = ?`, tableName, constraintName).Scan(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestMigrateSubscriptionPreConsumeUniquenessPostgreSQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	tests := []struct {
		name               string
		prepareOld         func(*testing.T, *gorm.DB, string)
		expectedError      string
		expectedConstraint string
	}{
		{name: "fresh"},
		{
			name: "legacy_idx_constraint",
			prepareOld: func(t *testing.T, tx *gorm.DB, tableName string) {
				t.Helper()
				require.NoError(t, tx.Migrator().DropIndex(&SubscriptionPreConsumeRecord{}, subscriptionPreConsumeRequestIDIndex))
				require.NoError(t, tx.Exec(
					"ALTER TABLE ? ADD CONSTRAINT ? UNIQUE (?)",
					clause.Table{Name: tableName},
					clause.Column{Name: subscriptionPreConsumeRequestIDIndex},
					clause.Column{Name: subscriptionPreConsumeRequestIDColumn},
				).Error)
			},
		},
		{
			name: "legacy_uni_constraint",
			prepareOld: func(t *testing.T, tx *gorm.DB, tableName string) {
				t.Helper()
				require.NoError(t, tx.Migrator().DropIndex(&SubscriptionPreConsumeRecord{}, subscriptionPreConsumeRequestIDIndex))
				require.NoError(t, tx.Exec(
					"ALTER TABLE ? ADD CONSTRAINT ? UNIQUE (?)",
					clause.Table{Name: tableName},
					clause.Column{Name: subscriptionPreConsumeRequestIDConstraint},
					clause.Column{Name: subscriptionPreConsumeRequestIDColumn},
				).Error)
			},
		},
		{
			name: "legacy_postgres_constraint",
			prepareOld: func(t *testing.T, tx *gorm.DB, tableName string) {
				t.Helper()
				require.NoError(t, tx.Migrator().DropIndex(&SubscriptionPreConsumeRecord{}, subscriptionPreConsumeRequestIDIndex))
				require.NoError(t, tx.Exec(
					"ALTER TABLE ? ADD CONSTRAINT ? UNIQUE (?)",
					clause.Table{Name: tableName},
					clause.Column{Name: subscriptionPreConsumeRequestIDPostgresKey},
					clause.Column{Name: subscriptionPreConsumeRequestIDColumn},
				).Error)
			},
		},
		{
			name: "unknown_constraint_rejected",
			prepareOld: func(t *testing.T, tx *gorm.DB, tableName string) {
				t.Helper()
				require.NoError(t, tx.Migrator().DropIndex(&SubscriptionPreConsumeRecord{}, subscriptionPreConsumeRequestIDIndex))
				require.NoError(t, tx.Exec(
					"ALTER TABLE ? ADD CONSTRAINT ? UNIQUE (?)",
					clause.Table{Name: tableName},
					clause.Column{Name: "keep_subscription_request_id_unique"},
					clause.Column{Name: subscriptionPreConsumeRequestIDColumn},
				).Error)
			},
			expectedError:      "未知唯一约束",
			expectedConstraint: "keep_subscription_request_id_unique",
		},
		{
			name: "composite_constraint_rejected",
			prepareOld: func(t *testing.T, tx *gorm.DB, tableName string) {
				t.Helper()
				require.NoError(t, tx.Migrator().DropIndex(&SubscriptionPreConsumeRecord{}, subscriptionPreConsumeRequestIDIndex))
				require.NoError(t, tx.Exec(
					"ALTER TABLE ? ADD CONSTRAINT ? UNIQUE (?, ?)",
					clause.Table{Name: tableName},
					clause.Column{Name: "subscription_request_user_unique"},
					clause.Column{Name: subscriptionPreConsumeRequestIDColumn},
					clause.Column{Name: "user_id"},
				).Error)
			},
			expectedError:      "复合唯一约束",
			expectedConstraint: "subscription_request_user_unique",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := db.Begin()
			require.NoError(t, tx.Error)
			t.Cleanup(func() { _ = tx.Rollback().Error })

			tableName := fmt.Sprintf("subscription_pre_consume_migration_%d", time.Now().UnixNano())
			require.NoError(t, tx.Exec("CREATE SCHEMA ?", clause.Table{Name: tableName}).Error)
			require.NoError(t, tx.Exec("SET LOCAL search_path TO ?", clause.Table{Name: tableName}).Error)
			require.NoError(t, migrateSubscriptionPreConsumeUniqueness(tx))
			require.NoError(t, tx.AutoMigrate(&SubscriptionPreConsumeRecord{}))

			original := SubscriptionPreConsumeRecord{
				RequestId:   "preserved-request",
				PreConsumed: 10,
				Status:      "consumed",
			}
			require.NoError(t, tx.Create(&original).Error)
			if test.prepareOld != nil {
				test.prepareOld(t, tx, "subscription_pre_consume_records")
			}

			if test.expectedError != "" {
				err := migrateSubscriptionPreConsumeUniqueness(tx)
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
				requireSubscriptionPreConsumeConstraint(t, tx, "subscription_pre_consume_records", test.expectedConstraint)
				return
			}

			for range 2 {
				require.NoError(t, migrateSubscriptionPreConsumeUniqueness(tx))
				require.NoError(t, tx.AutoMigrate(&SubscriptionPreConsumeRecord{}))
			}
			requireSubscriptionPreConsumeIndex(t, tx, "subscription_pre_consume_records", subscriptionPreConsumeRequestIDIndex)

			var preserved SubscriptionPreConsumeRecord
			require.NoError(t, tx.First(&preserved, original.Id).Error)
			assert.Equal(t, original.RequestId, preserved.RequestId)
			assert.EqualValues(t, original.PreConsumed, preserved.PreConsumed)

			duplicateError := tx.Transaction(func(duplicateTx *gorm.DB) error {
				return duplicateTx.Create(&SubscriptionPreConsumeRecord{
					RequestId:   original.RequestId,
					PreConsumed: 1,
					Status:      "consumed",
				}).Error
			})
			require.Error(t, duplicateError)
		})
	}
}
