package operation_setting

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpstreamAccountSyncSettingDuration(t *testing.T) {
	tests := []struct {
		name     string
		unit     string
		expected time.Duration
	}{
		{name: "month", unit: UpstreamAccountSyncUnitMonth, expected: 30 * 24 * time.Hour},
		{name: "week", unit: UpstreamAccountSyncUnitWeek, expected: 7 * 24 * time.Hour},
		{name: "day", unit: UpstreamAccountSyncUnitDay, expected: 24 * time.Hour},
		{name: "hour", unit: UpstreamAccountSyncUnitHour, expected: time.Hour},
		{name: "minute", unit: UpstreamAccountSyncUnitMinute, expected: time.Minute},
		{name: "second", unit: UpstreamAccountSyncUnitSecond, expected: time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			duration, err := (UpstreamAccountSyncSetting{
				Enabled:  true,
				Interval: 1,
				Unit:     test.unit,
			}).Duration()
			require.NoError(t, err)
			require.Equal(t, test.expected, duration)
		})
	}
}

func TestUpstreamAccountSyncSettingRejectsInvalidDuration(t *testing.T) {
	_, err := (UpstreamAccountSyncSetting{
		Enabled:  true,
		Interval: 0,
		Unit:     UpstreamAccountSyncUnitHour,
	}).Duration()
	require.Error(t, err)

	_, err = (UpstreamAccountSyncSetting{
		Enabled:  true,
		Interval: 1,
		Unit:     "invalid",
	}).Duration()
	require.Error(t, err)

	_, err = (UpstreamAccountSyncSetting{
		Enabled:  true,
		Interval: math.MaxInt64,
		Unit:     UpstreamAccountSyncUnitSecond,
	}).Duration()
	require.Error(t, err)

	duration, err := (UpstreamAccountSyncSetting{
		Enabled:  false,
		Interval: 0,
		Unit:     "invalid",
	}).Duration()
	require.NoError(t, err)
	require.Zero(t, duration)
}
