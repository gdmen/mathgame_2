// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

func InsertTestVideos(c *common.Config) {
	insertTestData(c, "videos")
}

func TestVideoBasic(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	_, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	// Create new user
	user := &User{
		Auth0Id:  "auth0id|test|1",
		Email:    "test_1@email.com",
		Username: "test_1",
	}
	resp := httptest.NewRecorder()
	body, _ := json.Marshal(user)
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/users?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp_user := User{}
	err = json.Unmarshal(body, &resp_user)
	if err != nil {
		t.Fatal(err)
	}
	user.Id = resp_user.Id
	if resp_user != *user {
		t.Fatalf("Model mismatch. Received: %v, but expected: %v", resp_user, user)
	}

	// Create
	resp = httptest.NewRecorder()

	video := Video{
		Title:     "son of man",
		URL:       "https://www.youtube.com/watch?v=-WcHPFUwd6U",
		YouTubeId: "-WcHPFUwd6U",
		Disabled:  false,
	}
	body, _ = json.Marshal(video)
	req, _ = http.NewRequest("POST", fmt.Sprintf("/api/v1/videos/?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusCreated, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"id":1,"title":"son of man","url":"https://www.youtube.com/watch?v=-WcHPFUwd6U","thumbnailurl":"","you_tube_id":"-WcHPFUwd6U","disabled":false}` {
		t.Fatal("ERROR: " + string(body))
	}

	// List
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/videos/?test_auth0_id=%s", user.Auth0Id), nil)

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `[{"id":1,"title":"son of man","url":"https://www.youtube.com/watch?v=-WcHPFUwd6U","thumbnailurl":"","you_tube_id":"-WcHPFUwd6U","disabled":false}]` {
		t.Fatal("ERROR: " + string(body))
	}

	// Update
	resp = httptest.NewRecorder()

	video.Title = "unda da sea"
	video.Disabled = true
	body, _ = json.Marshal(video)
	req, _ = http.NewRequest("POST", fmt.Sprintf("/api/v1/videos/1?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"id":1,"title":"unda da sea","url":"https://www.youtube.com/watch?v=-WcHPFUwd6U","thumbnailurl":"","you_tube_id":"-WcHPFUwd6U","disabled":true}` {
		t.Fatal("ERROR: " + string(body))
	}

	// Get
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/videos/1?test_auth0_id=%s", user.Auth0Id), nil)

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"id":1,"title":"unda da sea","url":"https://www.youtube.com/watch?v=-WcHPFUwd6U","thumbnailurl":"","you_tube_id":"-WcHPFUwd6U","disabled":true}` {
		t.Fatal("ERROR: " + string(body))
	}

	// Delete
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("DELETE", fmt.Sprintf("/api/v1/videos/1?test_auth0_id=%s", user.Auth0Id), nil)

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK && resp.Code != http.StatusNoContent {
		t.Fatalf("Expected status code %d or %d, got %d. . .\n%+v", http.StatusOK, http.StatusNoContent, resp.Code, resp)
	}

	// List
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/videos/?test_auth0_id=%s", user.Auth0Id), nil)

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != "[]" {
		t.Fatal("ERROR: " + string(body))
	}

	// Create with a title that has non-BMP unicode characters
	resp = httptest.NewRecorder()

	video = Video{
		Title:     "14 years old defeating 16 Blue Belts with Foot Locks! 🤯",
		URL:       "https://www.youtube.com/watch?v=B5YmbhNoD00",
		YouTubeId: "B5YmbhNoD00",
		Disabled:  false,
	}
	body, _ = json.Marshal(video)
	req, _ = http.NewRequest("POST", fmt.Sprintf("/api/v1/videos/?test_auth0_id=%s", user.Auth0Id), bytes.NewBuffer(body))

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusCreated, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"id":2,"title":"14 years old defeating 16 Blue Belts with Foot Locks! 🤯","url":"https://www.youtube.com/watch?v=B5YmbhNoD00","thumbnailurl":"","you_tube_id":"B5YmbhNoD00","disabled":false}` {
		t.Fatal("ERROR: " + string(body))
	}

}
