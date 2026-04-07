package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"garydmenezes.com/mathgame/server/common"
)

func TestGetSettings_ReturnsDefaults(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	_, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|get-settings", "getsettings@test.com", "getsettingsuser")

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/settings/%d?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var s Settings
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.UserId != user.Id {
		t.Errorf("user_id: want %d, got %d", user.Id, s.UserId)
	}
	if s.ProblemTypeBitmap != 1 {
		t.Errorf("problem_type_bitmap: want 1 (default), got %d", s.ProblemTypeBitmap)
	}
	if s.TargetDifficulty != 3 {
		t.Errorf("target_difficulty: want 3 (default), got %f", s.TargetDifficulty)
	}
	if s.TargetWorkPercentage != 70 {
		t.Errorf("target_work_percentage: want 70 (default), got %d", s.TargetWorkPercentage)
	}
}

func TestUpdateSettings_ChangesValues(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	_, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|update-settings", "updatesettings@test.com", "updatesettingsuser")

	// Update settings
	updated := Settings{
		UserId:               user.Id,
		ProblemTypeBitmap:    3,
		TargetDifficulty:     5,
		TargetWorkPercentage: 50,
	}
	body, _ := json.Marshal(updated)
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/settings/%d?test_auth0_id=%s", user.Id, user.Auth0Id), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("POST settings: expected %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	// Verify via GET
	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/settings/%d?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("GET settings: expected %d, got %d", http.StatusOK, resp.Code)
	}
	body, _ = ioutil.ReadAll(resp.Body)
	var s Settings
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.ProblemTypeBitmap != 3 {
		t.Errorf("problem_type_bitmap: want 3, got %d", s.ProblemTypeBitmap)
	}
	if s.TargetDifficulty != 5 {
		t.Errorf("target_difficulty: want 5, got %f", s.TargetDifficulty)
	}
	if s.TargetWorkPercentage != 50 {
		t.Errorf("target_work_percentage: want 50, got %d", s.TargetWorkPercentage)
	}
}

func TestUpdateSettings_GeneratesChangeEvents(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|settings-events", "settingsevents@test.com", "settingseventsuser")

	// Count baseline events from user creation
	var baselineWorkPct, baselineBitmap int
	api.DB.QueryRow("SELECT COUNT(*) FROM events WHERE user_id = ? AND event_type = ?", user.Id, SET_TARGET_WORK_PERCENTAGE).Scan(&baselineWorkPct)
	api.DB.QueryRow("SELECT COUNT(*) FROM events WHERE user_id = ? AND event_type = ?", user.Id, SET_PROBLEM_TYPE_BITMAP).Scan(&baselineBitmap)

	// Change target_work_percentage from default (70) to 40, keep bitmap same
	updated := Settings{
		UserId:               user.Id,
		ProblemTypeBitmap:    1, // same as default
		TargetDifficulty:     3, // same as default
		TargetWorkPercentage: 40,
	}
	body, _ := json.Marshal(updated)
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/settings/%d?test_auth0_id=%s", user.Id, user.Auth0Id), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("POST settings: expected %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	// Check that exactly one new SET_TARGET_WORK_PERCENTAGE event was generated
	var count int
	err = api.DB.QueryRow(
		"SELECT COUNT(*) FROM events WHERE user_id = ? AND event_type = ?",
		user.Id, SET_TARGET_WORK_PERCENTAGE,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if count != baselineWorkPct+1 {
		t.Errorf("expected %d SET_TARGET_WORK_PERCENTAGE events (baseline %d + 1), got %d", baselineWorkPct+1, baselineWorkPct, count)
	}

	// Bitmap was unchanged, so no new event
	err = api.DB.QueryRow(
		"SELECT COUNT(*) FROM events WHERE user_id = ? AND event_type = ?",
		user.Id, SET_PROBLEM_TYPE_BITMAP,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if count != baselineBitmap {
		t.Errorf("expected %d SET_PROBLEM_TYPE_BITMAP events (unchanged from baseline), got %d", baselineBitmap, count)
	}
}
