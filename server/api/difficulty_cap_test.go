package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"garydmenezes.com/mathgame/server/common"
)

// TestDifficultyCap_ClampsRunawayValue verifies that a user with a pathologically
// high TargetDifficulty (from the pre-fix adjustment bug) gets clamped to maxDiff
// on their next DONE_WATCHING_VIDEO cycle.
func TestDifficultyCap_ClampsRunawayValue(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0id|difftest", "diff@test.com", "difftest")
	insertVideosAndUserHasVideo(t, api, user.Id, 1)

	// Set the user's grade level and a pathologically high target_difficulty
	// that could result from the old unbounded adjuster.
	_, err = api.DB.Exec(
		`UPDATE settings SET target_difficulty = 74082001, grade_level = 5 WHERE user_id = ?`,
		user.Id,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Fire a DONE_WATCHING_VIDEO event to trigger the clamp
	event := Event{
		EventType: DONE_WATCHING_VIDEO,
		Value:     "1",
	}
	body, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id),
		bytes.NewBuffer(body))
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	// Verify target_difficulty was clamped
	var difficulty float64
	err = api.DB.QueryRow(
		`SELECT target_difficulty FROM settings WHERE user_id = ?`, user.Id,
	).Scan(&difficulty)
	if err != nil {
		t.Fatalf("query difficulty: %v", err)
	}

	// For grade 5, maxDiff = 5*2+4 = 14
	expectedMax := 14.0
	if difficulty > expectedMax {
		t.Errorf("expected difficulty <= %.0f (grade 5 cap), got %.0f", expectedMax, difficulty)
	}
	if difficulty < 3 {
		t.Errorf("difficulty dropped too low: %.2f", difficulty)
	}
}

// TestDifficultyCap_NoGradeLevel verifies the default cap of 20 applies when
// grade_level is not set.
func TestDifficultyCap_NoGradeLevel(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0id|difftest2", "diff2@test.com", "difftest2")
	insertVideosAndUserHasVideo(t, api, user.Id, 1)

	// Set pathological difficulty with grade_level = 0 (not set)
	_, err = api.DB.Exec(
		`UPDATE settings SET target_difficulty = 10000, grade_level = 0 WHERE user_id = ?`,
		user.Id,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Fire DONE_WATCHING_VIDEO
	event := Event{
		EventType: DONE_WATCHING_VIDEO,
		Value:     "1",
	}
	body, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id),
		bytes.NewBuffer(body))
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	var difficulty float64
	err = api.DB.QueryRow(
		`SELECT target_difficulty FROM settings WHERE user_id = ?`, user.Id,
	).Scan(&difficulty)
	if err != nil {
		t.Fatalf("query difficulty: %v", err)
	}

	if difficulty > 20.0 {
		t.Errorf("expected difficulty <= 20 (default cap), got %.0f", difficulty)
	}
}
