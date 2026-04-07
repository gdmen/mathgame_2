package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"garydmenezes.com/mathgame/server/common"
)

func TestListEvents_ReturnsRecentEvents(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|list-events", "listevents@test.com", "listeventsuser")

	// Insert events directly
	_, err = api.DB.Exec(
		"INSERT INTO events (user_id, event_type, value) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)",
		user.Id, LOGGED_IN, "",
		user.Id, SOLVED_PROBLEM, "1",
		user.Id, ANSWERED_PROBLEM, "42",
	)
	if err != nil {
		t.Fatalf("insert events: %v", err)
	}

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/events/%d/3600?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var events []Event
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// The endpoint filters to specific event types: LOGGED_IN, SELECTED_PROBLEM, ANSWERED_PROBLEM, SOLVED_PROBLEM, DONE_WATCHING_VIDEO
	// LOGGED_IN, SOLVED_PROBLEM, and ANSWERED_PROBLEM should all be included
	typeSet := make(map[string]bool)
	for _, e := range events {
		typeSet[e.EventType] = true
	}
	if !typeSet[LOGGED_IN] {
		t.Errorf("expected LOGGED_IN event in results")
	}
	if !typeSet[SOLVED_PROBLEM] {
		t.Errorf("expected SOLVED_PROBLEM event in results")
	}
	if !typeSet[ANSWERED_PROBLEM] {
		t.Errorf("expected ANSWERED_PROBLEM event in results")
	}
}

func TestListEvents_ExcludesFilteredTypes(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|list-events-filter", "listfilter@test.com", "listfilteruser")

	// Count baseline events from user creation
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/events/%d/3600?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	body, _ := ioutil.ReadAll(resp.Body)
	var baseline []Event
	json.Unmarshal(body, &baseline)
	baselineCount := len(baseline)

	// Insert event types that should be filtered out (WORKING_ON_PROBLEM, WATCHING_VIDEO)
	_, err = api.DB.Exec(
		"INSERT INTO events (user_id, event_type, value) VALUES (?, ?, ?), (?, ?, ?)",
		user.Id, WORKING_ON_PROBLEM, "30000",
		user.Id, WATCHING_VIDEO, "60000",
	)
	if err != nil {
		t.Fatalf("insert events: %v", err)
	}

	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/events/%d/3600?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	body, _ = ioutil.ReadAll(resp.Body)
	var events []Event
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// WORKING_ON_PROBLEM and WATCHING_VIDEO should not appear, so count should be same as baseline
	if len(events) != baselineCount {
		t.Errorf("expected %d events (same as baseline, filtered types excluded), got %d", baselineCount, len(events))
	}
	for _, e := range events {
		if e.EventType == WORKING_ON_PROBLEM || e.EventType == WATCHING_VIDEO {
			t.Errorf("filtered event type %s should not appear in list", e.EventType)
		}
	}
}

func TestListEvents_NoExtraAfterCreation(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	_, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|list-events-empty", "listempty@test.com", "listemptyuser")

	// Get baseline count (user creation may generate events)
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/events/%d/3600?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var events []Event
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// All returned events should be valid filtered types
	allowedTypes := map[string]bool{
		LOGGED_IN: true, SELECTED_PROBLEM: true, ANSWERED_PROBLEM: true,
		SOLVED_PROBLEM: true, DONE_WATCHING_VIDEO: true,
	}
	for _, e := range events {
		if !allowedTypes[e.EventType] {
			t.Errorf("unexpected event type %q in list response", e.EventType)
		}
	}
}

func TestListEvents_IsolatedPerUser(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	userA := createTestUser(t, r, "auth0|events-iso-a", "eventsisoa@test.com", "eventsisoa")
	userB := createTestUser(t, r, "auth0|events-iso-b", "eventsisob@test.com", "eventsisob")

	// Get user B's baseline event count (from user creation)
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/events/%d/3600?test_auth0_id=%s", userB.Id, userB.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	body, _ := ioutil.ReadAll(resp.Body)
	var baselineB []Event
	json.Unmarshal(body, &baselineB)
	baselineCount := len(baselineB)

	// Insert extra events for user A only
	_, err = api.DB.Exec(
		"INSERT INTO events (user_id, event_type, value) VALUES (?, ?, ?), (?, ?, ?)",
		userA.Id, LOGGED_IN, "",
		userA.Id, SOLVED_PROBLEM, "1",
	)
	if err != nil {
		t.Fatalf("insert events: %v", err)
	}

	// User B's count should not change
	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/events/%d/3600?test_auth0_id=%s", userB.Id, userB.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, resp.Code)
	}
	body, _ = ioutil.ReadAll(resp.Body)
	var events []Event
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(events) != baselineCount {
		t.Errorf("user B should still have %d events (user A's events should not leak), got %d", baselineCount, len(events))
	}
}
