package service

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAccountPoolAuthFileUpdateTest(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.AccountPoolGroup{}, &model.PoolAccount{}, &model.AccountPoolAuthFile{}))
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})
}

func createAuthFileUpdateGroup(t *testing.T, name string) *model.AccountPoolGroup {
	t.Helper()
	group := &model.AccountPoolGroup{
		Name:     name,
		Platform: "codex",
		AuthType: model.AccountPoolAuthTypeAPIKey,
		Source:   model.AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
		Strategy: model.AccountPoolStrategyRoundRobin,
	}
	require.NoError(t, model.DB.Create(group).Error)
	return group
}

func buildAuthFileUpdateContent(t *testing.T, name string, apiKey string) string {
	t.Helper()
	data, err := common.Marshal(map[string]interface{}{
		"name":           name,
		"provider":       "codex",
		"api_key":        apiKey,
		"account_groups": []string{"default"},
		"models":         "gpt-4o",
	})
	require.NoError(t, err)
	return string(data)
}

func detachImportedAuthFileAccount(t *testing.T, authFileID int, accountID int) {
	t.Helper()
	require.NoError(t, model.DB.Where("id = ?", accountID).Delete(&model.PoolAccount{}).Error)
	require.NoError(t, model.DB.Model(&model.AccountPoolAuthFile{}).
		Where("id = ?", authFileID).
		Updates(map[string]interface{}{
			"pool_account_id": 0,
			"status":          common.ChannelStatusManuallyDisabled,
		}).Error)
}

func TestUpdateAccountPoolAuthFileRecreatesMissingLinkedAccount(t *testing.T) {
	setupAccountPoolAuthFileUpdateTest(t)
	group := createAuthFileUpdateGroup(t, "auth-file-recreate")
	content := buildAuthFileUpdateContent(t, "auth-file-recreate-old", "sk-old-secret")
	imported, err := ImportAccountPoolAuthFile(AccountPoolAuthFileImportOptions{
		Content:     content,
		PoolGroupID: group.Id,
	})
	require.NoError(t, err)
	require.NotNil(t, imported.AuthFile)
	require.NotNil(t, imported.Account)
	oldAccountID := imported.Account.Id
	detachImportedAuthFileAccount(t, imported.AuthFile.Id, oldAccountID)

	newContent := buildAuthFileUpdateContent(t, "auth-file-recreate-new", "sk-new-secret")
	updated, err := UpdateAccountPoolAuthFile(imported.AuthFile.Id, AccountPoolAuthFileUpdateOptions{
		Content: &newContent,
	})

	require.NoError(t, err)
	require.NotNil(t, updated.AuthFile)
	require.NotNil(t, updated.Account)
	require.NotZero(t, updated.AuthFile.PoolAccountId)
	require.Equal(t, updated.AuthFile.PoolAccountId, updated.Account.Id)
	require.Equal(t, group.Id, updated.Account.PoolGroupId)
	require.Equal(t, common.ChannelStatusEnabled, updated.AuthFile.Status)
	require.Equal(t, common.ChannelStatusEnabled, updated.Account.Status)
	require.True(t, updated.Account.Schedulable)
	require.Contains(t, updated.Account.CredentialSummary, "api_key")
	require.NotContains(t, updated.Account.CredentialSummary, "sk-new-secret")
	decrypted, err := common.DecryptSensitiveString(updated.Account.Credentials)
	require.NoError(t, err)
	require.Contains(t, decrypted, "sk-new-secret")
	var accountCount int64
	require.NoError(t, model.DB.Model(&model.PoolAccount{}).Count(&accountCount).Error)
	require.EqualValues(t, 1, accountCount)
}

func TestUpdateAccountPoolAuthFileDoesNotRecreateMissingAccountWithoutContent(t *testing.T) {
	setupAccountPoolAuthFileUpdateTest(t)
	group := createAuthFileUpdateGroup(t, "auth-file-no-recreate")
	content := buildAuthFileUpdateContent(t, "auth-file-no-recreate-old", "sk-old-secret")
	imported, err := ImportAccountPoolAuthFile(AccountPoolAuthFileImportOptions{
		Content:     content,
		PoolGroupID: group.Id,
	})
	require.NoError(t, err)
	require.NotNil(t, imported.AuthFile)
	require.NotNil(t, imported.Account)
	detachImportedAuthFileAccount(t, imported.AuthFile.Id, imported.Account.Id)

	name := "auth-file-no-recreate-renamed"
	updated, err := UpdateAccountPoolAuthFile(imported.AuthFile.Id, AccountPoolAuthFileUpdateOptions{
		Name: &name,
	})

	require.NoError(t, err)
	require.NotNil(t, updated.AuthFile)
	require.Nil(t, updated.Account)
	require.Equal(t, name, updated.AuthFile.Name)
	require.Zero(t, updated.AuthFile.PoolAccountId)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.AuthFile.Status)
	var accountCount int64
	require.NoError(t, model.DB.Model(&model.PoolAccount{}).Count(&accountCount).Error)
	require.EqualValues(t, 0, accountCount)
}
