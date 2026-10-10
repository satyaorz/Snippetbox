package models

import (
	"database/sql"
	"os"
	"testing"
)

// newTestDB connects to the test_snippetbox database, creates the tables and
// seed data, and registers a cleanup which drops them again after the test.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// multiStatements=true lets us execute the whole setup/teardown script in
	// a single db.Exec() call.
	db, err := sql.Open("mysql", "test_web:pass@/test_snippetbox?parseTime=true&multiStatements=true")
	if err != nil {
		t.Fatal(err)
	}

	script, err := os.ReadFile("./testdata/setup.sql")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	_, err = db.Exec(string(script))
	if err != nil {
		db.Close()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		defer db.Close()

		script, err := os.ReadFile("./testdata/teardown.sql")
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(string(script))
		if err != nil {
			t.Fatal(err)
		}
	})

	return db
}
