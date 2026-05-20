package types

type ChannelError struct {
	ChannelId           int    `json:"channel_id"`
	ChannelType         int    `json:"channel_type"`
	ChannelName         string `json:"channel_name"`
	IsMultiKey          bool   `json:"is_multi_key"`
	AutoBan             bool   `json:"auto_ban"`
	UsingKey            string `json:"using_key"`
	CredentialMode      string `json:"credential_mode,omitempty"`
	AccountPool         bool   `json:"account_pool,omitempty"`
	ChannelAccountId    int    `json:"channel_account_id,omitempty"`
	ChannelAccountName  string `json:"channel_account_name,omitempty"`
	PoolGroupId         int    `json:"pool_group_id,omitempty"`
	PoolGroupName       string `json:"pool_group_name,omitempty"`
	PoolAccountId       int    `json:"pool_account_id,omitempty"`
	PoolAccountName     string `json:"pool_account_name,omitempty"`
	PoolAccountAuthType string `json:"pool_account_auth_type,omitempty"`
}

func NewChannelError(channelId int, channelType int, channelName string, isMultiKey bool, usingKey string, autoBan bool) *ChannelError {
	return &ChannelError{
		ChannelId:   channelId,
		ChannelType: channelType,
		ChannelName: channelName,
		IsMultiKey:  isMultiKey,
		AutoBan:     autoBan,
		UsingKey:    usingKey,
	}
}
