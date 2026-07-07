package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/mathcore"
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
	bitmap := uint64(mathcore.ADDITION | mathcore.SUBTRACTION | mathcore.MULTIPLICATION | mathcore.DIVISION | mathcore.MEDIUM_NUMBERS | mathcore.MISSING_NUMBER)
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

	expectedMax := mathcore.MaxDiffForBitmap(bitmap)
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
	fullBitmap := uint64(mathcore.ALL_PROBLEM_TYPES)
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

	ceiling := mathcore.MaxDiffForBitmap(fullBitmap)
	if difficulty > ceiling+0.01 {
		t.Errorf("expected difficulty <= %.2f (full-bitmap ceiling), got %.2f", ceiling, difficulty)
	}
	if ceiling < 20.0 {
		t.Errorf("full-bitmap ceiling %.2f should exceed the old hard cap of 20", ceiling)
	}
}

// TestDifficultyFloor_ClampsUpToEnvelopeFloor: the mirror of the runaway-value
// clamp in the other direction. A division-only envelope can't construct
// anything easier than ~7 (MinDiffForBitmap), so a target sitting at the
// global floor of 3 aims the selection window at a band that is empty by
// construction. The next DONE_WATCHING_VIDEO cycle raises it to the envelope
// floor.
func TestDifficultyFloor_ClampsUpToEnvelopeFloor(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0id|floortest", "floor@test.com", "floortest")
	insertVideosAndUserHasVideo(t, api, user.Id, 1)

	bitmap := uint64(mathcore.DIVISION)
	_, err = api.DB.Exec(
		`UPDATE settings SET target_difficulty = 3, problem_type_bitmap = ? WHERE user_id = ?`,
		bitmap, user.Id,
	)
	if err != nil {
		t.Fatal(err)
	}

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

	lo, _ := mathcore.TargetDifficultyRange(bitmap)
	if difficulty < lo-0.01 {
		t.Errorf("expected difficulty >= %.2f (envelope floor), got %.2f", lo, difficulty)
	}
}

// TestSetTargetDifficulty_RejectsBelowEnvelopeFloor: SET_TARGET_DIFFICULTY
// validates against the per-bitmap band, not just the global floor - a value
// the envelope can't populate is rejected up front.
func TestSetTargetDifficulty_RejectsBelowEnvelopeFloor(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0id|floorevt", "floorevt@test.com", "floorevt")
	insertVideosAndUserHasVideo(t, api, user.Id, 1)

	bitmap := uint64(mathcore.DIVISION)
	if _, err := api.DB.Exec(
		`UPDATE settings SET problem_type_bitmap = ? WHERE user_id = ?`, bitmap, user.Id,
	); err != nil {
		t.Fatal(err)
	}

	// 4.0 clears the global floor (3) but sits below the division floor (~7).
	event := Event{EventType: SET_TARGET_DIFFICULTY, Value: "4.0"}
	body, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id),
		bytes.NewBuffer(body))
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Errorf("below-floor SET_TARGET_DIFFICULTY: expected %d, got %d body %s",
			http.StatusBadRequest, resp.Code, resp.Body.Bytes())
	}

	// An in-band value is accepted.
	lo, hi := mathcore.TargetDifficultyRange(bitmap)
	event = Event{EventType: SET_TARGET_DIFFICULTY, Value: fmt.Sprintf("%.2f", (lo+hi)/2)}
	body, _ = json.Marshal(event)
	req, _ = http.NewRequest("POST",
		fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id),
		bytes.NewBuffer(body))
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Errorf("in-band SET_TARGET_DIFFICULTY: expected %d, got %d body %s",
			http.StatusOK, resp.Code, resp.Body.Bytes())
	}
}

// TestUpdateSettings_ClampsTargetDifficultyIntoBand: the settings-save path
// clamps into [floor, ceiling] before the write, in both directions.
func TestUpdateSettings_ClampsTargetDifficultyIntoBand(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0id|floorsave", "floorsave@test.com", "floorsave")

	bitmap := uint64(mathcore.DIVISION)
	lo, hi := mathcore.TargetDifficultyRange(bitmap)

	for _, tc := range []struct {
		name   string
		target float64
		want   float64
	}{
		{"below floor clamps up", 3, lo},
		{"above ceiling clamps down", 500, hi},
	} {
		updated := Settings{
			UserId:               user.Id,
			ProblemTypeBitmap:    bitmap,
			TargetDifficulty:     tc.target,
			TargetWorkPercentage: 50,
		}
		body, _ := json.Marshal(updated)
		resp := httptest.NewRecorder()
		req, _ := http.NewRequest("POST",
			fmt.Sprintf("/api/v1/settings/%d?test_auth0_id=%s", user.Id, user.Auth0Id),
			bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s: POST settings: expected %d, got %d body %s",
				tc.name, http.StatusOK, resp.Code, resp.Body.Bytes())
		}

		var difficulty float64
		if err := api.DB.QueryRow(
			`SELECT target_difficulty FROM settings WHERE user_id = ?`, user.Id,
		).Scan(&difficulty); err != nil {
			t.Fatalf("%s: query difficulty: %v", tc.name, err)
		}
		if diff := difficulty - tc.want; diff > 0.01 || diff < -0.01 {
			t.Errorf("%s: target_difficulty = %.2f, want %.2f", tc.name, difficulty, tc.want)
		}
	}
}
