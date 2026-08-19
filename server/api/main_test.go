package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
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
	db, dbConf, cleanup := testdb.Create(t, c, "api")
	api, err := NewApi(db, dbConf)
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

// createTestAdmin is createTestUser plus promotion to the admin role, which
// only ever happens by hand in the DB (docs/accounts.md).
func createTestAdmin(t *testing.T, api *Api, r *gin.Engine, auth0Id, email, username string) *User {
	t.Helper()
	u := createTestUser(t, r, auth0Id, email, username)
	if _, err := api.DB.Exec("UPDATE users SET role=? WHERE auth0_id=?", RoleAdmin, u.Auth0Id); err != nil {
		t.Fatalf("promote to admin: %v", err)
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
