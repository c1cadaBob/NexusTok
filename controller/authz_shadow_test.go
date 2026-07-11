package controller

import (
	"net/http"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/service/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComparePermissionRoleShadowPoliciesReturnsUnavailableWhenShadowNotReady(t *testing.T) {
	setupAuthzAuditControllerTestDB(t)

	ctx, recorder := newAuthzAuditContext(t, http.MethodGet, "/api/authz/shadow/role-mismatches", nil)
	ComparePermissionRoleShadowPolicies(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                             `json:"success"`
		Message string                           `json:"message"`
		Data    authz.ShadowRolePolicyComparison `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Empty(t, response.Message)
	assert.False(t, response.Data.Available)
	assert.Zero(t, response.Data.MismatchCount)
	assert.Empty(t, response.Data.Mismatches)
}
