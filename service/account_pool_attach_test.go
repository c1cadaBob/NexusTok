package service

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/require"
)

func TestAttachAccountPoolAuthFileToMultipleGroups(t *testing.T) {
	setupAccountPoolAuthFileUpdateTest(t)
	sourceGroup := createAuthFileUpdateGroup(t, "attach-source")
	targetGroup := createAuthFileUpdateGroup(t, "attach-target")
	content := buildAuthFileUpdateContent(t, "attach-shared", "sk-shared-secret")
	imported, err := ImportAccountPoolAuthFile(AccountPoolAuthFileImportOptions{
		Content:     content,
		PoolGroupID: sourceGroup.Id,
	})
	require.NoError(t, err)
	require.Equal(t, imported.AuthFile.Id, imported.Account.AuthFileId)

	result, err := AttachAccountPoolAccounts(AccountPoolAttachAccountsOptions{
		TargetGroupID: targetGroup.Id,
		AuthFileIDs:   []int{imported.AuthFile.Id},
		SkipExisting:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Total)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 0, result.Skipped)
	require.Equal(t, 0, result.Failed)

	var accounts []model.PoolAccount
	require.NoError(t, model.DB.Where("auth_file_id = ?", imported.AuthFile.Id).Order("pool_group_id ASC").Find(&accounts).Error)
	require.Len(t, accounts, 2)
	require.Equal(t, sourceGroup.Id, accounts[0].PoolGroupId)
	require.Equal(t, targetGroup.Id, accounts[1].PoolGroupId)
	for _, account := range accounts {
		decrypted, decryptErr := common.DecryptSensitiveString(account.Credentials)
		require.NoError(t, decryptErr)
		require.Contains(t, decrypted, "sk-shared-secret")
	}

	duplicate, err := AttachAccountPoolAccounts(AccountPoolAttachAccountsOptions{
		TargetGroupID: targetGroup.Id,
		AuthFileIDs:   []int{imported.AuthFile.Id},
		SkipExisting:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, duplicate.Total)
	require.Equal(t, 0, duplicate.Created)
	require.Equal(t, 1, duplicate.Skipped)
}

func TestAttachAccountPoolSourceGroupCopiesAccounts(t *testing.T) {
	setupAccountPoolAuthFileUpdateTest(t)
	sourceGroup := createAuthFileUpdateGroup(t, "copy-source")
	targetGroup := createAuthFileUpdateGroup(t, "copy-target")
	content := buildAuthFileUpdateContent(t, "copy-imported", "sk-imported-secret")
	imported, err := ImportAccountPoolAuthFile(AccountPoolAuthFileImportOptions{
		Content:     content,
		PoolGroupID: sourceGroup.Id,
	})
	require.NoError(t, err)

	manualCredential := `{"api_key":"sk-manual-secret"}`
	encryptedManual, err := common.EncryptSensitiveString(manualCredential)
	require.NoError(t, err)
	manualAccount := &model.PoolAccount{
		PoolGroupId:       sourceGroup.Id,
		Name:              "copy-manual",
		Platform:          "codex",
		AuthType:          model.AccountPoolAuthTypeAPIKey,
		Credentials:       encryptedManual,
		CredentialSummary: model.NormalizeAccountPoolCredentialSummary(manualCredential),
		Status:            common.ChannelStatusEnabled,
		Schedulable:       true,
		Weight:            1,
		MaxConcurrency:    3,
	}
	require.NoError(t, model.DB.Create(manualAccount).Error)

	result, err := AttachAccountPoolAccounts(AccountPoolAttachAccountsOptions{
		TargetGroupID: targetGroup.Id,
		SourceGroupID: sourceGroup.Id,
		SkipExisting:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 2, result.Total)
	require.Equal(t, 2, result.Created)
	require.Equal(t, 0, result.Skipped)
	require.Equal(t, 0, result.Failed)

	var copied []model.PoolAccount
	require.NoError(t, model.DB.Where("pool_group_id = ?", targetGroup.Id).Order("name ASC").Find(&copied).Error)
	require.Len(t, copied, 2)
	var importedCopy *model.PoolAccount
	var manualCopy *model.PoolAccount
	for index := range copied {
		switch copied[index].Name {
		case imported.Account.Name:
			importedCopy = &copied[index]
		case manualAccount.Name:
			manualCopy = &copied[index]
		}
	}
	require.NotNil(t, importedCopy)
	require.Equal(t, imported.AuthFile.Id, importedCopy.AuthFileId)
	require.NotNil(t, manualCopy)
	require.Zero(t, manualCopy.AuthFileId)
	require.Equal(t, manualAccount.MaxConcurrency, manualCopy.MaxConcurrency)
	decryptedManual, err := common.DecryptSensitiveString(manualCopy.Credentials)
	require.NoError(t, err)
	require.Contains(t, decryptedManual, "sk-manual-secret")
}
