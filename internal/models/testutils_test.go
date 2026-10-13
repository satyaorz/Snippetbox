package models

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"

	dbfiles "github.com/satyaorz/snippetbox/db"
	"github.com/satyaorz/snippetbox/internal/migrate"
)

// testDSN returns the test database DSN. CI can point it elsewhere with
// SNIPPETBOX_TEST_DSN.
func testDSN() string {
	if dsn := os.Getenv("SNIPPETBOX_TEST_DSN"); dsn != "" {
		return dsn
	}
	return "test_web:pass@/test_snippetbox?parseTime=true&multiStatements=true"
}

// TestMain builds the test schema once before any integration test runs, by
// applying the real migrations in db/migrations (so they're tested too), and
// drops it again afterwards. With -short no database is needed at all.
func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		os.Exit(m.Run())
	}

	if err := setupSchema(); err != nil {
		fmt.Fprintf(os.Stderr, "models: setting up test database (see db/test_setup.sql): %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	if err := execScript("./testdata/reset.sql"); err != nil {
		fmt.Fprintf(os.Stderr, "models: tearing down test database: %v\n", err)
		code = 1
	}

	os.Exit(code)
}

func setupSchema() error {
	if err := execScript("./testdata/reset.sql"); err != nil {
		return err
	}

	db, err := sql.Open("mysql", testDSN())
	if err != nil {
		return err
	}
	defer db.Close()

	migrations, err := migrate.Load(dbfiles.Migrations, "migrations")
	if err != nil {
		return err
	}

	_, err = (&migrate.Migrator{DB: db, Migrations: migrations}).Up(context.Background())
	return err
}

// execScript runs a SQL file against the test database. multiStatements=true
// in the DSN lets us execute the whole file in a single db.Exec() call.
func execScript(path string) error {
	db, err := sql.Open("mysql", testDSN())
	if err != nil {
		return err
	}
	defer db.Close()

	script, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	_, err = db.Exec(string(script))
	return err
}

// newTestDB connects to the test_snippetbox database and resets it to the
// seed data, so every test starts from the same state.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("mysql", testDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	script, err := os.ReadFile("./testdata/seed.sql")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(string(script))
	if err != nil {
		t.Fatal(err)
	}

	return db
}
