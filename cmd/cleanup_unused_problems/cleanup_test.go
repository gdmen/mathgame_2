package main

import (
	"database/sql"
	"fmt"
	"math/rand"
	"sync/atomic"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
)

var testDBCounter uint64

// setupCleanupTestDB creates a unique test database, runs migrations, and
// returns the db handle plus a cleanup function. Mirrors the
// cmd/recompute_problem_difficulty test-DB pattern.
func setupCleanupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	dbName := c.MySQLDatabase + "_cleanup_" + fmt.Sprintf("%d", atomic.AddUint64(&testDBCounter, 1))

	connNoDB := fmt.Sprintf("%s:%s@tcp(%s:%s)/?charset=utf8mb4&parseTime=true", c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort)
	dbAdmin, err := sql.Open("mysql", connNoDB)
	if err != nil {
		t.Fatalf("connect (admin): %v", err)
	}
	_, _ = dbAdmin.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName))
	_, err = dbAdmin.Exec(fmt.Sprintf("CREATE DATABASE `%s`", dbName))
	dbAdmin.Close()
	if err != nil {
		t.Fatalf("create database: %v", err)
	}

	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true", c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, dbName)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	// NewApi runs the codegen createXTableSQL for each model; needed before
	// migrations 15+ can ALTER those tables.
	if _, err := api.NewApi(db, c); err != nil {
		db.Close()
		t.Fatalf("NewApi: %v", err)
	}
	if err := api.RunMigrations(db); err != nil {
		db.Close()
		t.Fatalf("run migrations: %v", err)
	}
	return db, func() {
		db.Close()
		dropDB, _ := sql.Open("mysql", connNoDB)
		_, _ = dropDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName))
		dropDB.Close()
	}
}

func seedProblemRow(t *testing.T, db *sql.DB, p problemRow) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO problems (id, problem_type_bitmap, expression, symbolic_expression, answer, explanation, difficulty, status, generator, difficulty_version)
		 VALUES (?, ?, ?, '', '1', '', ?, ?, ?, 'test')`,
		p.id, p.bitmap, fmt.Sprintf("seed %d", p.id), p.difficulty, p.status, p.generator)
	if err != nil {
		t.Fatalf("seed problem %d: %v", p.id, err)
	}
}

func seedEvent(t *testing.T, db *sql.DB, eventType, value string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO events (user_id, event_type, value) VALUES (1, ?, ?)`, eventType, value); err != nil {
		t.Fatalf("seed event %s=%q: %v", eventType, value, err)
	}
}

// cullDeprecated is the default outright-cull scope used by most tests.
var cullDeprecated = map[string]bool{"deprecated": true}

// TestCullCandidates pins the two-rule policy on never-referenced rows:
// rows in cullStatuses are culled outright; active rows are culled only past
// keepPerCell per (bitmap, rounded-difficulty) cell. Referenced rows are
// never candidates.
func TestCullCandidates(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	problems := []problemRow{
		// Never-referenced deprecated: culled.
		{id: 1, bitmap: 1, difficulty: 5, generator: "llm_0.1", status: "deprecated"},
		{id: 2, bitmap: 1, difficulty: 5, generator: "llm_0.1", status: "deprecated"},
		// Referenced deprecated: kept.
		{id: 3, bitmap: 1, difficulty: 5, generator: "llm_0.1", status: "deprecated"},
		// Referenced active: kept, and does NOT count toward the cell cap.
		{id: 4, bitmap: 1, difficulty: 5, generator: "llm_0.6", status: "active"},
		// Never-referenced reported: kept by default (not in cullStatuses).
		{id: 5, bitmap: 1, difficulty: 5, generator: "llm_0.1", status: "reported"},
	}
	// One active cell (bitmap=2, bucket=7) with 5 never-referenced rows and
	// keepPerCell=3: exactly 2 are culled.
	for i := uint32(10); i < 15; i++ {
		problems = append(problems, problemRow{id: i, bitmap: 2, difficulty: 7.2, generator: "heuristic_2.0", status: "active"})
	}
	// A different bucket of the same bitmap is its own cell: under the cap,
	// untouched.
	problems = append(problems,
		problemRow{id: 20, bitmap: 2, difficulty: 9, generator: "heuristic_2.0", status: "active"},
		problemRow{id: 21, bitmap: 2, difficulty: 9, generator: "heuristic_2.0", status: "active"},
	)

	referenced := map[uint32]bool{3: true, 4: true}
	got := cullCandidates(problems, referenced, cullDeprecated, 3, rng)

	gotIds := map[uint32]bool{}
	for _, p := range got {
		gotIds[p.id] = true
	}
	for _, want := range []uint32{1, 2} {
		if !gotIds[want] {
			t.Errorf("never-referenced deprecated id=%d missing from candidates", want)
		}
	}
	for _, keep := range []uint32{3, 4, 5, 20, 21} {
		if gotIds[keep] {
			t.Errorf("id=%d must be kept (referenced, reported, or under cell cap), but is a candidate", keep)
		}
	}
	overCap := 0
	for i := uint32(10); i < 15; i++ {
		if gotIds[i] {
			overCap++
		}
	}
	if overCap != 2 {
		t.Errorf("over-cap active cell: %d culled, want 2 (5 rows, keep 3)", overCap)
	}
	if len(got) != 4 {
		t.Errorf("total candidates = %d, want 4 (2 deprecated + 2 over-cap active)", len(got))
	}
}

