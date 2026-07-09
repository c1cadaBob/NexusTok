package common

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewOutboundJSONBodyReturnsReadableBodyAndSize 验证转换后的 JSON body 会保留可读内容和精确大小。
func TestNewOutboundJSONBodyReturnsReadableBodyAndSize(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"model":"gpt-4.1","input":"hello"}`)

	body, size, closer, err := NewOutboundJSONBody(payload)
	require.NoError(t, err)
	require.NotNil(t, body)
	require.NotNil(t, closer)
	t.Cleanup(func() {
		require.NoError(t, closer.Close())
	})

	require.Equal(t, int64(len(payload)), size)

	readBack, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, payload, readBack)
}
