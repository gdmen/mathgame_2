package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

// assertPoolIsPlaylistUnion checks the invariant the playlists-only model rests
// on: a user's video pool is exactly the union of the videos in the playlists
// they subscribe to — no extra row that nothing backs, none missing.
func assertPoolIsPlaylistUnion(t *testing.T, api *Api, userID uint32, step string) {
	t.Helper()
	pool := queryVideoIDs(t, api, "SELECT video_id FROM user_has_video WHERE user_id = ?", userID)
	union := queryVideoIDs(t, api, `
		SELECT DISTINCT pv.video_id
		FROM user_playlist up
		INNER JOIN playlist_video pv ON pv.playlist_id = up.playlist_id
		WHERE up.user_id = ?`, userID)
	if !slices.Equal(pool, union) {
		t.Fatalf("%s: user_has_video is %v, but the union of the user's playlists is %v", step, pool, union)
	}
}

func queryVideoIDs(t *testing.T, api *Api, query string, args ...interface{}) []uint32 {
	t.Helper()
	rows, err := api.DB.Query(query, args...)
	if err != nil {
		t.Fatalf("query video ids: %v", err)
	}
	defer rows.Close()
	ids := []uint32{}
	for rows.Next() {
		var id uint32
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan video id: %v", err)
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func addPlaylistByID(t *testing.T, r *gin.Engine, user *User, playlistID uint32) {
	t.Helper()
	resp := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"playlist_id": playlistID})
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/playlists?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("add playlist %d: expected 200, got %d: %s", playlistID, resp.Code, resp.Body.Bytes())
	}
}

func removePlaylist(t *testing.T, r *gin.Engine, user *User, playlistID uint32) {
	t.Helper()
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/v1/playlists/%d?test_auth0_id=%s", playlistID, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK && resp.Code != http.StatusNoContent {
		t.Fatalf("remove playlist %d: expected 200 or 204, got %d: %s", playlistID, resp.Code, resp.Body.Bytes())
	}
}

// Videos are playlist-synced only, so every playlist mutation must leave the
// pool equal to the union of the user's playlists. Two overlapping playlists are
// the case that catches both failure directions: a shared video dropped when one
// of its playlists goes, or kept when the last one does.
func TestUserHasVideo_IsThePlaylistUnionAfterEveryMutation(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|union", "union@test.com", "union")

	videoIDs := insertVideos(t, api, 3)
	first := insertPlaylistWithVideos(t, api, "PLunionA", videoIDs[:2])
	second := insertPlaylistWithVideos(t, api, "PLunionB", videoIDs[1:])

	assertPoolIsPlaylistUnion(t, api, user.Id, "before any playlist")

	addPlaylistByID(t, r, user, first)
	assertPoolIsPlaylistUnion(t, api, user.Id, "after adding the first playlist")
	if got := len(listMyVideos(t, r, user)); got != 2 {
		t.Fatalf("expected 2 videos from the first playlist, got %d", got)
	}

	addPlaylistByID(t, r, user, second)
	assertPoolIsPlaylistUnion(t, api, user.Id, "after adding the overlapping playlist")
	if got := len(listMyVideos(t, r, user)); got != 3 {
		t.Fatalf("expected the 3-video union of both playlists, got %d", got)
	}

	// The shared video keeps its place: it is still in the playlist that stayed.
	removePlaylist(t, r, user, first)
	assertPoolIsPlaylistUnion(t, api, user.Id, "after removing the first playlist")
	if got := len(listMyVideos(t, r, user)); got != 2 {
		t.Fatalf("expected the second playlist's 2 videos to remain, got %d", got)
	}

	removePlaylist(t, r, user, second)
	assertPoolIsPlaylistUnion(t, api, user.Id, "after removing the last playlist")
	if got := len(listMyVideos(t, r, user)); got != 0 {
		t.Fatalf("expected an empty pool with no playlists left, got %d", got)
	}
}