// TestCullCandidates_ReportedOptIn: reported rows are culled only when
// explicitly named in cullStatuses; incorrect rows stay kept.
func TestCullCandidates_ReportedOptIn(t *testing.T) {
	problems := []problemRow{
		{id: 1, bitmap: 1, difficulty: 5, generator: "llm_0.1", status: "reported"},
		{id: 2, bitmap: 1, difficulty: 5, generator: "llm_0.1", status: "incorrect"},
	}
	got := cullCandidates(problems, map[uint32]bool{}, map[string]bool{"reported": true}, 3, rand.New(rand.NewSource(1)))
	if len(got) != 1 || got[0].id != 1 {
		t.Errorf("reported opt-in: got %v, want only id=1 (incorrect stays kept)", got)
	}
}

// TestLoadProblems_GeneratorScope: the generator filter restricts which rows
// the SQL returns.
func TestLoadProblems_GeneratorScope(t *testing.T) {
	db, cleanup := setupCleanupTestDB(t)
	defer cleanup()

	seedProblemRow(t, db, problemRow{id: 1, bitmap: 1, difficulty: 5, generator: "llm_0.1", status: "deprecated"})
	seedProblemRow(t, db, problemRow{id: 2, bitmap: 1, difficulty: 5, generator: "llm_0.2", status: "deprecated"})

	got, err := loadProblems(db, []string{"llm_0.1"})
	if err != nil {
		t.Fatalf("loadProblems: %v", err)
	}
	if len(got) != 1 || got[0].id != 1 {
		t.Errorf("generator scope: got %v, want only id=1", got)
	}
}

