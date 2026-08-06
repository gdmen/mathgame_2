// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

func TestVideoBasic(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0id|test|1", "test_1@email.com", "test_1")
	ids := seedUserVideosViaPlaylist(t, api, user.Id, 2)

	// Create: not exposed, and not an oversight — see docs/videos.md.
	resp := httptest.NewRecorder()
	body, _ := json.Marshal(Video{Title: "smuggled in", URL: "https://ex.co/x", YouTubeId: "x"})
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/videos/?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound && resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("Expected the create route to be absent (%d or %d), got %d. . .\n%+v", http.StatusNotFound, http.StatusMethodNotAllowed, resp.Code, resp)
	}

	// List: the pool the playlist put there, and nothing the attempt above added.
	if got := listMyVideos(t, r, user); len(got) != len(ids) {
		t.Fatalf("Expected %d videos from the playlist, got %d: %+v", len(ids), len(got), got)
	}

	// Update: not exposed, and not an oversight — see docs/videos.md.
	resp = httptest.NewRecorder()
	body, _ = json.Marshal(Video{Id: ids[0], Title: "unda da sea", Disabled: true})
	req, _ = http.NewRequest("POST", fmt.Sprintf("/api/v1/videos/%d?test_auth0_id=%s", ids[0], user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound && resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("Expected the update route to be absent (%d or %d), got %d. . .\n%+v", http.StatusNotFound, http.StatusMethodNotAllowed, resp.Code, resp)
	}

	// Get: these JSON names are the client's contract, so pin the whole wire
	// shape, and pin that the row is untouched by the attempt above.
	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/videos/%d?test_auth0_id=%s", ids[0], user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}
	var wire map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	wantFields := []string{"id", "title", "url", "thumbnailurl", "you_tube_id", "disabled"}
	if len(wire) != len(wantFields) {
		t.Errorf("Expected exactly the fields %v on the wire, got %v", wantFields, wire)
	}
	for _, f := range wantFields {
		if _, ok := wire[f]; !ok {
			t.Errorf("Missing field %q in the video JSON: %v", f, wire)
		}
	}
	got := Video{}
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Title == "unda da sea" || got.Disabled {
		t.Fatalf("Expected the video row unchanged, got %+v", got)
	}
	want := Video{}
	if err := api.DB.QueryRow(
		"SELECT id, title, url, thumbnailurl, you_tube_id, disabled FROM videos WHERE id=?", ids[0]).
		Scan(&want.Id, &want.Title, &want.URL, &want.ThumbnailURL, &want.YouTubeId, &want.Disabled); err != nil {
		t.Fatalf("read video row: %v", err)
	}
	if got != want {
		t.Fatalf("GET returned %+v, but the stored row is %+v", got, want)
	}

	// Delete: not exposed, and not an oversight — see docs/videos.md.
	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("/api/v1/videos/%d?test_auth0_id=%s", ids[0], user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound && resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("Expected the delete route to be absent (%d or %d), got %d. . .\n%+v", http.StatusNotFound, http.StatusMethodNotAllowed, resp.Code, resp)
	}
	if remaining := listMyVideos(t, r, user); len(remaining) != len(ids) {
		t.Fatalf("Expected the pool untouched at %d videos, got %d", len(ids), len(remaining))
	}
}

// Titles carry non-BMP characters (emoji), which only survive a round trip on a
// utf8mb4 column and connection.
func TestVideo_NonBMPTitleRoundTrips(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0id|nonbmp", "nonbmp@test.com", "nonbmp")
	title := "14 years old defeating 16 Blue Belts with Foot Locks! 🤯"
	res, err := api.DB.Exec(
		"INSERT INTO videos (title, url, thumbnailurl, you_tube_id, disabled) VALUES (?, ?, ?, ?, 0)",
		title, "https://www.youtube.com/watch?v=B5YmbhNoD00", "", "B5YmbhNoD00")
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}
	videoID, _ := res.LastInsertId()
	subscribeUserToPlaylists(t, api, user.Id,
		insertPlaylistWithVideos(t, api, "PLnonbmp", []uint32{uint32(videoID)}))

	videos := listMyVideos(t, r, user)
	if len(videos) != 1 {
		t.Fatalf("expected 1 video, got %d", len(videos))
	}
	if videos[0].Title != title {
		t.Errorf("expected title %q back, got %q", title, videos[0].Title)
	}
}

// The floor the client gates on (MIN_PLAYABLE_VIDEOS) is the floor /play
// enforces: an account the wizard would hold back must not be playable by
// asking the API directly.
func TestPlay_RequiresPlayableVideoFloor(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|playtest", "play@test.com", "playtest")

	t.Run("ForbiddenWhenNoVideos", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/play/%d?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusForbidden {
			t.Errorf("expected 403 when user has 0 videos, got %d: %s", resp.Code, resp.Body.Bytes())
		}
	})

	t.Run("SuccessAtTheFloor", func(t *testing.T) {
		user2 := createTestUser(t, r, "auth0id|playtest2", "play2@test.com", "playtest2")
		seedUserVideosViaPlaylist(t, api, user2.Id, minPlayableVideos)
		resp := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/play/%d?test_auth0_id=%s", user2.Id, user2.Auth0Id), nil)
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Errorf("expected 200 when user has %d videos, got %d: %s", minPlayableVideos, resp.Code, resp.Body.Bytes())
		}
		body, _ := ioutil.ReadAll(resp.Body)
		var pd struct {
			Gamestate *Gamestate `json:"gamestate"`
			Problem   *Problem   `json:"problem"`
			Video     *Video     `json:"video"`
		}
		if err := json.Unmarshal(body, &pd); err != nil {
			t.Fatalf("unmarshal play data: %v", err)
		}
		if pd.Gamestate == nil || pd.Problem == nil || pd.Video == nil {
			t.Errorf("expected gamestate, problem, video in response; got %+v", pd)
		}
	})
}

