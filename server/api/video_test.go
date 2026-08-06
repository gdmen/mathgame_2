// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
