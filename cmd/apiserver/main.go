package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"github.com/golang/glog"

	_ "garydmenezes.com/mathgame/server/docs"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
)

func init() {
	// Long ALTER TABLE migrations sit idle on the wire while the server works,
	// and AWS/firewall NAT layers idle-timeout TCP connections in ~3 minutes.
	// Override the driver's default dialer with a 30s TCP keepalive so the
	// connection stays alive through long DDL.
	mysql.RegisterDialContext("tcp", func(ctx context.Context, addr string) (net.Conn, error) {
		// Keep in sync with cmd/migrate/main.go.
		d := net.Dialer{KeepAlive: 5 * time.Second}
		return d.DialContext(ctx, "tcp", addr)
	})
}

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
	// readTimeout/writeTimeout=30m so the driver doesn't abort a long migration
	// (ALTER TABLE on a large table can take tens of minutes).
	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&time_zone=UTC&readTimeout=30m&writeTimeout=30m", c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
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
	if gin.Mode() == gin.ReleaseMode {
		api_router.RunTLS(fmt.Sprintf(":%s", c.ApiPort), "/etc/letsencrypt/live/mikeymath.org/fullchain.pem", "/etc/letsencrypt/live/mikeymath.org/privkey.pem")
	} else {
		api_router.Run(fmt.Sprintf(":%s", c.ApiPort))
	}
}