// Neither failure inside selectVideoIfNull can reach the client as a success.
// selectVideo only logs, so its error had nothing written behind it and gin
// answered an empty 200; the gamestate write's error was assigned to a shadowed
// variable and never looked at again. A client gating on response.ok reads
// either one as "the reward is set".
func TestSelectVideoIfNull_FailuresReachTheClient(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}

	t.Run("PickingTheVideoFails", func(t *testing.T) {
		api, r, cleanup := setupTestAPI(t, c)
		defer cleanup()
		// A fresh gamestate holds nullVideoId, so reading it takes the
		// selection path.
		user := createTestUser(t, r, "auth0id|nullsel", "nullsel@test.com", "nullsel")
		if _, err := api.DB.Exec("DROP TABLE user_has_video"); err != nil {
			t.Fatalf("drop user_has_video: %v", err)
		}

		resp := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/gamestates/%d?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 when the pool query fails, got %d: %q", resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), "Could not select a reward video") {
			t.Errorf("expected the reward-selection error in the body, got %q", resp.Body.String())
		}
	})

	t.Run("WritingTheGamestateFails", func(t *testing.T) {
		api, r, cleanup := setupTestAPI(t, c)
		defer cleanup()
		user := createTestUser(t, r, "auth0id|nullupd", "nullupd@test.com", "nullupd")
		seedUserVideosViaPlaylist(t, api, user.Id, 1)
		gamestate, _, _, err := api.gamestateManager.Get(user.Id)
		if err != nil {
			t.Fatalf("get gamestate: %v", err)
		}
		gamestate.VideoId = nullVideoId
		// Selection still succeeds; only the write that persists it fails.
		if _, err := api.DB.Exec("DROP TABLE gamestates"); err != nil {
			t.Fatalf("drop gamestates: %v", err)
		}

		resp := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(resp)
		if err := api.selectVideoIfNull("test", ctx, gamestate, false); err == nil {
			t.Error("expected the failed gamestate write to surface as an error")
		}
		if resp.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 written to the response, got %d: %q", resp.Code, resp.Body.String())
		}
	})
}

// The reward rotates away from the video just watched when the pool can spare
// another, and repeats it when it can't. A one-video pool is the case that
// makes the floor of 1 mean anything: the exclusion has to yield, or every
// reward cycle hands back the null sentinel instead of the kid's video.
func TestSelectVideo_ExclusionsAreAPreference(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	t.Run("RepeatsTheOnlyVideo", func(t *testing.T) {
		user := createTestUser(t, r, "auth0id|selvid-one", "selvid1@test.com", "selvid1")
		ids := seedUserVideosViaPlaylist(t, api, user.Id, 1)
		got, err := api.selectVideo("test", nil, user.Id, map[uint32]bool{ids[0]: true})
		if err != nil {
			t.Fatalf("selectVideo: %v", err)
		}
		if got != ids[0] {
			t.Errorf("expected the only video (%d) back, got %d", ids[0], got)
		}
	})

	t.Run("RotatesWhenThePoolCanSpareOne", func(t *testing.T) {
		user := createTestUser(t, r, "auth0id|selvid-many", "selvid2@test.com", "selvid2")
		ids := seedUserVideosViaPlaylist(t, api, user.Id, 3)
		// Random pick, so draw enough times that an ignored exclusion shows up.
		for i := 0; i < 20; i++ {
			got, err := api.selectVideo("test", nil, user.Id, map[uint32]bool{ids[0]: true})
			if err != nil {
				t.Fatalf("selectVideo: %v", err)
			}
			if got == ids[0] {
				t.Fatalf("draw %d returned the excluded video %d with 2 others available", i, ids[0])
			}
		}
	})

	t.Run("NullSentinelWhenThePoolIsEmpty", func(t *testing.T) {
		user := createTestUser(t, r, "auth0id|selvid-none", "selvid3@test.com", "selvid3")
		got, err := api.selectVideo("test", nil, user.Id, map[uint32]bool{})
		if err != nil {
			t.Fatalf("selectVideo: %v", err)
		}
		if got != nullVideoId {
			t.Errorf("expected the null sentinel for an empty pool, got %d", got)
		}
	})
}

func TestListVideos_FromUserHasVideo(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|listvid", "listvid@test.com", "listvid")
	seedUserVideosViaPlaylist(t, api, user.Id, 2)

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var list []Video
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("unmarshal videos: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 videos from user_has_video, got %d", len(list))
	}
}

func TestPageload_NumVideosEnabled(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|pageload", "pageload@test.com", "pageload")
	seedUserVideosViaPlaylist(t, api, user.Id, 4)

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/pageload/%s?test_auth0_id=%s", url.PathEscape(user.Auth0Id), user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		NumVideosEnabled interface{} `json:"num_videos_enabled"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("unmarshal pageload: %v", err)
	}
	var count int
	switch v := data.NumVideosEnabled.(type) {
	case float64:
		count = int(v)
	case string:
		fmt.Sscanf(v, "%d", &count)
	default:
		t.Fatalf("num_videos_enabled unexpected type: %T", data.NumVideosEnabled)
	}
	if count != 4 {
		t.Errorf("expected num_videos_enabled=4, got %d", count)
	}
}

func listMyVideos(t *testing.T, r *gin.Engine, user *User) []Video {
	t.Helper()
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/videos/?test_auth0_id=%s", user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET videos: expected %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}
	var videos []Video
	if err := json.Unmarshal(resp.Body.Bytes(), &videos); err != nil {
		t.Fatalf("unmarshal videos: %v", err)
	}
	return videos
}
