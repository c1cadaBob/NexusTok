package perfmetrics

import (
	"testing"
	"time"

	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPerfMetricsTestDB(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	model.DB = db
	resetPerfMetricHotBuckets()
	t.Cleanup(func() {
		resetPerfMetricHotBuckets()
		model.DB = oldDB
	})
}

func resetPerfMetricHotBuckets() {
	hotBuckets.Range(func(key, _ any) bool {
		hotBuckets.Delete(key)
		return true
	})
}

func createPerfMetricSummaryBucket(t *testing.T, metric model.PerfMetric) {
	t.Helper()
	require.NoError(t, model.DB.Create(&metric).Error)
}

func TestQuerySummaryAllReturnsRecentSuccessRatesForActiveGroups(t *testing.T) {
	setupPerfMetricsTestDB(t)
	now := time.Now().Unix()
	bucket1 := bucketStart(now - 3*3600)
	bucket2 := bucketStart(now - 2*3600)
	bucket3 := bucketStart(now - 1*3600)

	createPerfMetricSummaryBucket(t, model.PerfMetric{
		ModelName:      "gpt-active",
		Group:          "default",
		BucketTs:       bucket1,
		RequestCount:   10,
		SuccessCount:   10,
		TotalLatencyMs: 1000,
		OutputTokens:   100,
		GenerationMs:   1000,
	})
	createPerfMetricSummaryBucket(t, model.PerfMetric{
		ModelName:      "gpt-active",
		Group:          "default",
		BucketTs:       bucket2,
		RequestCount:   10,
		SuccessCount:   9,
		TotalLatencyMs: 2000,
		OutputTokens:   90,
		GenerationMs:   1000,
	})
	createPerfMetricSummaryBucket(t, model.PerfMetric{
		ModelName:      "gpt-active",
		Group:          "auto",
		BucketTs:       bucket3,
		RequestCount:   10,
		SuccessCount:   8,
		TotalLatencyMs: 3000,
		OutputTokens:   80,
		GenerationMs:   1000,
	})
	createPerfMetricSummaryBucket(t, model.PerfMetric{
		ModelName:      "gpt-active",
		Group:          "legacy",
		BucketTs:       bucket3,
		RequestCount:   100,
		SuccessCount:   0,
		TotalLatencyMs: 999000,
		OutputTokens:   1,
		GenerationMs:   1,
	})
	createPerfMetricSummaryBucket(t, model.PerfMetric{
		ModelName:      "gpt-legacy-only",
		Group:          "legacy",
		BucketTs:       bucket3,
		RequestCount:   50,
		SuccessCount:   50,
		TotalLatencyMs: 1000,
		OutputTokens:   50,
		GenerationMs:   1000,
	})

	result, err := QuerySummaryAll(24, []string{"default", "auto"})

	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	summary := result.Models[0]
	assert.Equal(t, "gpt-active", summary.ModelName)
	assert.Equal(t, int64(30), summary.RequestCount)
	assert.Equal(t, int64(200), summary.AvgLatencyMs)
	assert.Equal(t, 90.0, summary.SuccessRate)
	assert.Equal(t, 90.0, summary.AvgTps)
	assert.Equal(t, []float64{100, 90, 80}, summary.RecentSuccessRates)
}

func TestQuerySummaryAllKeepsOnlyLatestThreeSuccessRates(t *testing.T) {
	setupPerfMetricsTestDB(t)
	now := time.Now().Unix()
	for i, successes := range []int64{10, 9, 8, 7} {
		createPerfMetricSummaryBucket(t, model.PerfMetric{
			ModelName:      "gpt-window",
			Group:          "default",
			BucketTs:       bucketStart(now - int64(4-i)*3600),
			RequestCount:   10,
			SuccessCount:   successes,
			TotalLatencyMs: 1000,
			OutputTokens:   10,
			GenerationMs:   1000,
		})
	}

	result, err := QuerySummaryAll(24, []string{"default"})

	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	assert.Equal(t, []float64{90, 80, 70}, result.Models[0].RecentSuccessRates)
}

func TestQuerySummaryAllReturnsEmptyWhenAllowedGroupsEmpty(t *testing.T) {
	setupPerfMetricsTestDB(t)
	createPerfMetricSummaryBucket(t, model.PerfMetric{
		ModelName:      "gpt-hidden",
		Group:          "default",
		BucketTs:       bucketStart(time.Now().Unix()),
		RequestCount:   1,
		SuccessCount:   1,
		TotalLatencyMs: 10,
		OutputTokens:   1,
		GenerationMs:   1,
	})

	result, err := QuerySummaryAll(24, []string{})

	require.NoError(t, err)
	assert.Empty(t, result.Models)
}
