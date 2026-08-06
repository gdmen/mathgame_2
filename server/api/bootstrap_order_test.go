package api // import "garydmenezes.com/mathgame/server/api"

import (
	"testing"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/common/testdb"
)

// TestFreshBootstrapProductionOrder boots a brand-new database in the order
// cmd/apiserver/main.go uses: RunMigrations first, then NewApi. setupTestAPI
// runs NewApi first, so only this test covers the production startup path.
func TestFreshBootstrapProductionOrder(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	db, cleanup := testdb.Create(t, c, "freshboot")
	defer cleanup()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations on fresh database: %v", err)
	}
	if _, err := NewApi(db, c); err != nil {
		t.Fatalf("NewApi after migrations: %v", err)
	}
}
