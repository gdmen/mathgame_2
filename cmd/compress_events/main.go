package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
)

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON")
	dryRun := flag.Bool("dry-run", false, "log planned updates and deletes without writing")
	flag.Parse()

	c, err := common.ReadConfig(*configPath)
	if err != nil {
		glog.Fatal(err)
	}

	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&time_zone=UTC",
		c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		glog.Fatal(err)
	}
	defer db.Close()

	if err := api.RunMigrations(db); err != nil {
		glog.Fatalf("migrations: %v", err)
	}

	if *dryRun {
		updates, toDelete, err := api.PlanCompress(db)
		if err != nil {
			glog.Fatal(err)
		}
		fmt.Fprintf(os.Stdout, "dry-run: would update %d rows, delete %d rows\n", len(updates), len(toDelete))
		for _, e := range updates {
			fmt.Fprintf(os.Stdout, "  update id=%d user_id=%d %s value=%s\n", e.Id, e.UserId, e.EventType, e.Value)
		}
		for _, e := range toDelete {
			fmt.Fprintf(os.Stdout, "  delete id=%d user_id=%d\n", e.Id, e.UserId)
		}
		return
	}

	numUpdates, numDeletes, err := api.RunCompress(db)
	if err != nil {
		glog.Fatal(err)
	}
	fmt.Fprintf(os.Stdout, "compressed: updated %d rows, deleted %d rows\n", numUpdates, numDeletes)
}
