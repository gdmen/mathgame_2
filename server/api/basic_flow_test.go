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

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

func fetchGamestate(t *testing.T, r *gin.Engine, user *User, gamestate *Gamestate) {
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/gamestates/%d?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(body, gamestate)
	if err != nil {
		t.Fatal(err)
	}
}

func fetchProblem(t *testing.T, r *gin.Engine, user *User, id uint32, problem *Problem) {
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/problems/%d?test_auth0_id=%s", id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(body, &problem)
	if err != nil {
		t.Fatal(err)
	}
}

func reportEvent(t *testing.T, r *gin.Engine, user *User, eventType string, value string) *Gamestate {
	resp := httptest.NewRecorder()
	event := Event{
		EventType: eventType,
		Value:     value,
	}
	body, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/events?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	pd := &PlayData{}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(body, pd)
	if err != nil {
		t.Fatal(err)
	}
	return pd.Gamestate
}

func TestFlowBasic(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()

	u := createTestUser(t, r, "auth0id|test|1", "test_1@email.com", "test_1")
	t.Run("CreateUser", func(t *testing.T) {
		if u.Id == 0 {
			t.Fatal("expected non-zero user id")
		}
	})

	t.Run("AddVideos", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			v := &Video{
				UserId:       u.Id,
				Title:        "Sesame Street: We're The A Team -A Song",
				URL:          fmt.Sprintf("https://www.youtube.com/watch?v=rm_3bfAEpII%v", i),
				ThumbnailURL: "https://i.ytimg.com/vi/rm_3bfAEpII/hqdefault.jpg",
			}
			resp := httptest.NewRecorder()
			body, _ := json.Marshal(v)
			req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", u.Auth0Id), bytes.NewBuffer(body))
			r.ServeHTTP(resp, req)
			if resp.Code != http.StatusCreated {
				t.Fatalf("Expected status code %d, got %d", http.StatusCreated, resp.Code)
			}
		}
	})

	gs := &Gamestate{}
	t.Run("GetGamestate", func(t *testing.T) {
		fetchGamestate(t, r, u, gs)
		if gs.ProblemId == 0 {
			t.Fatal("expected non-zero problem id in gamestate")
		}
	})

	t.Run("ProblemSolvingLoop", func(t *testing.T) {
		for gs.Solved != gs.Target {
			t.Logf("gs: %v", gs)
			p := Problem{}
			fetchProblem(t, r, u, gs.ProblemId, &p)
			t.Logf("problem: %v", p)
			gs = reportEvent(t, r, u, SELECTED_PROBLEM, "")
			gs = reportEvent(t, r, u, WORKING_ON_PROBLEM, "15")

			ns := gs.Solved
			if gs.Solved < gs.Target/2 {
				gs = reportEvent(t, r, u, ANSWERED_PROBLEM, "definitely the wrong answer")
				gs = reportEvent(t, r, u, ANSWERED_PROBLEM, "-12323")
				if gs.Solved != ns {
					t.Fatal("Incorrect answers were treated as solved.")
				}
			}

			gs = reportEvent(t, r, u, ANSWERED_PROBLEM, p.Answer)
			if gs.Solved != ns+1 {
				t.Fatal("Correct answer was not incremented.")
			}
		}
	})

	t.Run("VideoRewardAndReset", func(t *testing.T) {
		gs = reportEvent(t, r, u, WATCHING_VIDEO, "10")
		gs = reportEvent(t, r, u, WATCHING_VIDEO, "10")
		gs = reportEvent(t, r, u, WATCHING_VIDEO, "10")
		gs = reportEvent(t, r, u, WATCHING_VIDEO, "10")
		gs = reportEvent(t, r, u, WATCHING_VIDEO, "10")
		gs = reportEvent(t, r, u, WATCHING_VIDEO, "10")
		gs = reportEvent(t, r, u, WATCHING_VIDEO, "10")
		gs = reportEvent(t, r, u, WATCHING_VIDEO, "10")
		if gs.Solved != gs.Target {
			t.Fatal("We should still be in a problems-done-watching-video state.")
		}
		gs = reportEvent(t, r, u, DONE_WATCHING_VIDEO, fmt.Sprint(gs.VideoId))
		if gs.Solved != 0 {
			t.Fatal("Solved should have been reset.")
		}
	})
}
