// Package apitest gives tests outside package api a database carrying the api schema.
package apitest

import (
	"database/sql"
	"testing"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/common/testdb"
)

// SetupTestDB creates a throwaway database carrying the api schema and returns an
// open handle plus a cleanup that drops it. label identifies the calling test binary.
// Package api's own tests reach for testdb.Create instead: importing this from an
// in-package test would cycle back through api.
func SetupTestDB(t *testing.T, c *common.Config, label string) (*sql.DB, func()) {
	t.Helper()
	db, cleanup := testdb.Create(t, c, label)
	if err := api.RunMigrations(db); err != nil {
		cleanup()
		t.Fatalf("run migrations: %v", err)
	}
	return db, cleanup
}
