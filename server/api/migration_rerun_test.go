package api // import "garydmenezes.com/mathgame/server/api"

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/common/testdb"
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
	// An empty database, so the first pass exercises the fresh-bootstrap path.
	db, cleanup := testdb.Create(t, c, "rerun")
	defer cleanup()

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
