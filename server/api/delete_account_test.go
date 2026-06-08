package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"garydmenezes.com/mathgame/server/common"
)

// seedUserData inserts a row into the per-user tables that aren't already
// created by user creation (settings + gamestates are auto-created by the
// create-user flow), plus a couple of events, so deletion has something to
// purge / anonymize.
func seedUserData(t *testing.T, api *Api, userID uint32) {
	t.Helper()
	stmts := []struct {
		q    string
		args []interface{}
	}{
		{"INSERT IGNORE INTO topic_stats (user_id, problem_type, attempts, correct, target_difficulty) VALUES (?, 1, 5, 3, 3)", []interface{}{userID}},
		{"INSERT IGNORE INTO review_queue (user_id, problem_id, interval_days) VALUES (?, 1, 1)", []interface{}{userID}},
		{"INSERT IGNORE INTO recently_shown_problems (user_id, problem_id, shown_at) VALUES (?, 1, NOW())", []interface{}{userID}},
		{"INSERT IGNORE INTO user_has_video (user_id, video_id) VALUES (?, 1)", []interface{}{userID}},
		{"INSERT IGNORE INTO user_playlist (user_id, playlist_id) VALUES (?, 1)", []interface{}{userID}},
		{"INSERT IGNORE INTO events (user_id, event_type, value) VALUES (?, 'ANSWERED_PROBLEM', '42')", []interface{}{userID}},
		{"INSERT IGNORE INTO events (user_id, event_type, value) VALUES (?, 'SELECTED_PROBLEM', '7')", []interface{}{userID}},
	}
	for _, s := range stmts {
		if _, err := api.DB.Exec(s.q, s.args...); err != nil {
			t.Fatalf("seedUserData %q: %v", s.q, err)
		}
	}
}

func countRows(t *testing.T, api *Api, query string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := api.DB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("countRows %q: %v", query, err)
	}
	return n
}

func deleteAccountReq(r interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, auth0Id, pin string) *httptest.ResponseRecorder {
	resp := httptest.NewRecorder()
	body := []byte(fmt.Sprintf(`{"pin":%q}`, pin))
	req, _ := http.NewRequest("DELETE",
		fmt.Sprintf("/api/v1/users/%s?test_auth0_id=%s", auth0Id, auth0Id),
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)
	return resp
}

func TestDeleteAccount_WrongPin_Forbidden(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0|del-wrongpin", "wrong@test.com", "wrongpin")
	if _, err := api.DB.Exec("UPDATE users SET pin='1234' WHERE id=?", user.Id); err != nil {
		t.Fatalf("set pin: %v", err)
	}
	seedUserData(t, api, user.Id)

	resp := deleteAccountReq(r, user.Auth0Id, "0000")
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong pin, got %d: %s", resp.Code, resp.Body.String())
	}

	// Nothing should have been deleted or anonymized.
	if n := countRows(t, api, "SELECT COUNT(*) FROM settings WHERE user_id=?", user.Id); n != 1 {
		t.Errorf("settings should be untouched on wrong pin, count=%d", n)
	}
	if n := countRows(t, api, "SELECT COUNT(*) FROM users WHERE auth0_id=?", user.Auth0Id); n != 1 {
		t.Errorf("users row should be untouched on wrong pin, count=%d", n)
	}
}

