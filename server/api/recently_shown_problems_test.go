// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"garydmenezes.com/mathgame/server/common"
)

// TestRecentlyShownProblems_PopulatedOnSelectedProblem confirms that
// processing a SELECTED_PROBLEM event upserts the (user_id, problem_id)
// row into recently_shown_problems, and that re-selecting the same
// problem updates shown_at in place (no duplicate row).
func TestRecentlyShownProblems_PopulatedOnSelectedProblem(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|rsp-populate", "rsp-pop@test.com", "rsppop")
	for i := 0; i < 2; i++ {
		ytID := fmt.Sprintf("rsp%d", i)
		v := &Video{Title: "V", URL: fmt.Sprintf("https://ex.co/%s", ytID), YouTubeId: ytID}
		resp := httptest.NewRecorder()
		body, _ := json.Marshal(v)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("create video: expected %d, got %d", http.StatusCreated, resp.Code)
		}
	}

	// User creation already wrote some SELECTED_PROBLEM events (every
	// SET_PROBLEM_TYPE_BITMAP / SET_TARGET_DIFFICULTY in the user-init
	// flow triggers select_new_problem, each of which synthesizes one).
	// Snapshot the baseline so we can assert deltas without depending on
	// the exact init-flow count.
	baseline := countRecentlyShown(t, api, user.Id)
	if baseline == 0 {
		t.Fatalf("expected user-creation to have populated recently_shown_problems; got 0 rows")
	}

	gs1 := reportEvent(t, r, user, SELECTED_PROBLEM, "")
	if gs1.ProblemId == 0 {
		t.Fatalf("SELECTED_PROBLEM should leave a non-zero problem on gamestate; got 0")
	}

	// SELECTED_PROBLEM with empty value is rejected by recordRecentlyShown
	// (warns on unparseable), so the count should be unchanged.
	if got := countRecentlyShown(t, api, user.Id); got != baseline {
		t.Fatalf("SELECTED_PROBLEM with empty value should not add a row; baseline=%d got=%d", baseline, got)
	}

	// Sleep a beat so the MySQL TIMESTAMP can advance on the next upsert.
	time.Sleep(1100 * time.Millisecond)

	// Pick an existing recently_shown_problems row's problem_id and
	// re-report it. That hits the ON DUPLICATE KEY UPDATE branch and
	// should update shown_at without changing the row count.
	existingProblemID := anyRecentlyShownProblemID(t, api, user.Id)
	firstShownAt := getShownAt(t, api, user.Id, existingProblemID)

	reportEvent(t, r, user, SELECTED_PROBLEM, fmt.Sprintf("%d", existingProblemID))

	if got := countRecentlyShown(t, api, user.Id); got != baseline {
		t.Fatalf("re-report existing problem should not add a row; baseline=%d got=%d", baseline, got)
	}
	secondShownAt := getShownAt(t, api, user.Id, existingProblemID)
	if !secondShownAt.After(firstShownAt) {
		t.Errorf("expected shown_at to advance on re-show; first=%v second=%v", firstShownAt, secondShownAt)
	}
}

// TestRecentlyShownProblems_FailTolerantWrite confirms that if the cache
// table is unavailable, the SELECTED_PROBLEM event INSERT still succeeds
// and the request returns 200. The cache upsert is best-effort by design.
func TestRecentlyShownProblems_FailTolerantWrite(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|rsp-fail", "rsp-fail@test.com", "rspfail")
	for i := 0; i < 2; i++ {
		ytID := fmt.Sprintf("rspf%d", i)
		v := &Video{Title: "V", URL: fmt.Sprintf("https://ex.co/%s", ytID), YouTubeId: ytID}
		resp := httptest.NewRecorder()
		body, _ := json.Marshal(v)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("create video: expected %d, got %d", http.StatusCreated, resp.Code)
		}
	}

	// Drop the cache table. The next SELECTED_PROBLEM event must NOT
	// crash the request — the event INSERT into events should still
	// succeed and the request should return 200, with only a glog
	// warning about the failed upsert.
	if _, err := api.DB.Exec("DROP TABLE recently_shown_problems"); err != nil {
		t.Fatalf("drop recently_shown_problems: %v", err)
	}
	defer func() {
		_, _ = api.DB.Exec(`
			CREATE TABLE IF NOT EXISTS recently_shown_problems (
			    user_id      INT UNSIGNED NOT NULL,
			    problem_id   INT UNSIGNED NOT NULL,
			    shown_at     TIMESTAMP NOT NULL,
			    PRIMARY KEY (user_id, problem_id),
			    KEY idx_recently_shown_problems_user_time (user_id, shown_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	}()

	// Write path: SELECTED_PROBLEM with an explicit value hits
	// recordRecentlyShown but doesn't trigger select_new_problem.
	resp := httptest.NewRecorder()
	body, _ := json.Marshal(&Event{EventType: SELECTED_PROBLEM, Value: "12345"})
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("SELECTED_PROBLEM with missing recently_shown_problems should still 200 (write is fail-tolerant); got %d body %s", resp.Code, resp.Body.Bytes())
	}

	// Read path: SET_TARGET_DIFFICULTY triggers select_new_problem which
	// reads recently_shown_problems via loadRecentProblemIds. With the
	// table dropped, the helper should warn and return an empty exclusion
	// list, and the request should still 200.
	resp = httptest.NewRecorder()
	body, _ = json.Marshal(&Event{EventType: SET_TARGET_DIFFICULTY, Value: "10"})
	req, _ = http.NewRequest("POST", fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("SET_TARGET_DIFFICULTY with missing recently_shown_problems should still 200 (read is fail-tolerant); got %d body %s", resp.Code, resp.Body.Bytes())
	}
}

func countRecentlyShown(t *testing.T, api *Api, userID uint32) int {
	t.Helper()
	var n int
	if err := api.DB.QueryRow(
		`SELECT COUNT(*) FROM recently_shown_problems WHERE user_id = ?`,
		userID,
	).Scan(&n); err != nil {
		t.Fatalf("count recently_shown_problems: %v", err)
	}
	return n
}

func anyRecentlyShownProblemID(t *testing.T, api *Api, userID uint32) uint32 {
	t.Helper()
	var id uint32
	if err := api.DB.QueryRow(
		`SELECT problem_id FROM recently_shown_problems
		 WHERE user_id = ? ORDER BY shown_at DESC LIMIT 1`,
		userID,
	).Scan(&id); err != nil {
		t.Fatalf("anyRecentlyShownProblemID: %v", err)
	}
	return id
}

func getShownAt(t *testing.T, api *Api, userID, problemID uint32) time.Time {
	t.Helper()
	var ts time.Time
	if err := api.DB.QueryRow(
		`SELECT shown_at FROM recently_shown_problems WHERE user_id = ? AND problem_id = ?`,
		userID, problemID,
	).Scan(&ts); err != nil {
		t.Fatalf("get shown_at user=%d problem=%d: %v", userID, problemID, err)
	}
	return ts
}
