package main

import (
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/internal/api"
	"garydmenezes.com/mathgame/internal/common"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	c, err := common.ReadConfig("conf.json")
	if err != nil {
		glog.Fatal(err)
	}
	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8", c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		glog.Fatal(err)
	}
	defer db.Close()

	api, err := api.NewApi(db)
	if err != nil {
		glog.Fatal(err)
	}
	api_router := api.GetRouter()
	api_router.Run(fmt.Sprintf(":%s", c.ApiPort))
}
