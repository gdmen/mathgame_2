// recompute_problem_difficulty walks all rows in the problems table, computes
// the universal difficulty via api.ComputeProblemDifficulty(expression), and
// writes it back to the difficulty column.
//
// Safe to run repeatedly. Idempotent. Use after deploying the universal
// difficulty change to migrate existing problems from their legacy pinned
// values (target_difficulty when generated) to computed values.
//
// Usage:
//
//	./recompute_problem_difficulty -config=conf.json
//	./recompute_problem_difficulty -config=conf.json -dry-run
//	./recompute_problem_difficulty -config=conf.json -limit=100    (test on a slice)
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
	dryRun := flag.Bool("dry-run", false, "don't write; just print what would change")
	limit := flag.Int("limit", 0, "process only this many rows (0 = all)")
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

	query := `SELECT id, expression, difficulty FROM problems ORDER BY id`
	if *limit > 0 {
		query = fmt.Sprintf("%s LIMIT %d", query, *limit)
	}
	rows, err := db.Query(query)
	if err != nil {
		glog.Fatalf("query problems: %v", err)
	}
	defer rows.Close()

	var updated, unchanged, total int
	var deltaSum float64
	// Collect rows first so we can close the query cursor before updating.
	type rec struct {
		id      uint32
		expr    string
		oldDiff float64
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.expr, &r.oldDiff); err != nil {
			glog.Errorf("scan: %v", err)
			continue
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		glog.Fatalf("rows iteration: %v", err)
	}
	rows.Close()

	for _, r := range recs {
		total++
		newDiff := api.ComputeProblemDifficulty(r.expr)
		if fuzzyEqual(newDiff, r.oldDiff) {
			unchanged++
			continue
		}
		delta := newDiff - r.oldDiff
		deltaSum += delta
		if *dryRun {
			fmt.Printf("id=%d expr=%q: %.2f -> %.2f (%+.2f)\n", r.id, r.expr, r.oldDiff, newDiff, delta)
		} else {
			if _, err := db.Exec(`UPDATE problems SET difficulty = ? WHERE id = ?`, newDiff, r.id); err != nil {
				glog.Errorf("update id=%d: %v", r.id, err)
				continue
			}
		}
		updated++
	}

	fmt.Printf("\nRecompute summary:\n")
	fmt.Printf("  total:     %d\n", total)
	fmt.Printf("  updated:   %d\n", updated)
	fmt.Printf("  unchanged: %d\n", unchanged)
	if updated > 0 {
		fmt.Printf("  avg delta: %+.2f\n", deltaSum/float64(updated))
	}
	if *dryRun {
		fmt.Println("\n(dry run; no changes written)")
	}
	os.Exit(0)
}

func fuzzyEqual(a, b float64) bool {
	delta := a - b
	if delta < 0 {
		delta = -delta
	}
	return delta < 0.01
}
