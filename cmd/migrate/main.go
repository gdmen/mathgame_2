// migrate applies pending database migrations from server/api/migrations/
// using the same RunMigrations code path as apiserver startup, but as a
// standalone binary you can run in a tmux/screen session.
//
// Use this when a migration is slow enough that it might outlast the
// apiserver's startup window (and crash the service into a restart loop).
// Run it before `systemctl start mathgame-api`:
//
//	tmux
//	bin/migrate
//	# wait for "All pending migrations applied successfully."
//	sudo systemctl start mathgame-api
//
// On error, exits non-zero with a clear message. Migrations recorded as
// applied in schema_migrations are not re-run on subsequent invocations,
// so re-running after a transient failure picks up where it left off.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
)

func init() {
	// 5s TCP keepalive so long-running ALTER/CREATE INDEX statements
	// don't get killed by firewall/proxy idle timeouts. Keep in sync
	// with cmd/apiserver/main.go.
	mysql.RegisterDialContext("tcp", func(ctx context.Context, addr string) (net.Conn, error) {
		d := net.Dialer{KeepAlive: 5 * time.Second}
		return d.DialContext(ctx, "tcp", addr)
	})
}

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON")
	// Send glog output to stderr so the operator sees migration progress.
	flag.Set("logtostderr", "true")
	flag.Set("stderrthreshold", "INFO")
	flag.Parse()

	c, err := common.ReadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	if err := c.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}

	connectStr := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&time_zone=UTC&readTimeout=30m&writeTimeout=30m",
		c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase,
	)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Fprintln(os.Stderr, "Applying pending migrations. Keep this session alive (tmux/screen).")
	if err := api.RunMigrations(db); err != nil {
		fmt.Fprintln(os.Stderr, "migration failed:", err)
		fmt.Fprintln(os.Stderr, "Already-applied migrations are recorded in schema_migrations and will be skipped on retry. Re-run bin/migrate to continue.")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "All pending migrations applied successfully.")
}
