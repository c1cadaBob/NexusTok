package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelMetaSearchTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	common.UsingSQLite = true

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Model{}, &Vendor{}))

	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
	})

	return db
}

func TestSearchModelsWithVendorQualifiesModelColumns(t *testing.T) {
	db := setupModelMetaSearchTestDB(t)

	openaiVendor := Vendor{Name: "OpenAI", Description: "official OpenAI models", Status: 1}
	anthropicVendor := Vendor{Name: "Anthropic", Description: "OpenAI-compatible routes", Status: 1}
	require.NoError(t, db.Create(&openaiVendor).Error)
	require.NoError(t, db.Create(&anthropicVendor).Error)

	require.NoError(t, db.Create(&Model{
		ModelName:   "gpt-5.6-terra",
		Description: "GPT-5.6 Terra is an AI model from OpenAI.",
		Tags:        "Reasoning,OpenAI",
		VendorID:    openaiVendor.Id,
		Status:      1,
	}).Error)
	require.NoError(t, db.Create(&Model{
		ModelName:   "claude-sonnet",
		Description: "OpenAI-compatible Claude route.",
		Tags:        "Claude",
		VendorID:    anthropicVendor.Id,
		Status:      1,
	}).Error)

	models, total, err := SearchModels("OpenAI", "OpenAI", 0, 10)

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, models, 1)
	require.Equal(t, "gpt-5.6-terra", models[0].ModelName)
}
