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

func TestStatistics_ReturnsTotals(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|statistics-totals", "statistics@test.com", "statisticsuser")

	// Insert events: 2 solved, 2 min work (120000 ms), 1 min video (60000 ms)
	_, err = api.DB.Exec(
		"INSERT INTO events (user_id, event_type, value) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?), (?, ?, ?)",
		user.Id, SOLVED_PROBLEM, "1",
		user.Id, SOLVED_PROBLEM, "1",
		user.Id, WORKING_ON_PROBLEM, "120000",
		user.Id, WATCHING_VIDEO, "60000",
	)
	if err != nil {
		t.Fatalf("insert events: %v", err)
	}

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/statistics/%d?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var pr StatisticsResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if pr.TotalProblemsSolved != 2 {
		t.Errorf("total_problems_solved: want 2, got %d", pr.TotalProblemsSolved)
	}
	if pr.TotalWorkMinutes != 2 {
		t.Errorf("total_work_minutes: want 2, got %d", pr.TotalWorkMinutes)
	}
	if pr.TotalVideoMinutes != 1 {
		t.Errorf("total_video_minutes: want 1, got %d", pr.TotalVideoMinutes)
	}
}

func TestStatistics_EmptyUser_ReturnsZeros(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	_, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|statistics-empty", "empty@test.com", "emptyuser")

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/statistics/%d?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var pr StatisticsResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if pr.TotalProblemsSolved != 0 || pr.TotalWorkMinutes != 0 || pr.TotalVideoMinutes != 0 {
		t.Errorf("expected all zeros, got solved=%d work_min=%d video_min=%d",
			pr.TotalProblemsSolved, pr.TotalWorkMinutes, pr.TotalVideoMinutes)
	}
}

func TestStatistics_ForbiddenWhenWrongUser(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	_, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	userA := createTestUser(t, r, "auth0|statistics-a", "a@test.com", "usera")
	userB := createTestUser(t, r, "auth0|statistics-b", "b@test.com", "userb")

	// Request user B's statistics while authenticated as user A
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/statistics/%d?test_auth0_id=%s", userB.Id, userA.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d body %s", http.StatusForbidden, resp.Code, resp.Body.Bytes())
	}
}

func TestStatistics_MsToMinutes_RoundsDown(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|statistics-ms", "ms@test.com", "msuser")

	// 90000 ms = 1.5 min -> DIV 60000 = 1 minute
	_, err = api.DB.Exec(
		"INSERT INTO events (user_id, event_type, value) VALUES (?, ?, ?), (?, ?, ?)",
		user.Id, WORKING_ON_PROBLEM, "90000",
		user.Id, WATCHING_VIDEO, "60000",
	)
	if err != nil {
		t.Fatalf("insert events: %v", err)
	}

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/statistics/%d?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var pr StatisticsResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if pr.TotalWorkMinutes != 1 {
		t.Errorf("total_work_minutes (90s): want 1, got %d", pr.TotalWorkMinutes)
	}
	if pr.TotalVideoMinutes != 1 {
		t.Errorf("total_video_minutes: want 1, got %d", pr.TotalVideoMinutes)
	}
}

func TestUpdateStatisticsForUser_BackfillsCacheAndMeta(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|update-stats", "updatestats@test.com", "updatestatsuser")

	_, err = api.DB.Exec(
		"INSERT INTO events (user_id, event_type, value) VALUES (?, ?, ?), (?, ?, ?)",
		user.Id, SOLVED_PROBLEM, "1",
		user.Id, WORKING_ON_PROBLEM, "60000",
	)
	if err != nil {
		t.Fatalf("insert events: %v", err)
	}

	logPrefix := "[test]"
	if err := api.UpdateStatisticsForUser(logPrefix, user.Id); err != nil {
		t.Fatalf("UpdateStatisticsForUser: %v", err)
	}

	var totalSolved, totalWork, totalVideo int64
	err = api.DB.QueryRow(
		"SELECT total_problems_solved, total_work_minutes, total_video_minutes FROM statistics_totals WHERE user_id = ?",
		user.Id,
	).Scan(&totalSolved, &totalWork, &totalVideo)
	if err != nil {
		t.Fatalf("read statistics_totals: %v", err)
	}
	if totalSolved != 1 || totalWork != 1 || totalVideo != 0 {
		t.Errorf("statistics_totals: want solved=1 work=1 video=0, got solved=%d work=%d video=%d", totalSolved, totalWork, totalVideo)
	}

	var lastEventID uint64
	err = api.DB.QueryRow("SELECT last_event_id FROM statistics_cache_meta WHERE user_id = ?", user.Id).Scan(&lastEventID)
	if err != nil {
		t.Fatalf("read statistics_cache_meta: %v", err)
	}
	if lastEventID == 0 {
		t.Error("statistics_cache_meta.last_event_id should be set")
	}
}
