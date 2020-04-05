// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/internal/api"

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/internal/common"
)

func InsertTestProblems(c *common.Config) {
	insertTestData(c, "problems")
}

func TestProblemBasic(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	ResetTestApi(c)
	r := TestApi.GetRouter()

	// Create
	resp := httptest.NewRecorder()

	values := url.Values{}
	values.Add("operations", "+")
	values.Add("operations", "-")
	values.Add("negatives", "false")
	values.Add("target_difficulty", "6")
	paramString := values.Encode()

	req, _ := http.NewRequest("POST", "/api/v1/problems/", strings.NewReader(paramString))
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

	// {"id":2806439303743259763,"expression":"5+112","answer":"117","difficulty":5.759936284165179}
	id := strings.Split(strings.Split(string(body), ",")[0], ":")[1]
	if len(id) == 0 {
		t.Fatal("ERROR: " + string(body))
	}

	// List
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("GET", "/api/v1/problems/", nil)

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(string(body)) < 2 {
		t.Fatal("ERROR: " + string(body))
	}

	// Get
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/problems/%s", id), nil)

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Delete
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("DELETE", fmt.Sprintf("/api/v1/problems/%s", id), nil)

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusNoContent, resp.Code, resp)
	}

	// List
	resp = httptest.NewRecorder()

	req, _ = http.NewRequest("GET", "/api/v1/problems/", nil)

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
