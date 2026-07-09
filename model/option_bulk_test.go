package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOptionBulkTestDB(t *testing.T) {
	t.Helper()
	originDB := DB
	originOptionMap := common.OptionMap
	originSMTPStartTLSEnabled := common.SMTPStartTLSEnabled
	originSMTPInsecureSkipVerify := common.SMTPInsecureSkipVerify
	originMerchantID := setting.WaffoPancakeMerchantID
	originStoreID := setting.WaffoPancakeStoreID
	originProductID := setting.WaffoPancakeProductID

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))

	DB = db
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		DB = originDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originOptionMap
		common.OptionMapRWMutex.Unlock()
		common.SMTPStartTLSEnabled = originSMTPStartTLSEnabled
		common.SMTPInsecureSkipVerify = originSMTPInsecureSkipVerify
		setting.WaffoPancakeMerchantID = originMerchantID
		setting.WaffoPancakeStoreID = originStoreID
		setting.WaffoPancakeProductID = originProductID
	})
}

func TestUpdateOptionsBulkPersistsAndRefreshesOptionMap(t *testing.T) {
	setupOptionBulkTestDB(t)

	require.NoError(t, UpdateOptionsBulk(map[string]string{
		"WaffoPancakeMerchantID": "merchant-test",
		"WaffoPancakeStoreID":    "store-test",
		"WaffoPancakeProductID":  "product-test",
	}))

	var options []Option
	require.NoError(t, DB.Find(&options).Error)
	require.Len(t, options, 3)

	common.OptionMapRWMutex.RLock()
	require.Equal(t, "merchant-test", common.OptionMap["WaffoPancakeMerchantID"])
	require.Equal(t, "store-test", common.OptionMap["WaffoPancakeStoreID"])
	require.Equal(t, "product-test", common.OptionMap["WaffoPancakeProductID"])
	common.OptionMapRWMutex.RUnlock()

	require.Equal(t, "merchant-test", setting.WaffoPancakeMerchantID)
	require.Equal(t, "store-test", setting.WaffoPancakeStoreID)
	require.Equal(t, "product-test", setting.WaffoPancakeProductID)
}

func TestUpdateOptionsBulkRefreshesSMTPTransportFlags(t *testing.T) {
	setupOptionBulkTestDB(t)
	common.SMTPStartTLSEnabled = false
	common.SMTPInsecureSkipVerify = false

	require.NoError(t, UpdateOptionsBulk(map[string]string{
		"SMTPStartTLSEnabled":    "true",
		"SMTPInsecureSkipVerify": "true",
	}))

	common.OptionMapRWMutex.RLock()
	require.Equal(t, "true", common.OptionMap["SMTPStartTLSEnabled"])
	require.Equal(t, "true", common.OptionMap["SMTPInsecureSkipVerify"])
	common.OptionMapRWMutex.RUnlock()

	require.True(t, common.SMTPStartTLSEnabled)
	require.True(t, common.SMTPInsecureSkipVerify)
}