func TestDeleteAccount_CorrectPin_PurgesAndAnonymizes(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0|del-ok", "ok@test.com", "okuser")
	if _, err := api.DB.Exec("UPDATE users SET pin='1234' WHERE id=?", user.Id); err != nil {
		t.Fatalf("set pin: %v", err)
	}
	seedUserData(t, api, user.Id)
	eventsBefore := countRows(t, api, "SELECT COUNT(*) FROM events WHERE user_id=?", user.Id)
	if eventsBefore == 0 {
		t.Fatalf("expected some events before delete, got 0")
	}

	resp := deleteAccountReq(r, user.Auth0Id, "1234")
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.Code, resp.Body.String())
	}

	// Per-user state tables are emptied.
	perUser := []string{"settings", "gamestates", "topic_stats", "review_queue", "recently_shown_problems", "user_has_video", "user_playlist"}
	for _, tbl := range perUser {
		if n := countRows(t, api, "SELECT COUNT(*) FROM "+tbl+" WHERE user_id=?", user.Id); n != 0 {
			t.Errorf("%s should be empty after delete, count=%d", tbl, n)
		}
	}

	// events are retained (aggregate value) and still keyed to the user id.
	if n := countRows(t, api, "SELECT COUNT(*) FROM events WHERE user_id=?", user.Id); n != eventsBefore {
		t.Errorf("events should be retained unchanged, count=%d (want %d)", n, eventsBefore)
	}

	// The users row is retained but anonymized: PII scrubbed, auth0_id sentinel.
	var auth0Id, email, username, pin string
	if err := api.DB.QueryRow("SELECT auth0_id, email, username, pin FROM users WHERE id=?", user.Id).
		Scan(&auth0Id, &email, &username, &pin); err != nil {
		t.Fatalf("users row should still exist (anonymized): %v", err)
	}
	if want := fmt.Sprintf("deleted-%d", user.Id); auth0Id != want {
		t.Errorf("auth0_id = %q, want sentinel %q", auth0Id, want)
	}
	if email != "" || username != "" || pin != "" {
		t.Errorf("PII not scrubbed: email=%q username=%q pin=%q", email, username, pin)
	}

	// The original Auth0 identity is freed, so re-login creates a fresh row.
	newUser := createTestUser(t, r, "auth0|del-ok", "ok@test.com", "okuser")
	if newUser.Id == user.Id {
		t.Errorf("re-login should create a fresh users row, got same id %d", newUser.Id)
	}
}

func TestDeleteAccount_NoPinSet_Forbidden(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	// User with no PIN configured (pin defaults to ""). An empty submitted
	// PIN must NOT delete the account.
	user := createTestUser(t, r, "auth0|del-nopin", "nopin@test.com", "nopin")
	seedUserData(t, api, user.Id)

	resp := deleteAccountReq(r, user.Auth0Id, "")
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when no PIN is set, got %d: %s", resp.Code, resp.Body.String())
	}
	if n := countRows(t, api, "SELECT COUNT(*) FROM users WHERE auth0_id=?", user.Auth0Id); n != 1 {
		t.Errorf("users row should be untouched when no PIN set, count=%d", n)
	}
}

// TestDeleteAccount_NoUnpurgedUserTables guards against a future per-user table
// being added without updating account deletion. Every table that carries a
// user_id column must be either purged by perUserDeleteSQL or explicitly listed
// in retainedUserTables (events). This is the schema-driven backstop for the
// hand-maintained delete list.
func TestDeleteAccount_NoUnpurgedUserTables(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	// Tables purged by the delete handler, parsed from "DELETE FROM <t> WHERE ...".
	purged := map[string]bool{}
	for _, q := range perUserDeleteSQL {
		fields := strings.Fields(q)
		purged[strings.ToLower(fields[2])] = true
	}

	rows, err := api.DB.Query(
		`SELECT table_name FROM information_schema.columns
		 WHERE table_schema = DATABASE() AND column_name = 'user_id'`)
	if err != nil {
		t.Fatalf("query information_schema: %v", err)
	}
	defer rows.Close()

	var found int
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("scan: %v", err)
		}
		found++
		lc := strings.ToLower(table)
		if !purged[lc] && !retainedUserTables[lc] {
			t.Errorf("table %q has a user_id column but is neither purged by perUserDeleteSQL nor in retainedUserTables; account deletion would leak its rows", table)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if found == 0 {
		t.Fatalf("found no user_id tables — query likely wrong")
	}
}

func TestDeleteAccount_OtherUser_Forbidden(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	victim := createTestUser(t, r, "auth0|del-victim", "victim@test.com", "victim")
	attacker := createTestUser(t, r, "auth0|del-attacker", "attacker@test.com", "attacker")

	// attacker authenticates (test_auth0_id) but targets victim's auth0_id.
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE",
		fmt.Sprintf("/api/v1/users/%s?test_auth0_id=%s", victim.Auth0Id, attacker.Auth0Id),
		bytes.NewBuffer([]byte(`{"pin":""}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 deleting another user, got %d: %s", resp.Code, resp.Body.String())
	}
	if n := countRows(t, api, "SELECT COUNT(*) FROM users WHERE auth0_id=?", victim.Auth0Id); n != 1 {
		t.Errorf("victim row should be untouched, count=%d", n)
	}
}
