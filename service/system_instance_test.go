package service

import (
	"testing"

	"github.com/c1cada/NexusTok/common"

	"github.com/stretchr/testify/require"
)

func TestResolveSystemInstanceNodeUsesConfiguredNodeName(t *testing.T) {
	origin := common.NodeName
	originSource := common.NodeNameSource
	originManuallyConfigured := common.NodeNameManuallyConfigured
	defer func() {
		common.NodeName = origin
		common.NodeNameSource = originSource
		common.NodeNameManuallyConfigured = originManuallyConfigured
	}()

	common.NodeName = " node-a "
	common.NodeNameSource = common.NodeNameSourceManual
	common.NodeNameManuallyConfigured = true

	node, err := ResolveSystemInstanceNode()

	require.NoError(t, err)
	require.Equal(t, "node-a", node.Name)
	require.Equal(t, common.NodeNameSourceManual, node.Source)
	require.True(t, node.ManuallyConfigured)
	require.False(t, node.ShouldConfigureManually)
}

func TestResolveSystemInstanceNodeFallsBackToHostname(t *testing.T) {
	origin := common.NodeName
	originSource := common.NodeNameSource
	originManuallyConfigured := common.NodeNameManuallyConfigured
	defer func() {
		common.NodeName = origin
		common.NodeNameSource = originSource
		common.NodeNameManuallyConfigured = originManuallyConfigured
	}()

	common.NodeName = ""
	common.NodeNameSource = ""
	common.NodeNameManuallyConfigured = false

	node, err := ResolveSystemInstanceNode()

	require.NoError(t, err)
	require.NotEmpty(t, node.Name)
	require.Equal(t, common.NodeNameSourceHostname, node.Source)
	require.False(t, node.ManuallyConfigured)
	require.True(t, node.ShouldConfigureManually)
}
