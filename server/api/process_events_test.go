// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

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

func TestProcessEvents_InvalidEventType(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|invalid-type", "invalid@test.com", "invalidtype")

	resp := httptest.NewRecorder()
	event := Event{EventType: "invalid_event_type", Value: "x"}
	body, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for invalid event type, got %d body %s", http.StatusBadRequest, resp.Code, resp.Body.Bytes())
	}
}

func TestProcessEvents_LoggedIn_Persisted(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|logged-in", "login@test.com", "loginuser")
	// Add videos so play-data response (writeCtx) can resolve gamestate.VideoId
	for i := 0; i < 2; i++ {
		v := &Video{UserId: user.Id, Title: "V", URL: fmt.Sprintf("https://ex.co/login%d", i)}
		resp := httptest.NewRecorder()
		body, _ := json.Marshal(v)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("create video: expected %d, got %d", http.StatusCreated, resp.Code)
		}
	}

	resp := httptest.NewRecorder()
	event := Event{EventType: LOGGED_IN, Value: ""}
	body, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d for LOGGED_IN, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	// List events (last 3600 seconds); LOGGED_IN is included in list filter
	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/events/%d/3600?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("list events: expected %d, got %d", http.StatusOK, resp.Code)
	}
	body, _ = ioutil.ReadAll(resp.Body)
	var events []Event
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatalf("list events unmarshal: %v", err)
	}
	var found bool
	for _, e := range events {
		if e.EventType == LOGGED_IN {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("LOGGED_IN event not found in list; got %d events: %v", len(events), events)
	}
}

func TestProcessEvents_WorkingOnProblem_Accepted(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|work-problem", "work@test.com", "workuser")
	// Need gamestate to exist: add two videos so play flow works, then get gamestate once
	for i := 0; i < 2; i++ {
		v := &Video{
			UserId: user.Id,
			Title:  "Test Video",
			URL:    fmt.Sprintf("https://example.com/v%d", i),
		}
		resp := httptest.NewRecorder()
		body, _ := json.Marshal(v)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("create video: expected %d, got %d", http.StatusCreated, resp.Code)
		}
	}
	// Trigger gamestate creation by getting play data (or report SELECTED_PROBLEM after we have a problem)
	// Simplest: post one event that goes through full path so gamestate exists
	_ = reportEvent(t, r, user, SELECTED_PROBLEM, "")

	resp := httptest.NewRecorder()
	event := Event{EventType: WORKING_ON_PROBLEM, Value: "30"}
	body, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Errorf("WORKING_ON_PROBLEM: expected status %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}
}

func TestProcessEvents_WatchingVideo_Accepted(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|watch-video", "watch@test.com", "watchuser")
	for i := 0; i < 2; i++ {
		v := &Video{UserId: user.Id, Title: "V", URL: fmt.Sprintf("https://ex.co/v%d", i)}
		resp := httptest.NewRecorder()
		body, _ := json.Marshal(v)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("create video: %d", resp.Code)
		}
	}
	_ = reportEvent(t, r, user, SELECTED_PROBLEM, "")

	resp := httptest.NewRecorder()
	event := Event{EventType: WATCHING_VIDEO, Value: "60"}
	body, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Errorf("WATCHING_VIDEO: expected %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}
}

func TestProcessEvents_AnsweredProblem_WrongAnswer_DoesNotIncrementSolved(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|wrong-answer", "wrong@test.com", "wronguser")
	for i := 0; i < 2; i++ {
		v := &Video{UserId: user.Id, Title: "V", URL: fmt.Sprintf("https://ex.co/w%d", i)}
		resp := httptest.NewRecorder()
		body, _ := json.Marshal(v)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("create video: %d", resp.Code)
		}
	}
	gs := reportEvent(t, r, user, SELECTED_PROBLEM, "")
	beforeSolved := gs.Solved

	_ = reportEvent(t, r, user, ANSWERED_PROBLEM, "wrong")
	gs = reportEvent(t, r, user, ANSWERED_PROBLEM, "also wrong")
	if gs.Solved != beforeSolved {
		t.Errorf("wrong answers should not increment solved: before=%d after=%d", beforeSolved, gs.Solved)
	}
}

func TestProcessEvents_AnsweredProblem_CorrectAnswer_IncrementsSolved(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|correct-answer", "correct@test.com", "correctuser")
	for i := 0; i < 2; i++ {
		v := &Video{UserId: user.Id, Title: "V", URL: fmt.Sprintf("https://ex.co/c%d", i)}
		resp := httptest.NewRecorder()
		body, _ := json.Marshal(v)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("create video: %d", resp.Code)
		}
	}
	gs := reportEvent(t, r, user, SELECTED_PROBLEM, "")
	p := &Problem{}
	fetchProblem(t, r, user, gs.ProblemId, p)
	beforeSolved := gs.Solved

	gs = reportEvent(t, r, user, ANSWERED_PROBLEM, p.Answer)
	if gs.Solved != beforeSolved+1 {
		t.Errorf("correct answer should increment solved: before=%d after=%d", beforeSolved, gs.Solved)
	}
}

func TestProcessEvents_SetTargetWorkPercentage_Accepted(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|set-pct", "pct@test.com", "pctuser")
	for i := 0; i < 2; i++ {
		v := &Video{UserId: user.Id, Title: "V", URL: fmt.Sprintf("https://ex.co/p%d", i)}
		resp := httptest.NewRecorder()
		body, _ := json.Marshal(v)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("create video: %d", resp.Code)
		}
	}
	_ = reportEvent(t, r, user, SELECTED_PROBLEM, "")

	resp := httptest.NewRecorder()
	event := Event{EventType: SET_TARGET_WORK_PERCENTAGE, Value: "50"}
	body, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Errorf("SET_TARGET_WORK_PERCENTAGE: expected %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}
}

func TestProcessEvents_RecordOnlyEvent_ThroughCreateEvent_UsesFullPath(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()
	user := createTestUser(t, r, "auth0|record-create", "record@test.com", "recorduser")
	for i := 0; i < 2; i++ {
		v := &Video{UserId: user.Id, Title: "V", URL: fmt.Sprintf("https://ex.co/r%d", i)}
		resp := httptest.NewRecorder()
		body, _ := json.Marshal(v)
		req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("create video: %d", resp.Code)
		}
	}
	_ = reportEvent(t, r, user, SELECTED_PROBLEM, "")

	// Post a record-only event through the create event endpoint (writeCtx=true)
	// This should use the full path even though it's a record-only event, because writeCtx=true
	resp := httptest.NewRecorder()
	event := Event{EventType: WORKING_ON_PROBLEM, Value: "45"}
	body, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Errorf("record-only event via create endpoint: expected %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	// Verify event was persisted and response includes play data (indicating full path was used)
	body, _ = ioutil.ReadAll(resp.Body)
	var playData PlayData
	if err := json.Unmarshal(body, &playData); err != nil {
		t.Fatalf("unmarshal play data: %v", err)
	}
	if playData.Gamestate == nil {
		t.Errorf("expected play data response (full path), got nil gamestate")
	}
}
