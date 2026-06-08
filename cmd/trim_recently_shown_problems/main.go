// trim_recently_shown_problems caps each user's row count in
// recently_shown_problems to recentlyShownProblemsTrimSize. Run daily via
// systemd timer.
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
	dryRun := flag.Bool("dry-run", false, "log planned trims without writing")
	flag.Set("logtostderr", "true")
	flag.Set("stderrthreshold", "INFO")
	flag.Parse()

	c, err := common.ReadConfig(*configPath)
	if err != nil {
		glog.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		glog.Fatal(err)
	}

	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&time_zone=UTC",
		c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		glog.Fatal(err)
	}
	defer db.Close()

	users, deleted, err := api.TrimRecentlyShownProblems(db, *dryRun)
	if err != nil {
		glog.Fatal(err)
	}
	if *dryRun {
		fmt.Fprintf(os.Stdout, "dry-run: %d users have rows beyond the cap\n", users)
		return
	}
	fmt.Fprintf(os.Stdout, "trimmed %d users, deleted %d rows\n", users, deleted)
}
