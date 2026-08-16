package upstreammodel

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldTryNextModelsURLForEndpointNotAllowed(t *testing.T) {
	require.True(t, shouldTryNextModelsURL(fmt.Errorf("status code: 403, body: {\"code\":\"ENDPOINT_NOT_ALLOWED\"}")))
	require.False(t, shouldTryNextModelsURL(fmt.Errorf("status code: 403, body: {\"code\":\"INSUFFICIENT_BALANCE\"}")))
}
