package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

// TestFreshBootstrapProductionOrder boots a brand-new database in the order
// cmd/apiserver/main.go uses: RunMigrations first, then NewApi. setupTestAPI
// runs NewApi first, so only this test covers the production startup path.
func TestFreshBootstrapProductionOrder(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	cfg := *c
	// The _freshboot suffix keeps it inside cmd/clean_test_dbs's sweep pattern.
	cfg.MySQLDatabase = c.MySQLDatabase + "_freshboot"
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
		t.Fatalf("RunMigrations on fresh database: %v", err)
	}
	if _, err := NewApi(db, &cfg); err != nil {
		t.Fatalf("NewApi after migrations: %v", err)
	}
}
