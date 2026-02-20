// Package api runs database migrations on startup.
// If you have already run some migrations by hand, insert their version numbers
// into schema_migrations so they are not re-run, e.g.:
//
//	INSERT INTO schema_migrations (version) VALUES ('1'), ('2'), ('3');
package api

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/glog"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const createSchemaMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) NOT NULL PRIMARY KEY
) DEFAULT CHARSET=utf8mb4;`

// RunMigrations creates the schema_migrations table if needed, then runs any
// migration file in server/api/migrations/ that is not yet recorded. Files are
// run in numeric order by base name (1.sql, 2.sql, ..., 10.sql, 11.sql, ...).
// If schema_migrations is empty, versions 1-14 are seeded as already applied
// (all environments had those run manually before this system existed).
func RunMigrations(db *sql.DB) error {
	if _, err := db.Exec(createSchemaMigrationsTable); err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		for v := 1; v <= 14; v++ {
			version := strconv.Itoa(v)
			if _, err := db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
				return fmt.Errorf("seeding migration %s: %w", version, err)
			}
			applied[version] = true
		}
		glog.Info("seeded schema_migrations with versions 1-14 (already applied in all environments)")
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}

	var versions []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(name, ".sql"))
		if err != nil {
			glog.Warningf("skipping migration with non-numeric name: %s", name)
			continue
		}
		versions = append(versions, n)
	}
	sort.Ints(versions)

	for _, v := range versions {
		version := strconv.Itoa(v)
		if applied[version] {
			continue
		}
		path := "migrations/" + version + ".sql"
		body, err := migrationsFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		if err := runOne(db, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", path, err)
		}
		if _, err := db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			return fmt.Errorf("recording migration %s: %w", version, err)
		}
		glog.Infof("migration applied: %s", path)
	}

	return nil
}

func appliedVersions(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("listing applied migrations: %w", err)
	}
	defer rows.Close()
	m := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		m[v] = true
	}
	return m, rows.Err()
}

func runOne(db *sql.DB, body string) error {
	body = strings.TrimSpace(body)
	for _, stmt := range splitStatements(body) {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec: %w", err)
		}
	}
	return nil
}

func splitStatements(body string) []string {
	var out []string
	for _, s := range strings.Split(body, ";") {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t+";")
		}
	}
	return out
}
