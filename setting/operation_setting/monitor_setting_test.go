package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMonitorSettingChannelTestEnabledEnvOverridesEnabledConfig(t *testing.T) {
	origin := monitorSetting
	t.Cleanup(func() { monitorSetting = origin })

	t.Setenv("CHANNEL_TEST_ENABLED", "false")
	t.Setenv("CHANNEL_TEST_FREQUENCY", "5")
	monitorSetting = MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 20,
		ChannelTestMode:        ChannelTestModePassiveRecovery,
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.False(t, setting.AutoTestChannelEnabled)
	assert.Equal(t, float64(5), setting.AutoTestChannelMinutes)
	assert.Equal(t, ChannelTestModeScheduledAll, setting.ChannelTestMode)
}

func TestGetMonitorSettingChannelTestEnabledEnvCanEnableDisabledConfig(t *testing.T) {
	origin := monitorSetting
	t.Cleanup(func() { monitorSetting = origin })

	t.Setenv("CHANNEL_TEST_ENABLED", "true")
	monitorSetting = MonitorSetting{
		AutoTestChannelEnabled: false,
		AutoTestChannelMinutes: 12,
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.True(t, setting.AutoTestChannelEnabled)
	assert.Equal(t, float64(12), setting.AutoTestChannelMinutes)
	assert.Equal(t, ChannelTestModeScheduledAll, setting.ChannelTestMode)
}
