package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

const (
	TestDataDir          = "./test_data/"
	TestDataInsertScript = "insert_data.sh"
)

var testDBCounter uint64
var testVideoIDCounter uint64

// setupTestAPI creates a unique test database, runs migrations, and returns the Api, router, and a cleanup function.
// Tests can run in parallel; each gets its own DB (e.g. mathgame_test_1, mathgame_test_2).
func setupTestAPI(t *testing.T, c *common.Config) (*Api, *gin.Engine, func()) {
	t.Helper()
	cfg := *c
	cfg.MySQLDatabase = c.MySQLDatabase + "_" + fmt.Sprintf("%d", atomic.AddUint64(&testDBCounter, 1))
	// Connect without a database so we can create the test DB (connecting with non-existent DB name fails).
	connNoDB := fmt.Sprintf("%s:%s@tcp(%s:%s)/?charset=utf8mb4&parseTime=true", cfg.MySQLUser, cfg.MySQLPass, cfg.MySQLHost, cfg.MySQLPort)
	dbAdmin, err := sql.Open("mysql", connNoDB)
	if err != nil {
		t.Fatalf("connect (admin): %v", err)
	}
	_, _ = dbAdmin.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", cfg.MySQLDatabase))
	_, err = dbAdmin.Exec(fmt.Sprintf("CREATE DATABASE `%s`", cfg.MySQLDatabase))
	dbAdmin.Close()
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true", cfg.MySQLUser, cfg.MySQLPass, cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDatabase)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	api, err := NewApi(db, &cfg)
	if err != nil {
		db.Close()
		t.Fatalf("NewApi: %v", err)
	}
	api.isTest = true
	r := api.GetRouter()
	cleanup := func() {
		db.Close()
		dropDB, _ := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%s)/?charset=utf8mb4", cfg.MySQLUser, cfg.MySQLPass, cfg.MySQLHost, cfg.MySQLPort))
		_, _ = dropDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", cfg.MySQLDatabase))
		dropDB.Close()
	}
	return api, r, cleanup
}

// Set up flags; no shared DB so tests can run in parallel with setupTestAPI.
func TestMain(m *testing.M) {
	flag.Set("alsologtostderr", "true")
	flag.Set("v", "100")
	flag.Parse()
	ret := m.Run()
	os.Exit(ret)
}

func insertTestData(c *common.Config, tableName string) {
	cmd := exec.Command(
		TestDataDir+TestDataInsertScript, c.MySQLUser, c.MySQLPass, c.MySQLDatabase,
		TestDataDir+tableName+".sql")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Couldn't populate %s in db: %v", tableName, err)
		os.Exit(1)
	}
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

