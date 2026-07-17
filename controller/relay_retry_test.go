package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryDoesNotRetrySpecificChannelForChannelError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("specific_channel_id", "1")
	err := types.NewError(errors.New("channel invalid key"), types.ErrorCodeChannelInvalidKey, types.ErrOptionWithStatusCode(http.StatusUnauthorized))

	require.False(t, shouldRetry(c, err, 1))
}
