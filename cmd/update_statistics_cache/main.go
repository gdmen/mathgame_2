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
	userID := flag.Uint("user_id", 0, "update cache for a single user (0 = all users)")
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

	a, err := api.NewApi(db, c)
	if err != nil {
		glog.Fatal(err)
	}

	var userIDs []uint32
	if *userID > 0 {
		userIDs = []uint32{uint32(*userID)}
	} else {
		rows, err := db.Query(`SELECT DISTINCT user_id FROM events ORDER BY user_id`)
		if err != nil {
			glog.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id uint32
			if err := rows.Scan(&id); err != nil {
				glog.Fatal(err)
			}
			userIDs = append(userIDs, id)
		}
		if err := rows.Err(); err != nil {
			glog.Fatal(err)
		}
	}

	var failed int
	for _, uid := range userIDs {
		logPrefix := fmt.Sprintf("[update_statistics user=%d]", uid)
		if err := a.UpdateStatisticsForUser(logPrefix, uid); err != nil {
			glog.Errorf("%s %v", logPrefix, err)
			failed++
			continue
		}
		glog.Infof("%s ok", logPrefix)
	}

	if failed > 0 {
		os.Exit(1)
	}
}
