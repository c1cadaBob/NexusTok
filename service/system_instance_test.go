package service

import (
	"testing"

	"github.com/c1cada/NexusTok/common"

	"github.com/stretchr/testify/require"
)

func TestResolveSystemInstanceNodeUsesConfiguredNodeName(t *testing.T) {
	origin := common.NodeName
	defer func() { common.NodeName = origin }()

	common.NodeName = " node-a "

	node, err := ResolveSystemInstanceNode()

	require.NoError(t, err)
	require.Equal(t, "node-a", node.Name)
	require.Equal(t, "env", node.Source)
	require.True(t, node.ManuallyConfigured)
	require.False(t, node.ShouldConfigureManually)
}

func TestResolveSystemInstanceNodeFallsBackToHostname(t *testing.T) {
	origin := common.NodeName
	defer func() { common.NodeName = origin }()

	common.NodeName = ""

	node, err := ResolveSystemInstanceNode()

	require.NoError(t, err)
	require.NotEmpty(t, node.Name)
	require.Equal(t, "hostname", node.Source)
	require.False(t, node.ManuallyConfigured)
	require.True(t, node.ShouldConfigureManually)
}
