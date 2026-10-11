package models

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"
)

const testDSN = "test_web:pass@/test_snippetbox?parseTime=true&multiStatements=true"

// TestMain creates the test tables once before any integration test runs, and
// drops them again afterwards. With -short no database is needed at all.
func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		os.Exit(m.Run())
	}

	if err := execScript("./testdata/setup.sql"); err != nil {
		fmt.Fprintf(os.Stderr, "models: setting up test database (see db/test_setup.sql): %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	if err := execScript("./testdata/teardown.sql"); err != nil {
		fmt.Fprintf(os.Stderr, "models: tearing down test database: %v\n", err)
		code = 1
	}

	os.Exit(code)
}

// execScript runs a SQL file against the test database. multiStatements=true
// in the DSN lets us execute the whole file in a single db.Exec() call.
func execScript(path string) error {
	db, err := sql.Open("mysql", testDSN)
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

	db, err := sql.Open("mysql", testDSN)
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
