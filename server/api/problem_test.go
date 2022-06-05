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

	test_model := &Problem{
		Id:         1305619059,
		Expression: "5+112",
		Answer:     "117",
		Difficulty: 5.759936284165179,
	}

	// Backend Create
	TestApi.problemManager.Create(test_model)

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
