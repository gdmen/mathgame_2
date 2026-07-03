// migrate_division_notation rewrites the pre-obelus spaced-slash division
// notation (" / ") to the obelus (" ÷ ") in stored rows, so the fraction-only
// lexer reads them correctly (a bare slash is now always a fraction). One-time
// and re-run-safe.
//
// It is purely notational: " / " and " ÷ " normalize to the same division
// operator, so value, detected bits, and difficulty are unchanged — no
// recompute_* run and no DifficultyVersion bump. Run it during the obelus
// deploy, BEFORE the server serves or any recompute_* / admin page re-lexes a
// row (an un-migrated spaced slash would otherwise mis-lex as a fraction).
//
//   - non-WORD rows: rewrite `expression` (the symbolic form).
//   - WORD rows:     rewrite `symbolic_expression`; the `\text{}` prose in
//     `expression` is left alone (its slashes are prose).
//
// Unspaced fraction literals (`3/8`) carry no surrounding spaces, so the
// replacement never touches them — only the division operator is spaced.
//
// Part of the problem-generation system - documented in docs/problem-generation.md.
//
// Usage:
//
//	./migrate_division_notation -config=conf.json -dry-run   (report only)
//	./migrate_division_notation -config=conf.json
//	./migrate_division_notation -config=conf.json -limit=100
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/mathcore"
)

const (
	spacedSlash = " / "
	obelus      = " ÷ "
)

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON")
	dryRun := flag.Bool("dry-run", false, "don't write; report what would change")
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

	query := `SELECT id, expression, symbolic_expression, problem_type_bitmap FROM problems ORDER BY id`
	if *limit > 0 {
		query = fmt.Sprintf("%s LIMIT %d", query, *limit)
	}
	rows, err := db.Query(query)
	if err != nil {
		glog.Fatalf("query problems: %v", err)
	}

	type rec struct {
		id       uint32
		expr     string
		symbolic sql.NullString
		bitmap   uint64
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.expr, &r.symbolic, &r.bitmap); err != nil {
			glog.Errorf("scan: %v", err)
			continue
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		glog.Fatalf("rows iteration: %v", err)
	}
	rows.Close()

	var total, wordUpdated, nonWordUpdated int
	for _, r := range recs {
		total++
		isWord := r.bitmap&uint64(mathcore.WORD) != 0

		var col, oldVal, newVal string
		if isWord {
			// WORD: division lives in the symbolic form; the prose is left alone.
			if !r.symbolic.Valid {
				continue
			}
			col, oldVal = "symbolic_expression", r.symbolic.String
		} else {
			col, oldVal = "expression", r.expr
		}
		newVal = strings.ReplaceAll(oldVal, spacedSlash, obelus)
		if newVal == oldVal {
			continue
		}

		if isWord {
			wordUpdated++
		} else {
			nonWordUpdated++
		}
		if *dryRun {
			fmt.Printf("DRY id=%d %s %q -> %q\n", r.id, col, oldVal, newVal)
			continue
		}
		if _, err := db.Exec(
			fmt.Sprintf("UPDATE problems SET %s = ? WHERE id = ?", col), newVal, r.id,
		); err != nil {
			glog.Errorf("update id=%d: %v", r.id, err)
		}
	}

	fmt.Printf("\n=== migrate_division_notation report ===\n")
	fmt.Printf("total=%d non_word_expression_updated=%d word_symbolic_updated=%d dry_run=%v\n",
		total, nonWordUpdated, wordUpdated, *dryRun)
	os.Exit(0)
}
