// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"encoding/json"
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
	ResetTestApi(c)
	r := TestApi.GetRouter()

	// Create
	resp := httptest.NewRecorder()

	video := Video{
		Title:    "son of man",
		URL:      "https://www.youtube.com/watch?v=-WcHPFUwd6U",
		Disabled: false,
	}
	body, _ := json.Marshal(video)
	req, _ := http.NewRequest("POST", "/api/v1/videos/", bytes.NewBuffer(body))

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusCreated, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"id":1,"title":"son of man","url":"https://www.youtube.com/watch?v=-WcHPFUwd6U","thumbnailurl":"","disabled":false}` {
		t.Fatal("ERROR: " + string(body))
	}

	// List
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("GET", "/api/v1/videos/", nil)

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `[{"id":1,"title":"son of man","url":"https://www.youtube.com/watch?v=-WcHPFUwd6U","thumbnailurl":"","disabled":false}]` {
		t.Fatal("ERROR: " + string(body))
	}

	// Update
	resp = httptest.NewRecorder()

	video.Title = "unda da sea"
	video.Disabled = true
	body, _ = json.Marshal(video)
	req, _ = http.NewRequest("POST", "/api/v1/videos/1", bytes.NewBuffer(body))

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"id":1,"title":"unda da sea","url":"https://www.youtube.com/watch?v=-WcHPFUwd6U","thumbnailurl":"","disabled":true}` {
		t.Fatal("ERROR: " + string(body))
	}

	// Get
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("GET", "/api/v1/videos/1", nil)

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"id":1,"title":"unda da sea","url":"https://www.youtube.com/watch?v=-WcHPFUwd6U","thumbnailurl":"","disabled":true}` {
		t.Fatal("ERROR: " + string(body))
	}

	// Delete
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("DELETE", "/api/v1/videos/1", nil)

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusNoContent, resp.Code, resp)
	}

	// List
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("GET", "/api/v1/videos/", nil)

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
}
