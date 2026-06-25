package main

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/mathcore"
)

var testDBCounter uint64

// setupRecomputeTestDB creates a unique test database, runs migrations, and
// returns the db handle plus a cleanup function. Mirrors api.setupTestAPI's
// shape but stays in this package to avoid widening api's test surface.
func setupRecomputeTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	dbName := c.MySQLDatabase + "_recompute_" + fmt.Sprintf("%d", atomic.AddUint64(&testDBCounter, 1))

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

func seedProblem(t *testing.T, db *sql.DB, id uint32, expr string, difficulty float64, ver string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO problems (id, problem_type_bitmap, expression, symbolic_expression, answer, difficulty, generator, difficulty_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, 1, expr, "", "0", difficulty, "test-seed", ver,
	)
	if err != nil {
		t.Fatalf("seed problem id=%d: %v", id, err)
	}
}

func getProblemRow(t *testing.T, db *sql.DB, id uint32) (difficulty float64, ver string) {
	t.Helper()
	if err := db.QueryRow(`SELECT difficulty, difficulty_version FROM problems WHERE id = ?`, id).Scan(&difficulty, &ver); err != nil {
		t.Fatalf("fetch problem id=%d: %v", id, err)
	}
	return
}

// TestRecomputeProblemRow_Skipped: row already at current DifficultyVersion
// short-circuits. No recompute, no UPDATE - the (deliberately wrong) stored
// difficulty stays in place, proving we never even called the formula.
func TestRecomputeProblemRow_Skipped(t *testing.T) {
	db, cleanup := setupRecomputeTestDB(t)
	defer cleanup()

	// Sentinel difficulty that ComputeProblemDifficulty would never produce
	// (clamps at 20). If the fast-path is broken, the sentinel gets clobbered.
	const sentinel = 99.0
	seedProblem(t, db, 12345, "1 + 1", sentinel, mathcore.DifficultyVersion)

	action, newDiff, err := recomputeProblemRow(db, 12345, "1 + 1", "", sentinel, mathcore.DifficultyVersion, false)
	if err != nil {
		t.Fatalf("recomputeProblemRow: %v", err)
	}
	if action != actionSkipped {
		t.Errorf("action = %q, want %q", action, actionSkipped)
	}
	if newDiff != 0 {
		t.Errorf("newDiff = %g, want 0 (skipped path shouldn't recompute)", newDiff)
	}

	gotDiff, _ := getProblemRow(t, db, 12345)
	if gotDiff != sentinel {
		t.Errorf("stored Difficulty = %g, want %g (skip path should not have written)", gotDiff, sentinel)
	}
}

// TestRecomputeProblemRow_Stamped: row's stored difficulty already matches
// the current formula but the version stamp is missing/stale. Forward-stamp
// the version; leave the difficulty value alone.
func TestRecomputeProblemRow_Stamped(t *testing.T) {
	db, cleanup := setupRecomputeTestDB(t)
	defer cleanup()

	expr := "2 + 3"
	correctDiff := mathcore.ComputeProblemDifficulty(expr, "")
	seedProblem(t, db, 23456, expr, correctDiff, "")

	action, _, err := recomputeProblemRow(db, 23456, expr, "", correctDiff, "", false)
	if err != nil {
		t.Fatalf("recomputeProblemRow: %v", err)
	}
	if action != actionStamped {
		t.Errorf("action = %q, want %q", action, actionStamped)
	}

	gotDiff, gotVer := getProblemRow(t, db, 23456)
	if gotVer != mathcore.DifficultyVersion {
		t.Errorf("stored DifficultyVersion = %q, want %q (stamp path should have written version)", gotVer, mathcore.DifficultyVersion)
	}
	// FLOAT column truncates float64 on round-trip; fuzzy compare is correct.
	if !recomputeFuzzyEqual(gotDiff, correctDiff) {
		t.Errorf("stored Difficulty = %g, want %g (stamp path should not have changed value)", gotDiff, correctDiff)
	}
}

// TestRecomputeProblemRow_Updated: row's stored difficulty diverges from the
// current formula. Rewrite both columns.
func TestRecomputeProblemRow_Updated(t *testing.T) {
	db, cleanup := setupRecomputeTestDB(t)
	defer cleanup()

	expr := "4 + 7"
	const wrongDiff = 99.0
	seedProblem(t, db, 34567, expr, wrongDiff, "")
	want := mathcore.ComputeProblemDifficulty(expr, "")

	action, newDiff, err := recomputeProblemRow(db, 34567, expr, "", wrongDiff, "", false)
	if err != nil {
		t.Fatalf("recomputeProblemRow: %v", err)
	}
	if action != actionUpdated {
		t.Errorf("action = %q, want %q", action, actionUpdated)
	}
	if newDiff != want {
		t.Errorf("returned newDiff = %g, want %g", newDiff, want)
	}

	gotDiff, gotVer := getProblemRow(t, db, 34567)
	if gotVer != mathcore.DifficultyVersion {
		t.Errorf("stored DifficultyVersion = %q, want %q", gotVer, mathcore.DifficultyVersion)
	}
	if !recomputeFuzzyEqual(gotDiff, want) {
		t.Errorf("stored Difficulty = %g, want %g", gotDiff, want)
	}
}

// TestRecomputeProblemRow_DryRun_Stamped: dry-run on a row that would be
// stamped (value matches, version stale) must not write the version stamp.
func TestRecomputeProblemRow_DryRun_Stamped(t *testing.T) {
	db, cleanup := setupRecomputeTestDB(t)
	defer cleanup()

	expr := "6 + 5"
	correctDiff := mathcore.ComputeProblemDifficulty(expr, "")
	seedProblem(t, db, 56789, expr, correctDiff, "")

	action, _, err := recomputeProblemRow(db, 56789, expr, "", correctDiff, "", true)
	if err != nil {
		t.Fatalf("recomputeProblemRow: %v", err)
	}
	if action != actionStamped {
		t.Errorf("action = %q, want %q", action, actionStamped)
	}

	_, gotVer := getProblemRow(t, db, 56789)
	if gotVer != "" {
		t.Errorf("stored DifficultyVersion = %q, want %q (dry-run must not write)", gotVer, "")
	}
}

// TestRecomputeProblemRow_DryRun: dry-run must NOT write to the DB,
// regardless of which action the row would normally take.
func TestRecomputeProblemRow_DryRun(t *testing.T) {
	db, cleanup := setupRecomputeTestDB(t)
	defer cleanup()

	expr := "9 + 1"
	const wrongDiff = 99.0
	seedProblem(t, db, 45678, expr, wrongDiff, "")

	action, _, err := recomputeProblemRow(db, 45678, expr, "", wrongDiff, "", true)
	if err != nil {
		t.Fatalf("recomputeProblemRow: %v", err)
	}
	if action != actionUpdated {
		t.Errorf("action = %q, want %q (should be classified the same regardless of dry-run)", action, actionUpdated)
	}

	gotDiff, gotVer := getProblemRow(t, db, 45678)
	if gotDiff != wrongDiff {
		t.Errorf("stored Difficulty = %g, want %g (dry-run must not write)", gotDiff, wrongDiff)
	}
	if gotVer != "" {
		t.Errorf("stored DifficultyVersion = %q, want %q (dry-run must not write)", gotVer, "")
	}
}
