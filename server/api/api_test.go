package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/common/testdb"
)

var testVideoIDCounter uint64

// setupTestAPI creates a unique test database, runs migrations, and returns the Api, router, and a cleanup function.
// Tests can run in parallel; each gets its own DB.
func setupTestAPI(t *testing.T, c *common.Config) (*Api, *gin.Engine, func()) {
	t.Helper()
	db, cleanup := testdb.Create(t, c, "api")
	api, err := NewApi(db, c)
	if err != nil {
		cleanup()
		t.Fatalf("NewApi: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		cleanup()
		t.Fatalf("run migrations: %v", err)
	}
	api.isTest = true
	return api, api.GetRouter(), cleanup
}

// Set up flags; no shared DB so tests can run in parallel with setupTestAPI.
func TestMain(m *testing.M) {
	flag.Set("alsologtostderr", "true")
	flag.Set("v", "100")
	flag.Parse()
	ret := m.Run()
	os.Exit(ret)
}

// createTestUser creates a user via the API and returns it. Used by tests that need a valid user in the DB.
func createTestUser(t *testing.T, r *gin.Engine, auth0Id, email, username string) *User {
	t.Helper()
	u := &User{
		Auth0Id:  auth0Id,
		Email:    email,
		Username: username,
	}
	resp := httptest.NewRecorder()
	body, _ := json.Marshal(u)
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/users?test_auth0_id=%s", u.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("createTestUser: expected status %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("createTestUser: %v", err)
	}
	err = json.Unmarshal(body, u)
	if err != nil {
		t.Fatalf("createTestUser: %v", err)
	}
	return u
}

// insertVideos inserts n catalog videos owned by nobody and returns their IDs.
// Uses a unique prefix per call so multiple calls in the same test (e.g. subtests) do not hit you_tube_id unique constraint.
func insertVideos(t *testing.T, api *Api, n int) []uint32 {
	t.Helper()
	prefix := atomic.AddUint64(&testVideoIDCounter, 1)
	var ids []uint32
	for i := 0; i < n; i++ {
		ytID := fmt.Sprintf("test_%d_%d", prefix, i)
		res, err := api.DB.Exec(
			"INSERT INTO videos (title, url, thumbnailurl, you_tube_id, disabled) VALUES (?, ?, ?, ?, 0)",
			fmt.Sprintf("Test Video %d", i),
			fmt.Sprintf("https://www.youtube.com/watch?v=%s", ytID),
			"",
			ytID,
		)
		if err != nil {
			t.Fatalf("insert video: %v", err)
		}
		lastID, _ := res.LastInsertId()
		ids = append(ids, uint32(lastID))
	}
	return ids
}

// subscribeUserToPlaylists adds the user to each playlist and rebuilds their video pool.
func subscribeUserToPlaylists(t *testing.T, api *Api, userID uint32, playlistIDs ...uint32) {
	t.Helper()
	for _, pid := range playlistIDs {
		if _, err := api.DB.Exec(
			"INSERT IGNORE INTO user_playlist (user_id, playlist_id) VALUES (?, ?)", userID, pid); err != nil {
			t.Fatalf("insert user_playlist: %v", err)
		}
	}
	if err := api.refreshUserHasVideo(userID); err != nil {
		t.Fatalf("refreshUserHasVideo: %v", err)
	}
}

// seedUserVideosViaPlaylist gives the user a pool of n videos the only way the
// product supports one: a playlist they are subscribed to. Returns video IDs.
func seedUserVideosViaPlaylist(t *testing.T, api *Api, userID uint32, n int) []uint32 {
	t.Helper()
	ids := insertVideos(t, api, n)
	pid := insertPlaylistWithVideos(t, api, fmt.Sprintf("PLseed_%d", atomic.AddUint64(&testVideoIDCounter, 1)), ids)
	subscribeUserToPlaylists(t, api, userID, pid)
	return ids
}

