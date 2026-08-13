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
	// Default glog to mirror INFO+ to stderr so app logs land alongside
	// gin's request log. CLI flags can still override.
	flag.Set("alsologtostderr", "true")
	flag.Set("stderrthreshold", "INFO")
	// call this for glog to work
	flag.Parse()

	rand.Seed(time.Now().UnixNano())
	c, err := common.ReadConfig("conf.json")
	if err != nil {
		glog.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		glog.Fatal(err)
	}
	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&time_zone=UTC", c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		glog.Fatal(err)
	}
	defer db.Close()

	if err := api.RunMigrations(db); err != nil {
		glog.Fatalf("migrations: %v", err)
	}

	api, err := api.NewApi(db, c)
	if err != nil {
		glog.Fatal(err)
	}
	api_router := api.GetRouter()
	addr := fmt.Sprintf(":%s", c.ApiPort)
	if gin.Mode() == gin.ReleaseMode {
		// nginx terminates TLS and proxies /api/ here; loopback-only so the
		// plain-HTTP port is never reachable directly.
		addr = "127.0.0.1" + addr
	}
	if err := api_router.Run(addr); err != nil {
		glog.Fatal(err)
	}
}
