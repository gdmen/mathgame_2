// cleanup_unused_problems deletes problem rows that nothing references.
//
// Policy (two rules, both restricted to rows referenced by NO events /
// gamestates / review_queue / recently_shown_problems row — the no-orphan
// guard):
//
//   - rows whose status is in -status (default `deprecated`) are dead
//     inventory: delete them all. `reported`/`incorrect` are kept unless
//     named.
//   - active rows are a paid-for pool: delete only the excess past
//     -keep-per-cell per (bitmap, rounded-difficulty) cell, chosen at random,
//     so a cull never drops a cell below the selection pool cap.
//
// DRY-RUN IS THE DEFAULT: deletes are irreversible, so this tool inverts the
// house "-dry-run to opt out of writing" convention — pass -apply to delete.
//
// Usage:
//
//	./cleanup_unused_problems -config=conf.json                  (report only)
//	./cleanup_unused_problems -config=conf.json -apply
//	./cleanup_unused_problems -config=conf.json -generator=llm_0.1 -apply
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
)

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON")
	apply := flag.Bool("apply", false, "actually delete; without this, report only")
	generators := flag.String("generator", "", "comma-separated generator scope (empty = all)")
	statuses := flag.String("status", api.StatusDeprecated, "comma-separated non-active statuses culled outright (active is always per-cell)")
	keepPerCell := flag.Int("keep-per-cell", api.SelectionPoolCap, "never-referenced ACTIVE rows kept per (bitmap, difficulty) cell")
	limit := flag.Int("limit", 0, "delete at most this many candidates, lowest ids first (0 = all)")
	batchSize := flag.Int("batch-size", 1000, "ids per DELETE statement")
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

	var gens []string
	if *generators != "" {
		for _, g := range strings.Split(*generators, ",") {
			gens = append(gens, strings.TrimSpace(g))
		}
	}

	cullStatuses := map[string]bool{}
	for _, s := range strings.Split(*statuses, ",") {
		if s = strings.TrimSpace(s); s != "" {
			cullStatuses[s] = true
		}
	}
	if cullStatuses[api.StatusActive] {
		fmt.Fprintf(os.Stderr, "warning: 'active' in -status is ignored; active rows are only ever culled past -keep-per-cell\n")
	}

	report, err := runCleanup(db, cleanupOptions{
		generators:   gens,
		cullStatuses: cullStatuses,
		keepPerCell:  *keepPerCell,
		limit:        *limit,
		batchSize:    *batchSize,
		apply:        *apply,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	})
	if err != nil {
		glog.Fatal(err)
	}

	fmt.Printf("scanned=%d candidates=%d apply=%v\n", report.scanned, len(report.candidates), *apply)

	counts := report.countsByGeneratorStatus()
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("\ncandidates by generator / status:\n")
	for _, k := range keys {
		fmt.Printf("%8d  %s\n", counts[k], k)
	}
	if len(keys) == 0 {
		fmt.Printf("  (none)\n")
	}

	if n := len(report.candidates); n > 0 {
		sample := report.candidates
		if len(sample) > 20 {
			sample = sample[:20]
		}
		fmt.Printf("\nsample candidate ids (spot-check against the four reference tables):\n")
		for _, p := range sample {
			fmt.Printf("  %d (%s, %s, difficulty %.2f)\n", p.id, p.generator, p.status, p.difficulty)
		}
	}

	if *apply {
		fmt.Printf("\ndeleted %d rows\n", report.deleted)
	} else {
		fmt.Printf("\ndry-run: nothing deleted; pass -apply to delete %d rows\n", len(report.candidates))
	}
	os.Exit(0)
}
