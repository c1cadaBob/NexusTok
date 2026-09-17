package service

import (
	"testing"

	"github.com/c1cadaBob/NexusTok/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnqueueUpstreamSiteSyncDeduplicatesPerChannel(t *testing.T) {
	truncate(t)

	first, created, err := EnqueueUpstreamSiteSync(101)
	require.NoError(t, err)
	require.True(t, created)

	same, created, err := EnqueueUpstreamSiteSync(101)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.TaskID, same.TaskID)

	other, created, err := EnqueueUpstreamSiteSync(102)
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotEqual(t, first.TaskID, other.TaskID)
	require.NotNil(t, other.ActiveKey)
	assert.Equal(t, model.SystemTaskTypeUpstreamSync+":102", *other.ActiveKey)
}
