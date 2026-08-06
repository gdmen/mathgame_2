package main

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/api"
)

// problemRow is the slice of a problems row the cull decision needs.
type problemRow struct {
	id         uint32
	bitmap     uint64
	difficulty float64
	generator  string
	status     string
}

// poolCell groups active rows the same way the admin bitmap matrix does:
// exact stamped bitmap × rounded difficulty bucket. Serving pools draw from
// unions of these cells across a ±1.5 window, so capping per cell over-keeps
// relative to any real pool — conservative by construction.
type poolCell struct {
	bitmap uint64
	bucket int
}

// loadProblems reads the cull-relevant columns for every problem, optionally
// scoped to a generator list.
func loadProblems(db *sql.DB, generators []string) ([]problemRow, error) {
	query := `SELECT id, problem_type_bitmap, difficulty, generator, status FROM problems`
	if len(generators) > 0 {
		quoted := make([]string, len(generators))
		for i, g := range generators {
			quoted[i] = "'" + strings.ReplaceAll(g, "'", "''") + "'"
		}
		query += ` WHERE generator IN (` + strings.Join(quoted, ",") + `)`
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("load problems: %w", err)
	}
	defer rows.Close()

	var out []problemRow
	for rows.Next() {
		var p problemRow
		if err := rows.Scan(&p.id, &p.bitmap, &p.difficulty, &p.generator, &p.status); err != nil {
			return nil, fmt.Errorf("scan problem: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// loadReferencedProblemIds returns every problem id referenced anywhere.
// There are no DB-level foreign keys, so this is the explicit no-orphan
// guard: a problem may be deleted only if it appears in NONE of these.
//
// The events scan is restricted to the problem-id-carrying event types —
// other types hold durations, answers, or video ids in value, and a blanket
// CAST would false-match plain numbers. The dual-format extraction (bare
// integer or JSON {problem_id: N}) matches migration 46: bad_problem_*
// values exist in both shapes; JSON_VALID guards malformed values into NULL
// instead of an error.
//
// The reference tables are events, gamestates, review_queue, and
// recently_shown_problems — every current table that carries a problem_id.
// (statistics_hardest_aggregates was created in migration 16 and dropped in
// 17; the current statistics tables key on user/month, not problem.)
func loadReferencedProblemIds(db *sql.DB) (map[uint32]bool, error) {
	problemIDEventTypes := []string{api.SELECTED_PROBLEM, api.SOLVED_PROBLEM, api.BAD_PROBLEM_SYSTEM, api.BAD_PROBLEM_USER}
	quoted := make([]string, len(problemIDEventTypes))
	for i, et := range problemIDEventTypes {
		quoted[i] = "'" + et + "'"
	}
	queries := []string{
		`SELECT DISTINCT CASE
			WHEN value REGEXP '^[0-9]+$' THEN CAST(value AS UNSIGNED)
			WHEN JSON_VALID(value) THEN CAST(JSON_EXTRACT(value, '$.problem_id') AS UNSIGNED)
			ELSE NULL END
		 FROM events WHERE event_type IN (` + strings.Join(quoted, ",") + `)`,
		`SELECT DISTINCT problem_id FROM gamestates`,
		`SELECT DISTINCT problem_id FROM review_queue`,
		`SELECT DISTINCT problem_id FROM recently_shown_problems`,
	}

	ref := map[uint32]bool{}
	for _, q := range queries {
		if err := scanIdsInto(db, q, ref); err != nil {
			return nil, err
		}
	}
	return ref, nil
}

func scanIdsInto(db *sql.DB, query string, into map[uint32]bool) error {
	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("referenced-ids query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan referenced id: %w", err)
		}
		if id.Valid && id.Int64 > 0 && id.Int64 <= math.MaxUint32 {
			into[uint32(id.Int64)] = true
		}
	}
	return rows.Err()
}

// cullCandidates applies the two-rule policy to never-referenced rows:
//   - rows whose status is in cullStatuses are dead inventory: cull them all.
//     Defaults to just `deprecated` — the retired generators nothing serves.
//     `reported`/`incorrect` are excluded by default: a reported row is a live
//     bad-problem claim (usually still referenced by its event anyway) and an
//     incorrect row is an admin decision worth keeping for analysis.
//   - active rows are a paid-for pool: cull only the excess past keepPerCell
//     per (bitmap, difficulty-bucket) cell, chosen uniformly at random. Active
//     rows never take the outright-cull path even if 'active' is passed in
//     cullStatuses — the per-cell floor always protects the serving pool.
//
// Referenced rows are never candidates, whatever their status.
func cullCandidates(problems []problemRow, referenced map[uint32]bool, cullStatuses map[string]bool, keepPerCell int, rng *rand.Rand) []problemRow {
	var out []problemRow
	activeByCell := map[poolCell][]problemRow{}
	for _, p := range problems {
		if referenced[p.id] {
			continue
		}
		if p.status == api.StatusActive {
			cell := poolCell{p.bitmap, int(math.Round(p.difficulty))}
			activeByCell[cell] = append(activeByCell[cell], p)
			continue
		}
		if cullStatuses[p.status] {
			out = append(out, p)
		}
	}
	for _, rows := range activeByCell {
		if len(rows) <= keepPerCell {
			continue
		}
		rng.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
		out = append(out, rows[keepPerCell:]...)
	}
	return out
}

// deleteProblems deletes ids in batches. A failed batch is logged and
// skipped, not fatal (the trim-job precedent) — a partial cull is safe
// because every candidate is independently unreferenced.
func deleteProblems(db *sql.DB, ids []uint32, batchSize int) int {
	deleted := 0
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		parts := make([]string, len(batch))
		for i, id := range batch {
			parts[i] = fmt.Sprint(id)
		}
		res, err := db.Exec(`DELETE FROM problems WHERE id IN (` + strings.Join(parts, ",") + `)`)
		if err != nil {
			glog.Errorf("delete batch [%d:%d]: %v (continuing)", start, end, err)
			continue
		}
		n, _ := res.RowsAffected()
		deleted += int(n)
	}
	return deleted
}

type cleanupOptions struct {
	generators   []string        // empty = all generators
	cullStatuses map[string]bool // non-active statuses culled outright
	keepPerCell  int
	limit        int // 0 = no limit
	batchSize    int
	apply        bool
	rng          *rand.Rand
}

type cleanupReport struct {
	scanned    int
	candidates []problemRow
	deleted    int
}

// countsByGeneratorStatus aggregates candidates for the dry-run report.
func (r *cleanupReport) countsByGeneratorStatus() map[string]int {
	counts := map[string]int{}
	for _, p := range r.candidates {
		counts[p.generator+" / "+p.status]++
	}
	return counts
}

// runCleanup orchestrates one cull pass: load → guard → decide → (apply).
// Candidates are sorted by id before the limit and the delete, so a limited
// run is deterministic and resumable.
func runCleanup(db *sql.DB, o cleanupOptions) (*cleanupReport, error) {
	problems, err := loadProblems(db, o.generators)
	if err != nil {
		return nil, err
	}
	referenced, err := loadReferencedProblemIds(db)
	if err != nil {
		return nil, err
	}

	candidates := cullCandidates(problems, referenced, o.cullStatuses, o.keepPerCell, o.rng)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].id < candidates[j].id })
	if o.limit > 0 && len(candidates) > o.limit {
		candidates = candidates[:o.limit]
	}

	report := &cleanupReport{scanned: len(problems), candidates: candidates}
	if o.apply {
		ids := make([]uint32, len(candidates))
		for i, p := range candidates {
			ids[i] = p.id
		}
		report.deleted = deleteProblems(db, ids, o.batchSize)
	}
	return report, nil
}
