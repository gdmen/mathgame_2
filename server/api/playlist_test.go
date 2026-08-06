package api // import "garydmenezes.com/mathgame/server/api"

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"garydmenezes.com/mathgame/server/common"
)

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

// Removing a playlist that held the selected reward has to repoint the
// gamestate: /play resolves the reward by id alone, so a stale id keeps serving
// a video the user no longer has until the next reward cycle.
func TestRemovePlaylist_RepointsTheRewardItTookAway(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|rmreward", "rmreward@test.com", "rmreward")

	videoIDs := insertVideos(t, api, 2)
	removed := insertPlaylistWithVideos(t, api, "PLrewardGone", videoIDs[:1])
	kept := insertPlaylistWithVideos(t, api, "PLrewardKept", videoIDs[1:])
	subscribeUserToPlaylists(t, api, user.Id, removed, kept)
	setRewardVideo(t, api, user.Id, videoIDs[0])

	removePlaylist(t, r, user, removed)
	if got := rewardVideo(t, api, user.Id); got != videoIDs[1] {
		t.Fatalf("expected the reward to move to the kept playlist's video (%d), got %d", videoIDs[1], got)
	}

	// With no playlists left there is nothing to point at, and nullVideoId is
	// how the gamestate already spells an empty pool.
	removePlaylist(t, r, user, kept)
	if got := rewardVideo(t, api, user.Id); got != nullVideoId {
		t.Fatalf("expected nullVideoId once the pool is empty, got %d", got)
	}
}

// Adding a playlist shrinks the pool too, because the sync rebuilds
// playlist_video from YouTube's current answer. The DELETE below stands in for
// that rebuild dropping the video that was the reward.
func TestAddPlaylist_RepointsARewardTheResyncDropped(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|addreward", "addreward@test.com", "addreward")

	videoIDs := insertVideos(t, api, 2)
	playlistID := insertPlaylistWithVideos(t, api, "PLresync", videoIDs)
	subscribeUserToPlaylists(t, api, user.Id, playlistID)
	setRewardVideo(t, api, user.Id, videoIDs[0])

	if _, err := api.DB.Exec(
		"DELETE FROM playlist_video WHERE playlist_id=? AND video_id=?", playlistID, videoIDs[0]); err != nil {
		t.Fatalf("drop video from playlist: %v", err)
	}

	addPlaylistByID(t, r, user, playlistID)
	if got := rewardVideo(t, api, user.Id); got != videoIDs[1] {
		t.Fatalf("expected the reward to move to the video the playlist still has (%d), got %d", videoIDs[1], got)
	}
}

func setRewardVideo(t *testing.T, api *Api, userID, videoID uint32) {
	t.Helper()
	if _, err := api.DB.Exec("UPDATE gamestates SET video_id=? WHERE user_id=?", videoID, userID); err != nil {
		t.Fatalf("set reward video: %v", err)
	}
}

func rewardVideo(t *testing.T, api *Api, userID uint32) uint32 {
	t.Helper()
	var videoID uint32
	if err := api.DB.QueryRow("SELECT video_id FROM gamestates WHERE user_id=?", userID).Scan(&videoID); err != nil {
		t.Fatalf("read reward video: %v", err)
	}
	return videoID
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
