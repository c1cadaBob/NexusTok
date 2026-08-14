package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRegisterControllerTest(t *testing.T) {
	t.Helper()

	setupUserAuthzControllerTestDB(t)

	oldRegisterEnabled := common.RegisterEnabled
	oldPasswordRegisterEnabled := common.PasswordRegisterEnabled
	oldEmailVerificationEnabled := common.EmailVerificationEnabled
	oldQuotaForNewUser := common.QuotaForNewUser
	oldGenerateDefaultToken := constant.GenerateDefaultToken

	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.QuotaForNewUser = 0
	constant.GenerateDefaultToken = false

	t.Cleanup(func() {
		common.RegisterEnabled = oldRegisterEnabled
		common.PasswordRegisterEnabled = oldPasswordRegisterEnabled
		common.EmailVerificationEnabled = oldEmailVerificationEnabled
		common.QuotaForNewUser = oldQuotaForNewUser
		constant.GenerateDefaultToken = oldGenerateDefaultToken
	})
}

func performRegisterRequest(t *testing.T, body string) simpleAuthzResponse {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(body))

	Register(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response simpleAuthzResponse
	require.NoError(t, common.DecodeJson(w.Body, &response))
	return response
}

func TestRegisterReturnsFriendlyUsernameLengthError(t *testing.T) {
	setupRegisterControllerTest(t)

	resp := performRegisterRequest(t, `{
		"username": "this-username-is-too-long",
		"password": "Password123"
	}`)

	require.False(t, resp.Success)
	assert.Contains(t, resp.Message, "user.username_too_long")
	assert.NotContains(t, resp.Message, "Field validation")
	assert.NotContains(t, resp.Message, "User.Username")
}

func TestRegisterIgnoresEmailWhenVerificationDisabled(t *testing.T) {
	setupRegisterControllerTest(t)

	resp := performRegisterRequest(t, `{
		"username": "plain-user",
		"password": "Password123",
		"email": "owner@example.com"
	}`)

	require.True(t, resp.Success, resp.Message)
	var stored model.User
	require.NoError(t, model.DB.Where("username = ?", "plain-user").First(&stored).Error)
	assert.Empty(t, stored.Email)
}

func TestRegisterStoresVerifiedEmailWhenVerificationEnabled(t *testing.T) {
	setupRegisterControllerTest(t)
	common.EmailVerificationEnabled = true
	common.RegisterVerificationCodeWithKey("verified@example.com", "123456", common.EmailVerificationPurpose)

	resp := performRegisterRequest(t, `{
		"username": "verified-user",
		"password": "Password123",
		"email": "Verified@Example.com",
		"verification_code": "123456"
	}`)

	require.True(t, resp.Success, resp.Message)
	var stored model.User
	require.NoError(t, model.DB.Where("username = ?", "verified-user").First(&stored).Error)
	assert.Equal(t, "verified@example.com", stored.Email)
}

func TestRegisterRejectsInvalidVerifiedEmail(t *testing.T) {
	setupRegisterControllerTest(t)
	common.EmailVerificationEnabled = true

	resp := performRegisterRequest(t, `{
		"username": "invalid-email-user",
		"password": "Password123",
		"email": "not-an-email",
		"verification_code": "123456"
	}`)

	require.False(t, resp.Success)
	assert.Contains(t, resp.Message, "user.email_invalid")
}
