package main

import (
	"database/sql"
	"flag"
	"fmt"
	"math/rand"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"

	_ "garydmenezes.com/mathgame/server/docs"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
)

func main() {
	// call this for glog to work
	flag.Parse()

	rand.Seed(time.Now().UnixNano())
	c, err := common.ReadConfig("conf.json")
	if err != nil {
		glog.Fatal(err)
	}
	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8&parseTime=true&time_zone=UTC", c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
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
	if gin.Mode() == gin.ReleaseMode {
		api_router.RunTLS(fmt.Sprintf(":%s", c.ApiPort), "/etc/letsencrypt/live/cowabunga.online/fullchain.pem", "/etc/letsencrypt/live/cowabunga.online/privkey.pem")
	} else {
		api_router.Run(fmt.Sprintf(":%s", c.ApiPort))
	}
}
