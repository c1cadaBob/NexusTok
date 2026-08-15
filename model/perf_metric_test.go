package model

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func setupPerfMetricModelTestDB(t *testing.T) {
	t.Helper()
	originDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PerfMetric{}))
	DB = db
	t.Cleanup(func() {
		DB = originDB
	})
}

func TestUpsertPerfMetricAccumulatesAllMetricsOnSQLite(t *testing.T) {
	setupPerfMetricModelTestDB(t)

	first := &PerfMetric{
		ModelName:      "gpt-test",
		Group:          "default",
		BucketTs:       100,
		RequestCount:   2,
		SuccessCount:   1,
		TotalLatencyMs: 100,
		TtftSumMs:      40,
		TtftCount:      2,
		OutputTokens:   80,
		GenerationMs:   60,
	}
	second := &PerfMetric{
		ModelName:      first.ModelName,
		Group:          first.Group,
		BucketTs:       first.BucketTs,
		RequestCount:   3,
		SuccessCount:   2,
		TotalLatencyMs: 150,
		TtftSumMs:      70,
		TtftCount:      3,
		OutputTokens:   120,
		GenerationMs:   90,
	}

	require.NoError(t, UpsertPerfMetric(first))
	require.NoError(t, UpsertPerfMetric(second))

	var got PerfMetric
	require.NoError(t, DB.Where("model_name = ? AND `group` = ? AND bucket_ts = ?", first.ModelName, first.Group, first.BucketTs).First(&got).Error)
	require.Equal(t, int64(5), got.RequestCount)
	require.Equal(t, int64(3), got.SuccessCount)
	require.Equal(t, int64(250), got.TotalLatencyMs)
	require.Equal(t, int64(110), got.TtftSumMs)
	require.Equal(t, int64(5), got.TtftCount)
	require.Equal(t, int64(200), got.OutputTokens)
	require.Equal(t, int64(150), got.GenerationMs)
}

func TestPerfMetricUpsertUsesQualifiedDialectExpressions(t *testing.T) {
	connPool := setupPerfMetricDryRunConnPool(t)
	tests := []struct {
		name       string
		db         *gorm.DB
		contains   string
		notContain string
	}{
		{
			name: "postgres",
			db: mustOpenPerfMetricDryRunDB(t, postgres.New(postgres.Config{
				Conn:                 connPool,
				PreferSimpleProtocol: true,
			})),
			contains:   `EXCLUDED."generation_ms"`,
			notContain: `"generation_ms" + ?`,
		},
		{
			name: "mysql",
			db: mustOpenPerfMetricDryRunDB(t, mysql.New(mysql.Config{
				Conn:                      connPool,
				SkipInitializeWithVersion: true,
			})),
			contains:   "VALUES(`generation_ms`)",
			notContain: "`generation_ms` + ?",
		},
		{
			name:       "sqlite",
			db:         mustOpenPerfMetricDryRunDB(t, sqlite.Open(":memory:")),
			contains:   "excluded.`generation_ms`",
			notContain: `"generation_ms" + ?`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := &PerfMetric{
				ModelName:    "gpt-dry-run",
				Group:        "default",
				BucketTs:     100,
				RequestCount: 1,
				GenerationMs: 2,
			}
			stmt := tt.db.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "model_name"},
					{Name: "group"},
					{Name: "bucket_ts"},
				},
				DoUpdates: clause.Assignments(perfMetricUpsertAssignments(tt.db)),
			}).Create(metric).Statement
			sql := stmt.SQL.String()

			require.Contains(t, sql, tt.contains)
			require.NotContains(t, sql, tt.notContain)
			for _, field := range []string{
				"request_count",
				"success_count",
				"total_latency_ms",
				"ttft_sum_ms",
				"ttft_count",
				"output_tokens",
				"generation_ms",
			} {
				require.Contains(t, strings.ToLower(sql), field)
			}
		})
	}
}

func setupPerfMetricDryRunConnPool(t *testing.T) gorm.ConnPool {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db.ConnPool
}

func mustOpenPerfMetricDryRunDB(t *testing.T, dialector gorm.Dialector) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(dialector, &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)
	return db
}