// TestLoadReferencedProblemIds: every reference source protects its id; the
// dual-format events extraction handles bare ints and JSON; non-problem
// event types never protect (a duration that collides with a problem id must
// not count); malformed values are skipped, not fatal.
func TestLoadReferencedProblemIds(t *testing.T) {
	db, cleanup := setupCleanupTestDB(t)
	defer cleanup()

	seedEvent(t, db, api.SELECTED_PROBLEM, "101")
	seedEvent(t, db, api.SOLVED_PROBLEM, "102")
	seedEvent(t, db, api.BAD_PROBLEM_USER, `{"problem_id": 103, "note": "confusing"}`)
	seedEvent(t, db, api.BAD_PROBLEM_SYSTEM, "104")
	seedEvent(t, db, api.BAD_PROBLEM_USER, "not-a-number") // malformed: skipped
	// A working_on_problem duration numerically equal to a problem id must
	// NOT protect that id (its value is milliseconds, not a problem id).
	seedEvent(t, db, api.WORKING_ON_PROBLEM, "105")

	if _, err := db.Exec(`INSERT INTO gamestates (user_id, problem_id, video_id, solved, target) VALUES (1, 201, 0, 0, 5)`); err != nil {
		t.Fatalf("seed gamestates: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO review_queue (user_id, problem_id) VALUES (1, 202)`); err != nil {
		t.Fatalf("seed review_queue: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO recently_shown_problems (user_id, problem_id, shown_at) VALUES (1, 203, NOW())`); err != nil {
		t.Fatalf("seed recently_shown_problems: %v", err)
	}

	ref, err := loadReferencedProblemIds(db)
	if err != nil {
		t.Fatalf("loadReferencedProblemIds: %v", err)
	}
	for _, want := range []uint32{101, 102, 103, 104, 201, 202, 203} {
		if !ref[want] {
			t.Errorf("id=%d should be referenced", want)
		}
	}
	if ref[105] {
		t.Errorf("id=105 protected by a working_on_problem duration; non-problem event types must not count")
	}
}

func countProblems(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM problems`).Scan(&n); err != nil {
		t.Fatalf("count problems: %v", err)
	}
	return n
}

// TestRunCleanup: dry-run reports candidates and deletes nothing; apply
// deletes exactly the candidates (across multiple batches) and leaves
// referenced rows.
func TestRunCleanup(t *testing.T) {
	db, cleanup := setupCleanupTestDB(t)
	defer cleanup()

	// 5 never-referenced deprecated rows + 1 referenced deprecated + 1 active.
	for i := uint32(1); i <= 5; i++ {
		seedProblemRow(t, db, problemRow{id: i, bitmap: 1, difficulty: 5, generator: "llm_0.1", status: "deprecated"})
	}
	seedProblemRow(t, db, problemRow{id: 6, bitmap: 1, difficulty: 5, generator: "llm_0.1", status: "deprecated"})
	seedEvent(t, db, api.SELECTED_PROBLEM, "6")
	seedProblemRow(t, db, problemRow{id: 7, bitmap: 1, difficulty: 5, generator: "llm_0.6", status: "active"})

	// Dry run: full report, no writes.
	report, err := runCleanup(db, cleanupOptions{cullStatuses: cullDeprecated, keepPerCell: 200, batchSize: 2, rng: rand.New(rand.NewSource(1))})
	if err != nil {
		t.Fatalf("runCleanup dry-run: %v", err)
	}
	if len(report.candidates) != 5 {
		t.Errorf("dry-run candidates = %d, want 5", len(report.candidates))
	}
	if report.deleted != 0 {
		t.Errorf("dry-run deleted = %d, want 0", report.deleted)
	}
	if got := countProblems(t, db); got != 7 {
		t.Errorf("dry-run wrote: %d rows remain, want 7", got)
	}

	// Apply with batchSize=2 (forces 3 batches for 5 ids).
	report, err = runCleanup(db, cleanupOptions{cullStatuses: cullDeprecated, keepPerCell: 200, batchSize: 2, apply: true, rng: rand.New(rand.NewSource(1))})
	if err != nil {
		t.Fatalf("runCleanup apply: %v", err)
	}
	if report.deleted != 5 {
		t.Errorf("apply deleted = %d, want 5", report.deleted)
	}
	if got := countProblems(t, db); got != 2 {
		t.Errorf("after apply: %d rows remain, want 2 (referenced + active)", got)
	}
	var stillThere int
	if err := db.QueryRow(`SELECT COUNT(*) FROM problems WHERE id IN (6, 7)`).Scan(&stillThere); err != nil {
		t.Fatalf("check kept rows: %v", err)
	}
	if stillThere != 2 {
		t.Errorf("kept rows = %d, want 2 (ids 6 and 7)", stillThere)
	}
}

// TestRunCleanup_Limit: -limit caps how many candidates are deleted, lowest
// ids first (deterministic).
func TestRunCleanup_Limit(t *testing.T) {
	db, cleanup := setupCleanupTestDB(t)
	defer cleanup()

	for i := uint32(1); i <= 4; i++ {
		seedProblemRow(t, db, problemRow{id: i, bitmap: 1, difficulty: 5, generator: "llm_0.1", status: "deprecated"})
	}

	report, err := runCleanup(db, cleanupOptions{cullStatuses: cullDeprecated, keepPerCell: 200, batchSize: 100, limit: 2, apply: true, rng: rand.New(rand.NewSource(1))})
	if err != nil {
		t.Fatalf("runCleanup: %v", err)
	}
	if report.deleted != 2 {
		t.Errorf("deleted = %d, want 2 (limit)", report.deleted)
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM problems WHERE id IN (3, 4)`).Scan(&remaining); err != nil {
		t.Fatalf("check remaining: %v", err)
	}
	if remaining != 2 {
		t.Errorf("limit should keep the highest ids: ids 3,4 remaining = %d, want 2", remaining)
	}
}
