package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

// TestMigrationsAreRerunSafe enforces invariant 1 in docs/schema.md: a
// migration that fails partway is not recorded and re-runs from the top next
// startup, so every statement must be a no-op when re-applied. Re-applies each
// migration body to an already-migrated schema and fails on the first error.
// Versions 1-14 are excluded: the runner records them without ever running
// them, and they target tables that no longer exist under those names.
func TestMigrationsAreRerunSafe(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	cfg := *c
	// The _rerun suffix keeps it inside cmd/clean_test_dbs's sweep pattern.
	cfg.MySQLDatabase = c.MySQLDatabase + "_rerun"
	connNoDB := fmt.Sprintf("%s:%s@tcp(%s:%s)/?charset=utf8mb4&parseTime=true", cfg.MySQLUser, cfg.MySQLPass, cfg.MySQLHost, cfg.MySQLPort)
	admin, err := sql.Open("mysql", connNoDB)
	if err != nil {
		t.Fatalf("connect (admin): %v", err)
	}
	if _, err := admin.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", cfg.MySQLDatabase)); err != nil {
		admin.Close()
		t.Fatalf("drop database: %v", err)
	}
	if _, err := admin.Exec(fmt.Sprintf("CREATE DATABASE `%s`", cfg.MySQLDatabase)); err != nil {
		admin.Close()
		t.Fatalf("create database: %v", err)
	}
	defer func() {
		_, _ = admin.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", cfg.MySQLDatabase))
		admin.Close()
	}()

	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true", cfg.MySQLUser, cfg.MySQLPass, cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDatabase)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("first pass: %v", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("reading migrations dir: %v", err)
	}
	var versions []int
	for _, e := range entries {
		n, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".sql"))
		if err != nil || n <= 14 {
			continue
		}
		versions = append(versions, n)
	}
	sort.Ints(versions)
	if len(versions) == 0 {
		t.Fatal("no migrations found to re-apply")
	}

	for _, v := range versions {
		path := fmt.Sprintf("migrations/%d.sql", v)
		body, err := migrationsFS.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if err := runOne(db, string(body)); err != nil {
			t.Errorf("%s is not re-run-safe: %v", path, err)
		}
	}
}
