package model

import (
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"gorm.io/gorm"
)

// UserVerificationState is an authoritative, credential-free projection for
// choosing an authentication method. Loading it never fetches credential secrets.
type UserVerificationState struct {
	UserID      int
	Status      int
	Role        int
	AuthVersion int64
	HasPassword bool
	HasTwoFA    bool
	TwoFALocked bool
	HasPasskey  bool
}

func GetUserVerificationState(userID int) (*UserVerificationState, error) {
	return getUserVerificationState(DB, userID, false)
}

func getUserVerificationState(tx *gorm.DB, userID int, forUpdate bool) (*UserVerificationState, error) {
	if userID <= 0 {
		return nil, ErrUserSessionInvalid
	}
	var state UserVerificationState
	query := tx.Model(&User{}).Select(
		"id AS user_id, status, role, auth_version, CASE WHEN password <> '' THEN 1 ELSE 0 END AS has_password, "+
			"EXISTS (?) AS has_two_fa, EXISTS (?) AS two_fa_locked, EXISTS (?) AS has_passkey",
		tx.Model(&TwoFA{}).Select("1").Where("user_id = ? AND is_enabled = ?", userID, true),
		tx.Model(&TwoFA{}).Select("1").Where("user_id = ? AND is_enabled = ? AND locked_until > ?", userID, true, time.Now()),
		tx.Model(&PasskeyCredential{}).Select("1").Where("user_id = ?", userID),
	).Where("id = ?", userID)
	if forUpdate {
		query = lockForUpdate(query)
	}
	if err := query.Take(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

// CreateUserSessionFromLoginFlow commits the one-time login authorization and
// the resulting session together. The user lock serializes credential changes
// and session issuance, including the per-user session limits.
func CreateUserSessionFromLoginFlow(token string, session *UserSession, validate func(*AuthFlow, *UserVerificationState) error) error {
	_, err := CreateOrReuseUserSessionFromLoginFlow(token, session, "", validate)
	return err
}

// CreateOrReuseUserSessionFromLoginFlow 在同一事务内消费一次性登录授权，
// 并续期匹配的浏览器 Session 或创建新行。
func CreateOrReuseUserSessionFromLoginFlow(token string, session *UserSession, reusableSID string, validate func(*AuthFlow, *UserVerificationState) error) (bool, error) {
	if session == nil || validate == nil {
		return false, ErrUserSessionInvalid
	}
	cacheDeadline := userSessionCacheDeadline()
	var revoked []UserSession
	var replaced *UserSession
	reused := false
	_, err := ConsumeAuthFlowWithAction(token, AuthFlowMatch{
		Purpose: AuthFlowPurposeLoginVerification, UserId: session.UserID,
	}, func(tx *gorm.DB, flow *AuthFlow) error {
		state, err := getUserVerificationState(tx, session.UserID, true)
		if err != nil {
			return err
		}
		if state.Status != common.UserStatusEnabled || state.AuthVersion != session.UserAuthVersion {
			return ErrUserSessionInactive
		}
		if err := validate(flow, state); err != nil {
			return err
		}
		var renewErr error
		reused, replaced, renewErr = renewUserSessionWithTx(tx, session, reusableSID, time.Now().Unix())
		if renewErr != nil || reused {
			return renewErr
		}
		revoked, err = createUserSessionWithLimitsTx(tx, session, time.Now().Unix())
		return err
	})
	if err != nil {
		return false, err
	}
	if reused && replaced != nil {
		replaced.Status = UserSessionStatusRevoked
		replaced.RevokedAt = time.Now().Unix()
		replaced.RevokedReason = "login_session_renewed"
		if err := writeUserSessionCache(replaced.cacheEntry(), time.Time{}); err != nil {
			common.SysLog("failed to publish renewed user session tombstone: " + err.Error())
		}
	} else {
		publishRevokedUserSessionCaches(revoked)
	}
	return reused, publishCreatedUserSession(session, cacheDeadline)
}
