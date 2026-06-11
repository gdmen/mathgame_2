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
// high TargetDifficulty (from the pre-fix adjustment bug) gets clamped to the
// bitmap-derived ceiling (MaxDiffForBitmap) on their next DONE_WATCHING_VIDEO
// cycle: the ceiling is the hardest problem the enabled bits can express.
func TestDifficultyCap_ClampsRunawayValue(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0id|difftest", "diff@test.com", "difftest")
	insertVideosAndUserHasVideo(t, api, user.Id, 1)

	// Give the user a moderate envelope and a pathologically high
	// target_difficulty that could result from the old unbounded adjuster.
	bitmap := uint64(ADDITION | SUBTRACTION | MULTIPLICATION | DIVISION | MEDIUM_NUMBERS | MISSING_NUMBER)
	_, err = api.DB.Exec(
		`UPDATE settings SET target_difficulty = 74082001, problem_type_bitmap = ? WHERE user_id = ?`,
		bitmap, user.Id,
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

	expectedMax := MaxDiffForBitmap(bitmap)
	if difficulty > expectedMax+0.01 {
		t.Errorf("expected difficulty <= %.2f (bitmap ceiling), got %.2f", expectedMax, difficulty)
	}
	if difficulty < 3 {
		t.Errorf("difficulty dropped too low: %.2f", difficulty)
	}
}

// TestDifficultyCap_FullBitmap verifies the ceiling scales with the envelope:
// an everything-enabled bitmap clamps at the open-scale system maximum
// (~62), not at the old hard 20.
func TestDifficultyCap_FullBitmap(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0id|difftest2", "diff2@test.com", "difftest2")
	insertVideosAndUserHasVideo(t, api, user.Id, 1)

	// Set pathological difficulty with every bit enabled.
	fullBitmap := uint64(ALL_PROBLEM_TYPES)
	_, err = api.DB.Exec(
		`UPDATE settings SET target_difficulty = 10000, problem_type_bitmap = ? WHERE user_id = ?`,
		fullBitmap, user.Id,
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

	ceiling := MaxDiffForBitmap(fullBitmap)
	if difficulty > ceiling+0.01 {
		t.Errorf("expected difficulty <= %.2f (full-bitmap ceiling), got %.2f", ceiling, difficulty)
	}
	if ceiling < 20.0 {
		t.Errorf("full-bitmap ceiling %.2f should exceed the old hard cap of 20", ceiling)
	}
}
