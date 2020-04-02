// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/internal/api"

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/internal/common"
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

	values := url.Values{}
	values.Add("title", "under the sea")
	values.Add("youtube_id", "C8OBlq_svBY")
	values.Add("start", "12")
	values.Add("end", "186")
	values.Add("enabled", "1")
	paramString := values.Encode()

	req, _ := http.NewRequest("POST", "/api/v1/videos/", strings.NewReader(paramString))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(paramString)))

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusCreated, resp.Code, resp)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"id":1,"title":"under the sea","youtube_id":"C8OBlq_svBY","start":12,"end":186,"enabled":true}` {
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
	if strings.TrimSpace(string(body)) != `[{"id":1,"title":"under the sea","youtube_id":"C8OBlq_svBY","start":12,"end":186,"enabled":true}]` {
		t.Fatal("ERROR: " + string(body))
	}

	// Update
	resp = httptest.NewRecorder()

	values.Set("title", "unda da sea")
	values.Set("enabled", "0")
	paramString = values.Encode()

	req, _ = http.NewRequest("POST", "/api/v1/videos/1", strings.NewReader(paramString))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(paramString)))

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"id":1,"title":"unda da sea","youtube_id":"C8OBlq_svBY","start":12,"end":186,"enabled":false}` {
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
	if strings.TrimSpace(string(body)) != `{"id":1,"title":"unda da sea","youtube_id":"C8OBlq_svBY","start":12,"end":186,"enabled":false}` {
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
