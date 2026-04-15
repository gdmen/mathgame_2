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

// Regression: watching_video events are posted by the frontend with decimal
// millisecond values (e.g. "505.9579275207507"). Before the fix, the incremental
// merge path used strconv.ParseInt which fails on decimals and silently returned 0,
// so every post-cache video event contributed 0 min to both totals and monthly.
func TestUpdateStatisticsForUser_IncrementalHandlesDecimalWatchingVideo(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|stats-decimal", "decimal@test.com", "decimaluser")

	// First call: no events yet, triggers fullProgressBackfill and establishes cache_meta.
	if err := api.UpdateStatisticsForUser("[test]", user.Id); err != nil {
		t.Fatalf("initial UpdateStatisticsForUser: %v", err)
	}

	// Now insert decimal-valued watching_video events. 120 events * ~506 ms ≈ 60720 ms ≈ 1 min.
	tx, err := api.DB.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	stmt, err := tx.Prepare("INSERT INTO events (user_id, event_type, value) VALUES (?, ?, ?)")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	for i := 0; i < 120; i++ {
		if _, err := stmt.Exec(user.Id, WATCHING_VIDEO, "505.9579275207507"); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Second call: takes the incremental path.
	if err := api.UpdateStatisticsForUser("[test]", user.Id); err != nil {
		t.Fatalf("incremental UpdateStatisticsForUser: %v", err)
	}

	var totalVideo int64
	err = api.DB.QueryRow(
		"SELECT total_video_minutes FROM statistics_totals WHERE user_id = ?",
		user.Id,
	).Scan(&totalVideo)
	if err != nil {
		t.Fatalf("read statistics_totals: %v", err)
	}
	// int64(505.9579...) = 505; 505 * 120 = 60600 ms = 1 minute (floor).
	if totalVideo != 1 {
		t.Errorf("total_video_minutes: want 1, got %d", totalVideo)
	}

	var monthVideo int64
	err = api.DB.QueryRow(
		"SELECT total_video_minutes FROM statistics_monthly WHERE user_id = ? ORDER BY month DESC LIMIT 1",
		user.Id,
	).Scan(&monthVideo)
	if err != nil {
		t.Fatalf("read statistics_monthly: %v", err)
	}
	if monthVideo != 1 {
		t.Errorf("monthly total_video_minutes: want 1, got %d", monthVideo)
	}
}

// Regression: the monthly branch of mergeProgressEventsIntoCache used to divide
// per-event (d.workMin += v / 60000), which rounded every sub-minute event to 0 and
// made monthly stats dramatically under-count when event granularity is small
// (the frontend posts working_on_problem events every 500 ms).
func TestUpdateStatisticsForUser_IncrementalMonthlySumsThenDivides(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|stats-sub-min", "submin@test.com", "subminuser")

	// First call: establish cache_meta with no events, so the next call is incremental.
	if err := api.UpdateStatisticsForUser("[test]", user.Id); err != nil {
		t.Fatalf("initial UpdateStatisticsForUser: %v", err)
	}

	// 180 events * 500 ms = 90000 ms = 1 minute (with leftover 30000 ms).
	tx, err := api.DB.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	stmt, err := tx.Prepare("INSERT INTO events (user_id, event_type, value) VALUES (?, ?, ?)")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	for i := 0; i < 180; i++ {
		if _, err := stmt.Exec(user.Id, WORKING_ON_PROBLEM, "500"); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := api.UpdateStatisticsForUser("[test]", user.Id); err != nil {
		t.Fatalf("incremental UpdateStatisticsForUser: %v", err)
	}

	var monthWork int64
	err = api.DB.QueryRow(
		"SELECT total_work_minutes FROM statistics_monthly WHERE user_id = ? ORDER BY month DESC LIMIT 1",
		user.Id,
	).Scan(&monthWork)
	if err != nil {
		t.Fatalf("read statistics_monthly: %v", err)
	}
	// 180 * 500 = 90000 ms. Pre-fix: per-event 500/60000 = 0 each -> 0 min.
	// Post-fix: sum first, 90000/60000 = 1 min.
	if monthWork != 1 {
		t.Errorf("monthly total_work_minutes: want 1, got %d", monthWork)
	}
}