// insertVideosAndUserHasVideo inserts n videos (no user_id) and adds them to user_has_video for the given user. Returns video IDs.
// Uses a unique prefix per call so multiple calls in the same test (e.g. subtests) do not hit you_tube_id unique constraint.
func insertVideosAndUserHasVideo(t *testing.T, api *Api, userID uint32, n int) []uint32 {
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
	for _, vid := range ids {
		_, err := api.DB.Exec("INSERT IGNORE INTO user_has_video (user_id, video_id) VALUES (?, ?)", userID, vid)
		if err != nil {
			t.Fatalf("insert user_has_video: %v", err)
		}
	}
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

func TestPlay_RequiresThreeVideos(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|playtest", "play@test.com", "playtest")

	t.Run("ForbiddenWhenFewerThanThree", func(t *testing.T) {
		insertVideosAndUserHasVideo(t, api, user.Id, 2)
		resp := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/play/%d?test_auth0_id=%s", user.Id, user.Auth0Id), nil)
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusForbidden {
			t.Errorf("expected 403 when user has 2 videos, got %d: %s", resp.Code, resp.Body.Bytes())
		}
	})

	t.Run("SuccessWhenThreeOrMore", func(t *testing.T) {
		user2 := createTestUser(t, r, "auth0id|playtest2", "play2@test.com", "playtest2")
		insertVideosAndUserHasVideo(t, api, user2.Id, 3)
		resp := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/play/%d?test_auth0_id=%s", user2.Id, user2.Auth0Id), nil)
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Errorf("expected 200 when user has 3 videos, got %d: %s", resp.Code, resp.Body.Bytes())
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

func TestListVideos_FromUserHasVideo(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|listvid", "listvid@test.com", "listvid")
	insertVideosAndUserHasVideo(t, api, user.Id, 2)

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
	insertVideosAndUserHasVideo(t, api, user.Id, 4)

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
	var list []Playlist
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("unmarshal playlists: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 playlists, got %d", len(list))
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
	videoIDs := insertVideosAndUserHasVideo(t, api, user.Id, 1)
	playlistID := insertPlaylistWithVideos(t, api, "PLtest123", videoIDs)
	_, err = api.DB.Exec("INSERT IGNORE INTO user_playlist (user_id, playlist_id) VALUES (?, ?)", user.Id, playlistID)
	if err != nil {
		t.Fatalf("insert user_playlist: %v", err)
	}

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/playlists?test_auth0_id=%s", user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}
	body, _ := ioutil.ReadAll(resp.Body)
	var list []Playlist
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("unmarshal playlists: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 playlist, got %d", len(list))
	}
	if list[0].Id != playlistID || list[0].YouTubeId != "PLtest123" {
		t.Errorf("expected playlist id=%d you_tube_id=PLtest123, got %+v", playlistID, list[0])
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
	videoIDs := make([]uint32, 3)
	for i := 0; i < 3; i++ {
		res, err := api.DB.Exec(
			"INSERT INTO videos (title, url, thumbnailurl, you_tube_id, disabled) VALUES (?, ?, ?, ?, 0)",
			fmt.Sprintf("V%d", i), fmt.Sprintf("https://youtube.com/watch?v=v%d", i), "", fmt.Sprintf("v%d", i),
		)
		if err != nil {
			t.Fatalf("insert video: %v", err)
		}
		id, _ := res.LastInsertId()
		videoIDs[i] = uint32(id)
	}
	playlistID := insertPlaylistWithVideos(t, api, "PLadd", videoIDs)

	resp := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"playlist_id": playlistID})
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/playlists?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}

	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/videos?test_auth0_id=%s", user.Auth0Id), nil)
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
	var plist []Playlist
	if err := json.Unmarshal(resp.Body.Bytes(), &plist); err != nil {
		t.Fatalf("unmarshal playlists: %v", err)
	}
	if len(plist) != 1 {
		t.Errorf("expected 1 playlist, got %d", len(plist))
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
	var videoIDs []uint32
	for i := 0; i < 2; i++ {
		res, err := api.DB.Exec(
			"INSERT INTO videos (title, url, thumbnailurl, you_tube_id, disabled) VALUES (?, ?, ?, ?, 0)",
			fmt.Sprintf("V%d", i), fmt.Sprintf("https://youtube.com/watch?v=w%d", i), "", fmt.Sprintf("w%d", i),
		)
		if err != nil {
			t.Fatalf("insert video: %v", err)
		}
		id, _ := res.LastInsertId()
		videoIDs = append(videoIDs, uint32(id))
	}
	playlistID := insertPlaylistWithVideos(t, api, "PLrm", videoIDs)
	_, err = api.DB.Exec("INSERT IGNORE INTO user_playlist (user_id, playlist_id) VALUES (?, ?)", user.Id, playlistID)
	if err != nil {
		t.Fatalf("insert user_playlist: %v", err)
	}
	if err := api.refreshUserHasVideo(user.Id); err != nil {
		t.Fatalf("refreshUserHasVideo: %v", err)
	}

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/v1/playlists/%d?test_auth0_id=%s", playlistID, user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK && resp.Code != http.StatusNoContent {
		t.Fatalf("DELETE playlist: expected 200 or 204, got %d: %s", resp.Code, resp.Body.Bytes())
	}

	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/playlists?test_auth0_id=%s", user.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET playlists: %d", resp.Code)
	}
	var plist []Playlist
	if err := json.Unmarshal(resp.Body.Bytes(), &plist); err != nil {
		t.Fatalf("unmarshal playlists: %v", err)
	}
	if len(plist) != 0 {
		t.Errorf("expected 0 playlists after remove, got %d", len(plist))
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
