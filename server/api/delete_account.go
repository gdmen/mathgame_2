// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/common/auth0"
)

// perUserDeleteSQL lists the DELETE statements that purge a user's purely
// per-user state. These tables carry no aggregate value, so they're hard
// deleted. The events table is deliberately NOT here: its rows are kept and
// de-identified (see anonymizeAndPurgeUser) so aggregate solve-time data
// survives for future difficulty calibration.
var perUserDeleteSQL = []string{
	"DELETE FROM settings WHERE user_id=?",
	"DELETE FROM gamestates WHERE user_id=?",
	"DELETE FROM topic_stats WHERE user_id=?",
	"DELETE FROM review_queue WHERE user_id=?",
	"DELETE FROM recently_shown_problems WHERE user_id=?",
	"DELETE FROM user_has_video WHERE user_id=?",
	"DELETE FROM user_playlist WHERE user_id=?",
	"DELETE FROM statistics_cache_meta WHERE user_id=?",
	"DELETE FROM statistics_totals WHERE user_id=?",
	"DELETE FROM statistics_monthly WHERE user_id=?",
}

// retainedUserTables are tables that carry a user_id but are deliberately NOT
// purged on account deletion. events rows are kept (de-identified by the
// anonymized users row) so aggregate solve-time data survives for difficulty
// calibration. TestDeleteAccount_NoUnpurgedUserTables enforces that every
// user_id-bearing table is either purged above or listed here.
var retainedUserTables = map[string]bool{
	"events": true,
}

// deleteAccountRequest is the JSON body for account deletion: the adult PIN,
// re-entered to confirm.
type deleteAccountRequest struct {
	Pin string `json:"pin"`
}

// customDeleteAccount deletes the authenticated user's account. It is gated by
// the adult PIN (re-entered by the client), purges per-user state and
// anonymizes the users row in a single transaction, then best-effort removes
// the Auth0 identity so the same login can't re-claim the (now anonymized)
// record.
func (a *Api) customDeleteAccount(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	user := GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, common.GetError("unauthorized"))
		return
	}

	// You may only delete your own account: the :auth0_id path param must
	// match the token-authenticated user loaded by userMiddleware.
	if c.Param("auth0_id") != user.Auth0Id {
		c.JSON(http.StatusForbidden, common.GetError("You can only delete your own account"))
		return
	}

	var body deleteAccountRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, common.GetError("Couldn't parse input JSON body"))
		return
	}
	// Verify the adult PIN so a stolen access token alone can't delete an
	// account. Require a PIN to actually be set: pin defaults to "" in the
	// schema, and without this guard a no-PIN account would be deletable with
	// an empty PIN, defeating the gate entirely.
	if user.Pin == "" {
		c.JSON(http.StatusForbidden, common.GetError("Set a PIN in the Adults section before deleting your account"))
		return
	}
	if body.Pin != user.Pin {
		c.JSON(http.StatusForbidden, common.GetError("Incorrect PIN"))
		return
	}

	auth0Id := user.Auth0Id
	if err := a.anonymizeAndPurgeUser(user.Id); err != nil {
		glog.Errorf("%s delete account (user_id=%d): %v", logPrefix, user.Id, err)
		c.JSON(http.StatusInternalServerError, common.GetError("Couldn't delete account"))
		return
	}

	// Best-effort: our DB is already scrubbed and is the source of truth. If
	// Auth0 removal fails or isn't configured, log and continue — the
	// anonymized local row no longer maps to this identity anyway.
	a.bestEffortDeleteAuth0User(logPrefix, auth0Id)

	c.Status(http.StatusNoContent)
}

// anonymizeAndPurgeUser hard-deletes all purely-per-user rows and anonymizes
// the users row (scrubbing PII and breaking the Auth0 link via a sentinel
// auth0_id) in a single transaction. The users row and its id are retained so
// events.user_id stays a valid foreign key.
func (a *Api) anonymizeAndPurgeUser(userID uint32) (err error) {
	tx, err := a.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, q := range perUserDeleteSQL {
		if _, err = tx.Exec(q, userID); err != nil {
			return err
		}
	}

	// Scrub PII and replace auth0_id with a per-user sentinel. auth0_id is the
	// PK and the login link: blanking it stops the same Auth0 identity from
	// silently re-claiming this row on next login, and the id-based sentinel
	// stays unique across multiple deleted users.
	sentinel := fmt.Sprintf("deleted-%d", userID)
	if _, err = tx.Exec(
		"UPDATE users SET auth0_id=?, email='', username='', pin='' WHERE id=?",
		sentinel, userID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// bestEffortDeleteAuth0User removes the Auth0 identity if Management API
// credentials are configured; otherwise it logs and returns. Never fatal.
func (a *Api) bestEffortDeleteAuth0User(logPrefix, auth0Id string) {
	id, secret := a.auth0ManagementClientId, a.auth0ManagementClientSecret
	if a.auth0Domain == "" || (id == "" && secret == "") {
		glog.Infof("%s skipping Auth0 deletion for %s: management credentials not configured", logPrefix, auth0Id)
		return
	}
	if id == "" || secret == "" {
		// Only one of the two creds is set — almost certainly a misconfig.
		// Surface it loudly rather than silently no-op'ing every deletion.
		glog.Errorf("%s skipping Auth0 deletion for %s: management credentials partially configured (need both clientId and clientSecret)", logPrefix, auth0Id)
		return
	}
	if err := auth0.DeleteUser(a.auth0Domain, a.auth0ManagementClientId, a.auth0ManagementClientSecret, auth0Id); err != nil {
		glog.Errorf("%s best-effort Auth0 deletion failed for %s: %v", logPrefix, auth0Id, err)
		return
	}
	glog.Infof("%s deleted Auth0 identity %s", logPrefix, auth0Id)
}
