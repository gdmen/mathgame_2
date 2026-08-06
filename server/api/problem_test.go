// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

func TestProblemBasic(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	test_model := &Problem{
		Id:         1305619059,
		Expression: "5+112",
		Answer:     "117",
		Difficulty: 5.759936284165179,
	}

	// Backend Create
	api.problemManager.Create(test_model)

	// Get
	resp := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/problems/%d", test_model.Id), nil)

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. . .\n%+v", http.StatusOK, resp.Code, resp)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	model := Problem{}
	json.Unmarshal(body, &model)
	h := fnv.New32a()
	h.Write([]byte(model.Expression))
	if model.Id != h.Sum32() {
		t.Fatalf("Expected Id: %d, got: %d", h.Sum32(), model.Id)
	}
}

// Guards the DEFAULT CURRENT_TIMESTAMP wiring: created_at must be absent from
// createProblemSQL so MySQL stamps it, rather than Go writing a zero time.
func TestProblemCreatedAt(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	created := &Problem{Id: 1, Expression: "1+1", Answer: "2"}
	status, msg, err := api.problemManager.Create(created)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("Create: status %d, %s, %v", status, msg, err)
	}

	got, _, msg, err := api.problemManager.Get(created.Id)
	if err != nil {
		t.Fatalf("Get: %s %v", msg, err)
	}
	// Compare against the DB clock over the same connection so the session
	// time zone cancels out of both sides.
	var dbNow time.Time
	if err := api.problemManager.DB.QueryRow("SELECT NOW()").Scan(&dbNow); err != nil {
		t.Fatalf("SELECT NOW(): %v", err)
	}
	if delta := dbNow.Sub(got.CreatedAt); delta < -time.Minute || delta > time.Minute {
		t.Fatalf("Expected CreatedAt within a minute of DB now (%v), got %v", dbNow, got.CreatedAt)
	}
}
