// clean_test_dbs drops MySQL databases that match the per-test pattern (mathgame_test_1, mathgame_test_2, ...).
// Run from repo root so test_conf.json is found. Intended for "make clean".
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

func main() {
	configPath := flag.String("config", "test_conf.json", "path to test config JSON")
	flag.Parse()
	c, err := common.ReadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read config: %v\n", err)
		os.Exit(1)
	}
	connStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/?charset=utf8mb4", c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort)
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	pattern := c.MySQLDatabase + "_%"
	rows, err := db.Query("SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE ?", pattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list databases: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	var dropped int
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		_, err := db.Exec(fmt.Sprintf("DROP DATABASE `%s`", name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "drop %s: %v\n", name, err)
			continue
		}
		dropped++
		fmt.Printf("dropped %s\n", name)
	}
	if dropped > 0 {
		fmt.Printf("cleaned %d test database(s)\n", dropped)
	}
}