// insertPlaylistWithVideos creates a playlist and links the given video IDs to it via playlist_video. Returns playlist ID.
func insertPlaylistWithVideos(t *testing.T, api *Api, youTubeID string, videoIDs []uint32) uint32 {
	t.Helper()
	res, err := api.DB.Exec(
		"INSERT INTO playlists (you_tube_id, title, thumbnailurl, etag) VALUES (?, ?, ?, ?)",
		youTubeID, "Test Playlist", "", "etag1",
	)
	if err != nil {
		t.Fatalf("insert playlist: %v", err)
	}
	pid, _ := res.LastInsertId()
	for _, vid := range videoIDs {
		_, err := api.DB.Exec("INSERT INTO playlist_video (playlist_id, video_id) VALUES (?, ?)", pid, vid)
		if err != nil {
			t.Fatalf("insert playlist_video: %v", err)
		}
	}
	return uint32(pid)
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

func TestListPlaylists_Empty(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	_, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|listpl-empty", "listpl@test.com", "listpl")

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/playlists?test_auth0_id=%s", user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}
	body, _ := ioutil.ReadAll(resp.Body)
	var mine MyPlaylists
	if err := json.Unmarshal(body, &mine); err != nil {
		t.Fatalf("unmarshal playlists: %v", err)
	}
	if len(mine.Playlists) != 0 {
		t.Errorf("expected 0 playlists, got %d", len(mine.Playlists))
	}
	if mine.PlayableTotal != 0 {
		t.Errorf("expected playable_total=0, got %d", mine.PlayableTotal)
	}
}

func TestListPlaylists_ReturnsUserPlaylists(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|listpl-user", "listpl2@test.com", "listpl2")
	playlistID := insertPlaylistWithVideos(t, api, "PLtest123", insertVideos(t, api, 1))
	subscribeUserToPlaylists(t, api, user.Id, playlistID)

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/playlists?test_auth0_id=%s", user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}
	body, _ := ioutil.ReadAll(resp.Body)
	var mine MyPlaylists
	if err := json.Unmarshal(body, &mine); err != nil {
		t.Fatalf("unmarshal playlists: %v", err)
	}
	if len(mine.Playlists) != 1 {
		t.Fatalf("expected 1 playlist, got %d", len(mine.Playlists))
	}
	if mine.Playlists[0].Id != playlistID || mine.Playlists[0].YouTubeId != "PLtest123" {
		t.Errorf("expected playlist id=%d you_tube_id=PLtest123, got %+v", playlistID, mine.Playlists[0])
	}
}

func TestAddPlaylist_ByPlaylistID(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|addpl", "addpl@test.com", "addpl")
	playlistID := insertPlaylistWithVideos(t, api, "PLadd", insertVideos(t, api, 3))

	addPlaylistByID(t, r, user, playlistID)

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET videos: expected 200, got %d", resp.Code)
	}
	var videos []Video
	if err := json.Unmarshal(resp.Body.Bytes(), &videos); err != nil {
		t.Fatalf("unmarshal videos: %v", err)
	}
	if len(videos) != 3 {
		t.Errorf("expected 3 videos from playlist, got %d", len(videos))
	}

	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/playlists?test_auth0_id=%s", user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET playlists: expected 200, got %d", resp.Code)
	}
	var mine MyPlaylists
	if err := json.Unmarshal(resp.Body.Bytes(), &mine); err != nil {
		t.Fatalf("unmarshal playlists: %v", err)
	}
	if len(mine.Playlists) != 1 {
		t.Errorf("expected 1 playlist, got %d", len(mine.Playlists))
	}
}

func TestRemovePlaylist(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|rmpl", "rmpl@test.com", "rmpl")
	playlistID := insertPlaylistWithVideos(t, api, "PLrm", insertVideos(t, api, 2))
	subscribeUserToPlaylists(t, api, user.Id, playlistID)

	removePlaylist(t, r, user, playlistID)

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/playlists?test_auth0_id=%s", user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET playlists: %d", resp.Code)
	}
	var mine MyPlaylists
	if err := json.Unmarshal(resp.Body.Bytes(), &mine); err != nil {
		t.Fatalf("unmarshal playlists: %v", err)
	}
	if len(mine.Playlists) != 0 {
		t.Errorf("expected 0 playlists after remove, got %d", len(mine.Playlists))
	}

	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET videos: %d", resp.Code)
	}
	var videos []Video
	if err := json.Unmarshal(resp.Body.Bytes(), &videos); err != nil {
		t.Fatalf("unmarshal videos: %v", err)
	}
	if len(videos) != 0 {
		t.Errorf("expected 0 videos after playlist removed (user_has_video refreshed), got %d", len(videos))
	}
}

// Two playlists sharing videos is the case that makes a client-side total wrong:
// the per-playlist counts sum to more reward videos than exist, because the
// shared ones are counted once per playlist. playable_total is the count the
// reward loop actually has, so the setup wizard's video gate and its final step
// read the same number.
func TestListPlaylists_PlayableTotalDeduplicates(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|dedupe", "dedupe@test.com", "dedupe")

	// Three videos, split across two playlists that overlap on two of them: the
	// per-playlist counts are 2 and 2, the union is 3.
	videoIDs := insertVideos(t, api, 3)
	first := insertPlaylistWithVideos(t, api, "PLdedupeA", videoIDs[:2])
	second := insertPlaylistWithVideos(t, api, "PLdedupeB", videoIDs[1:])
	subscribeUserToPlaylists(t, api, user.Id, first, second)

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/playlists?test_auth0_id=%s", user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}
	var mine MyPlaylists
	if err := json.Unmarshal(resp.Body.Bytes(), &mine); err != nil {
		t.Fatalf("unmarshal playlists: %v", err)
	}
	if len(mine.Playlists) != 2 {
		t.Fatalf("expected 2 playlists, got %d", len(mine.Playlists))
	}
	summed := 0
	for _, p := range mine.Playlists {
		summed += p.PlayableCount
	}
	if summed != 4 {
		t.Errorf("expected the per-playlist counts to sum to 4 (the overlap counted twice), got %d", summed)
	}
	if mine.PlayableTotal != 3 {
		t.Errorf("expected playable_total=3, got %d", mine.PlayableTotal)
	}

	// And it is the same number /pageload reports, which is the invariant the
	// wizard depends on.
	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/pageload/%s?test_auth0_id=%s", user.Auth0Id, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("pageload: expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}
	var page struct {
		NumVideosEnabled interface{} `json:"num_videos_enabled"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &page); err != nil {
		t.Fatalf("unmarshal pageload: %v", err)
	}
	// /pageload reports this through an interface{} field, so it arrives as a
	// string or a number depending on the driver; the client parseInts it.
	var count int
	switch v := page.NumVideosEnabled.(type) {
	case float64:
		count = int(v)
	case string:
		fmt.Sscanf(v, "%d", &count)
	default:
		t.Fatalf("num_videos_enabled unexpected type: %T", page.NumVideosEnabled)
	}
	if count != mine.PlayableTotal {
		t.Errorf("pageload says %d enabled videos, playlists say %d", count, mine.PlayableTotal)
	}
}
