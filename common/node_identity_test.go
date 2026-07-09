package common

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func restoreNodeIdentityGlobals(t *testing.T) {
	t.Helper()

	originName := NodeName
	originSource := NodeNameSource
	originManuallyConfigured := NodeNameManuallyConfigured

	t.Cleanup(func() {
		NodeName = originName
		NodeNameSource = originSource
		NodeNameManuallyConfigured = originManuallyConfigured
	})
}

func TestInitNodeNameIdentityUsesManualNodeName(t *testing.T) {
	restoreNodeIdentityGlobals(t)
	t.Setenv("NODE_NAME", " node-a ")

	initNodeNameIdentity()
	identity := GetNodeIdentity()

	require.Equal(t, "node-a", NodeName)
	require.Equal(t, NodeNameSourceManual, NodeNameSource)
	require.True(t, NodeNameManuallyConfigured)
	require.Equal(t, NodeIdentity{
		Name:                    "node-a",
		Source:                  NodeNameSourceManual,
		ManuallyConfigured:      true,
		ShouldConfigureManually: false,
	}, identity)
}

func TestInitNodeNameIdentityFallsBackToHostname(t *testing.T) {
	restoreNodeIdentityGlobals(t)
	t.Setenv("NODE_NAME", "")

	hostname, err := os.Hostname()
	require.NoError(t, err)
	hostname = strings.TrimSpace(hostname)
	require.NotEmpty(t, hostname)

	initNodeNameIdentity()
	identity := GetNodeIdentity()

	require.Equal(t, hostname, NodeName)
	require.Equal(t, NodeNameSourceHostname, NodeNameSource)
	require.False(t, NodeNameManuallyConfigured)
	require.Equal(t, NodeIdentity{
		Name:                    hostname,
		Source:                  NodeNameSourceHostname,
		ManuallyConfigured:      false,
		ShouldConfigureManually: true,
	}, identity)
}
