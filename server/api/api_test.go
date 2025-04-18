package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"testing"

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
