// Package testdb hands a test its own throwaway MySQL database.
package testdb

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

var counter uint64

// Create makes an empty database beside c.MySQLDatabase and returns an open handle
// plus a cleanup that drops it. label separates the test binaries, the counter
// separates the tests within one, so tests can run in parallel. Two concurrent runs
// of the same binary still land on the same names and will clobber each other.
// Names stay under c.MySQLDatabase's prefix so clean_test_dbs sweeps up whatever a
// crashed run left behind.
func Create(t *testing.T, c *common.Config, label string) (*sql.DB, func()) {
	t.Helper()
	name := fmt.Sprintf("%s_%s_%d", c.MySQLDatabase, label, atomic.AddUint64(&counter, 1))
	// Connecting with a non-existent database name fails, so the CREATE goes over a
	// connection that names no database.
	connNoDB := fmt.Sprintf("%s:%s@tcp(%s:%s)/?charset=utf8mb4&parseTime=true", c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort)
	admin, err := sql.Open("mysql", connNoDB)
	if err != nil {
		t.Fatalf("connect (admin): %v", err)
	}
	_, _ = admin.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", name))
	_, err = admin.Exec(fmt.Sprintf("CREATE DATABASE `%s`", name))
	admin.Close()
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	drop := func() {
		dropDB, _ := sql.Open("mysql", connNoDB)
		_, _ = dropDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", name))
		dropDB.Close()
	}
	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true", c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, name)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		drop()
		t.Fatalf("connect: %v", err)
	}
	return db, func() {
		db.Close()
		drop()
	}
}
