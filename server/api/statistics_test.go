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
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|statistics-totals", "statistics@test.com", "statisticsuser")

	// Insert events: 2 solved, 2 min work (120000 ms), 1 min video (60000 ms)
	_, err = TestApi.DB.Exec(
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
	ResetTestApi(c)
	r := TestApi.GetRouter()
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
	ResetTestApi(c)
	r := TestApi.GetRouter()
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
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|statistics-ms", "ms@test.com", "msuser")

	// 90000 ms = 1.5 min -> DIV 60000 = 1 minute
	_, err = TestApi.DB.Exec(
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

func TestStatistics_HardestProblems_EmptyWhenNoSessions(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|statistics-hardest-empty", "hardest-empty@test.com", "hardempty")

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
	if pr.HardestProblems == nil {
		pr.HardestProblems = []HardestProblem{}
	}
	if len(pr.HardestProblems) != 0 {
		t.Errorf("hardest_problems: want 0, got %d", len(pr.HardestProblems))
	}
}

func TestStatistics_HardestProblems_OneSession(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|statistics-hardest-one", "hardest-one@test.com", "hardone")

	// One solve session: SELECTED(5), WORK 30s, ANSWERED(wrong), WORK 10s, SOLVED(5) -> time 40000ms, attempts 2
	_, err = TestApi.DB.Exec(
		`INSERT INTO events (user_id, event_type, value) VALUES
		 (?, ?, ''), (?, ?, '30000'), (?, ?, 'wrong'), (?, ?, '10000'), (?, ?, '5')`,
		user.Id, SELECTED_PROBLEM,
		user.Id, WORKING_ON_PROBLEM,
		user.Id, ANSWERED_PROBLEM,
		user.Id, WORKING_ON_PROBLEM,
		user.Id, SOLVED_PROBLEM,
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
	if len(pr.HardestProblems) != 1 {
		t.Fatalf("hardest_problems: want 1, got %d", len(pr.HardestProblems))
	}
	h := pr.HardestProblems[0]
	if h.ProblemId != 5 {
		t.Errorf("problem_id: want 5, got %d", h.ProblemId)
	}
	if h.AvgTimeToSolveMs != 40000 {
		t.Errorf("avg_time_to_solve_ms: want 40000, got %d", h.AvgTimeToSolveMs)
	}
	if h.AvgAttemptsPerSolve != 2.0 {
		t.Errorf("avg_attempts_per_solve: want 2, got %v", h.AvgAttemptsPerSolve)
	}
	if h.TimesSeen != 1 {
		t.Errorf("times_seen: want 1, got %d", h.TimesSeen)
	}
}

func TestStatistics_HardestProblems_OrderedByAvgTime(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|statistics-hardest-order", "hardest-order@test.com", "hardorder")

	// Problem 1: one solve, 10s. Problem 2: one solve, 50s. Expect order [2, 1].
	_, err = TestApi.DB.Exec(
		`INSERT INTO events (user_id, event_type, value) VALUES
		 (?, ?, ''), (?, ?, '10000'), (?, ?, '1'),
		 (?, ?, ''), (?, ?, '50000'), (?, ?, '2')`,
		user.Id, SELECTED_PROBLEM, user.Id, WORKING_ON_PROBLEM, user.Id, SOLVED_PROBLEM,
		user.Id, SELECTED_PROBLEM, user.Id, WORKING_ON_PROBLEM, user.Id, SOLVED_PROBLEM,
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
	if len(pr.HardestProblems) != 2 {
		t.Fatalf("hardest_problems: want 2, got %d", len(pr.HardestProblems))
	}
	if pr.HardestProblems[0].ProblemId != 2 || pr.HardestProblems[0].AvgTimeToSolveMs != 50000 {
		t.Errorf("first (hardest) want problem 2 50s, got %d %d", pr.HardestProblems[0].ProblemId, pr.HardestProblems[0].AvgTimeToSolveMs)
	}
	if pr.HardestProblems[1].ProblemId != 1 || pr.HardestProblems[1].AvgTimeToSolveMs != 10000 {
		t.Errorf("second want problem 1 10s, got %d %d", pr.HardestProblems[1].ProblemId, pr.HardestProblems[1].AvgTimeToSolveMs)
	}
}

func TestStatistics_HardestProblems_AvgAcrossMultipleSolves(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|statistics-hardest-avg", "hardest-avg@test.com", "hardavg")

	// Problem 5 solved twice: first 20s + 2 attempts, second 40s + 1 attempt. Avg time 30s, avg attempts 1.5, times_seen 2.
	_, err = TestApi.DB.Exec(
		`INSERT INTO events (user_id, event_type, value) VALUES
		 (?, ?, ''), (?, ?, '20000'), (?, ?, 'x'), (?, ?, '5'),
		 (?, ?, ''), (?, ?, '40000'), (?, ?, '5')`,
		user.Id, SELECTED_PROBLEM, user.Id, WORKING_ON_PROBLEM, user.Id, ANSWERED_PROBLEM, user.Id, SOLVED_PROBLEM,
		user.Id, SELECTED_PROBLEM, user.Id, WORKING_ON_PROBLEM, user.Id, SOLVED_PROBLEM,
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
	if len(pr.HardestProblems) != 1 {
		t.Fatalf("hardest_problems: want 1, got %d", len(pr.HardestProblems))
	}
	h := pr.HardestProblems[0]
	if h.ProblemId != 5 {
		t.Errorf("problem_id: want 5, got %d", h.ProblemId)
	}
	if h.AvgTimeToSolveMs != 30000 {
		t.Errorf("avg_time_to_solve_ms: want 30000, got %d", h.AvgTimeToSolveMs)
	}
	if h.AvgAttemptsPerSolve != 1.5 {
		t.Errorf("avg_attempts_per_solve: want 1.5, got %v", h.AvgAttemptsPerSolve)
	}
	if h.TimesSeen != 2 {
		t.Errorf("times_seen: want 2, got %d", h.TimesSeen)
	}
}
