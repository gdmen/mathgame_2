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
	"os"
	"os/exec"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

const (
	TestDataDir          = "./test_data/"
	TestDataInsertScript = "insert_data.sh"
)

var TestApi *Api

// Set up a global test db and clean up after running all tests
func TestMain(m *testing.M) {
	flag.Set("alsologtostderr", "true")
	flag.Set("v", "100")
	flag.Parse()
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		fmt.Printf("Couldn't read config: %v", err)
		os.Exit(1)
	}
	ResetTestApi(c)
	defer TestApi.DB.Close()
	ret := m.Run()
	os.Exit(ret)
}

func ResetTestApi(c *common.Config) {
	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true", c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		fmt.Printf("Couldn't connect to db: %v", err)
		os.Exit(1)
	}
	db.Exec(fmt.Sprintf("DROP DATABASE %s;", c.MySQLDatabase))
	db.Exec(fmt.Sprintf("CREATE DATABASE %s;", c.MySQLDatabase))
	db.Close()
	// Reconnect specifically to the test database
	db, err = sql.Open("mysql", connectStr)
	if err != nil {
		fmt.Printf("Couldn't connect to db: %v", err)
		os.Exit(1)
	}
	TestApi, err = NewApi(db)
	if err != nil {
		fmt.Printf("Couldn't init Api: %v", err)
		os.Exit(1)
	}
	TestApi.isTest = true
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
