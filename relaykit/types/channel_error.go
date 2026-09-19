package types

type ChannelError struct {
	ChannelId    int    `json:"channel_id"`
	ChannelType  int    `json:"channel_type"`
	ChannelName  string `json:"channel_name"`
	IsMultiKey   bool   `json:"is_multi_key"`
	AutoBan      bool   `json:"auto_ban"`
	UsingKey     string `json:"using_key"`
	RoutingKeyID uint   `json:"routing_key_id,omitempty"`
	KeySource    string `json:"key_source,omitempty"`
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
