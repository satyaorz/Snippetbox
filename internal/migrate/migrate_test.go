package migrate

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/satyaorz/snippetbox/internal/assert"

	_ "github.com/go-sql-driver/mysql"
)

func TestLoad(t *testing.T) {
	fsys := fstest.MapFS{
		"m/0002_second.sql": {Data: []byte("SELECT 2;")},
		"m/0001_first.sql":  {Data: []byte("SELECT 1;")},
		"m/README.md":       {Data: []byte("ignored")},
	}

	migrations, err := Load(fsys, "m")
	assert.NilError(t, err)
	assert.Equal(t, len(migrations), 2)
	assert.Equal(t, migrations[0].String(), "0001_first")
	assert.Equal(t, migrations[1].SQL, "SELECT 2;")

	bad := []fstest.MapFS{
		{"m/first.sql": {Data: []byte("")}},
		{"m/0001_Bad-Name.sql": {Data: []byte("")}},
		{"m/0000_zero.sql": {Data: []byte("")}},
		{"m/0001_a.sql": {Data: []byte("")}, "m/001_b.sql": {Data: []byte("")}},
	}
	for _, fsys := range bad {
		_, err := Load(fsys, "m")
		if err == nil {
			t.Errorf("expected an error for %v", fsys)
		}
	}
}

// newTestMigrator returns a Migrator using its own tracking table and widget
// table, so it can share the test database with the models tests (which
// run in parallel in a separate process).
func newTestMigrator(t *testing.T, migrations ...Migration) *Migrator {
	t.Helper()

	if testing.Short() {
		t.Skip("migrate: skipping integration test")
	}

	dsn := os.Getenv("SNIPPETBOX_TEST_DSN")
	if dsn == "" {
		dsn = "test_web:pass@/test_snippetbox?parseTime=true&multiStatements=true"
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS migrate_test_versions, migrate_test_widgets")
	}
	cleanup()
	t.Cleanup(func() {
		cleanup()
		db.Close()
	})

	return &Migrator{DB: db, Migrations: migrations, Table: "migrate_test_versions"}
}

var (
	createWidgets = Migration{1, "create_widgets", "CREATE TABLE migrate_test_widgets (id INTEGER PRIMARY KEY);"}
	addName       = Migration{2, "add_name", "ALTER TABLE migrate_test_widgets ADD COLUMN name VARCHAR(10); INSERT INTO migrate_test_widgets VALUES (1, 'one');"}
	broken        = Migration{3, "broken", "THIS IS NOT SQL;"}
)

func TestUp(t *testing.T) {
	m := newTestMigrator(t, createWidgets, addName)
	ctx := context.Background()

	ran, err := m.Up(ctx)
	assert.NilError(t, err)
	assert.Equal(t, len(ran), 2)

	// Both statements in the multi-statement migration ran.
	var name string
	err = m.DB.QueryRow("SELECT name FROM migrate_test_widgets WHERE id = 1").Scan(&name)
	assert.NilError(t, err)
	assert.Equal(t, name, "one")

	// Running again is a no-op.
	ran, err = m.Up(ctx)
	assert.NilError(t, err)
	assert.Equal(t, len(ran), 0)

	statuses, err := m.Status(ctx)
	assert.NilError(t, err)
	assert.Equal(t, len(statuses), 2)
	assert.Equal(t, statuses[1].Applied, true)
}

func TestUpFailure(t *testing.T) {
	m := newTestMigrator(t, createWidgets, broken)

	ran, err := m.Up(context.Background())
	assert.Equal(t, len(ran), 1)
	assert.StringContains(t, err.Error(), "0003_broken failed")

	// The failed migration isn't recorded, so it will be retried next time.
	statuses, err := m.Status(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, statuses[0].Applied, true)
	assert.Equal(t, statuses[1].Applied, false)
}

func TestUpOutOfOrder(t *testing.T) {
	m := newTestMigrator(t, createWidgets, addName)
	ctx := context.Background()

	// Pretend only migration 2 has been applied.
	m.Migrations = []Migration{addName}
	_, err := m.Baseline(ctx, 2)
	assert.NilError(t, err)

	m.Migrations = []Migration{createWidgets, addName}
	_, err = m.Up(ctx)
	if err == nil || !strings.Contains(err.Error(), "is pending but newer migration") {
		t.Errorf("expected an out-of-order error, got %v", err)
	}
}

func TestBaseline(t *testing.T) {
	m := newTestMigrator(t, createWidgets, addName)
	ctx := context.Background()

	_, err := m.Baseline(ctx, 7)
	assert.StringContains(t, err.Error(), "no migration with version 7")

	// Create the table by hand, as if the schema predates migrations.
	_, err = m.DB.Exec(createWidgets.SQL)
	assert.NilError(t, err)

	marked, err := m.Baseline(ctx, 1)
	assert.NilError(t, err)
	assert.Equal(t, len(marked), 1)

	// Up now only runs migration 2.
	ran, err := m.Up(ctx)
	assert.NilError(t, err)
	assert.Equal(t, len(ran), 1)
	assert.Equal(t, ran[0].Version, 2)
}

func TestInvalidTableName(t *testing.T) {
	m := newTestMigrator(t, createWidgets)
	m.Table = "users; DROP TABLE users"

	_, err := m.Up(context.Background())
	assert.StringContains(t, err.Error(), "invalid table name")
}
