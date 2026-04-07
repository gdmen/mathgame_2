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

func TestGetGamestate_ReturnsInitialState(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|get-gamestate", "getgs@test.com", "getgsuser")
	insertVideosAndUserHasVideo(t, api, user.Id, 3)

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/gamestates/%d?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var gs Gamestate
	if err := json.Unmarshal(body, &gs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if gs.UserId != user.Id {
		t.Errorf("user_id: want %d, got %d", user.Id, gs.UserId)
	}
	if gs.Solved != 0 {
		t.Errorf("solved: want 0 initially, got %d", gs.Solved)
	}
	if gs.VideoId == 0 {
		t.Errorf("video_id should be auto-selected, got 0")
	}
}

func TestGetGamestate_SelectsVideo(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|gs-video-select", "gsvid@test.com", "gsviduser")
	videoIDs := insertVideosAndUserHasVideo(t, api, user.Id, 3)

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/gamestates/%d?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, resp.Code)
	}
	body, _ := ioutil.ReadAll(resp.Body)
	var gs Gamestate
	if err := json.Unmarshal(body, &gs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// The selected video should be one of the user's videos
	found := false
	for _, vid := range videoIDs {
		if gs.VideoId == vid {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("video_id %d not in user's video list %v", gs.VideoId, videoIDs)
	}
}
