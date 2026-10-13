// Package migrate applies numbered SQL migration files to a MySQL database
// and records which ones have run, in the spirit of tools like goose and
// golang-migrate but small enough to read in one sitting.
//
// Migration files are named NNNN_description.sql and applied in version
// order. Each file is executed as a single multi-statement Exec, so the DSN
// must include multiStatements=true. Note that MySQL can't roll back schema
// changes (DDL commits implicitly), so a migration that fails part-way can
// leave the database partially migrated and needs fixing by hand.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"time"
)

// Migration is one migration file.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// String returns the migration's file name, without the .sql extension.
func (m Migration) String() string {
	return fmt.Sprintf("%04d_%s", m.Version, m.Name)
}

var fileRX = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.sql$`)

// Load reads every *.sql file in dir within fsys, sorted by version.
func Load(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}

	var migrations []Migration
	seen := map[int]string{}

	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".sql" {
			continue
		}

		m := fileRX.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("migrate: %s: file name must look like 0001_description.sql", e.Name())
		}

		version, err := strconv.Atoi(m[1])
		if err != nil || version < 1 {
			return nil, fmt.Errorf("migrate: %s: invalid version", e.Name())
		}
		if other, ok := seen[version]; ok {
			return nil, fmt.Errorf("migrate: %s and %s have the same version", other, e.Name())
		}
		seen[version] = e.Name()

		content, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, Migration{Version: version, Name: m[2], SQL: string(content)})
	}

	slices.SortFunc(migrations, func(a, b Migration) int { return a.Version - b.Version })

	return migrations, nil
}

// Migrator applies Migrations to DB, tracking them in Table (by default
// "schema_migrations").
type Migrator struct {
	DB         *sql.DB
	Migrations []Migration
	Table      string
}

// Status describes whether one migration has been applied.
type Status struct {
	Migration
	Applied   bool
	AppliedAt time.Time
}

var identRX = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,55}$`)

func (m *Migrator) table() (string, error) {
	t := m.Table
	if t == "" {
		t = "schema_migrations"
	}
	// The table name is interpolated into SQL, so only allow safe names. (The
	// length limit keeps "migrate:<table>" within MySQL's 64-character limit
	// for lock names.)
	if !identRX.MatchString(t) {
		return "", fmt.Errorf("migrate: invalid table name %q", t)
	}
	return t, nil
}

// withLock runs fn on a single connection while holding a MySQL named lock,
// so two copies of the migrator (say, two deploys) can't run at once.
func (m *Migrator) withLock(ctx context.Context, fn func(conn *sql.Conn, table string) error) error {
	table, err := m.table()
	if err != nil {
		return err
	}

	conn, err := m.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	lockName := "migrate:" + table

	var got sql.NullInt64
	err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 30)", lockName).Scan(&got)
	if err != nil {
		return err
	}
	if got.Int64 != 1 {
		return errors.New("migrate: timed out waiting for another migration to finish")
	}
	defer conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockName)

	_, err = conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+table+` (
		version INTEGER NOT NULL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		applied_at DATETIME NOT NULL
	)`)
	if err != nil {
		return err
	}

	return fn(conn, table)
}

func applied(ctx context.Context, conn *sql.Conn, table string) (map[int]time.Time, error) {
	rows, err := conn.QueryContext(ctx, "SELECT version, applied_at FROM "+table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := map[int]time.Time{}
	for rows.Next() {
		var v int
		var at time.Time
		if err := rows.Scan(&v, &at); err != nil {
			return nil, err
		}
		versions[v] = at
	}
	return versions, rows.Err()
}

func record(ctx context.Context, conn *sql.Conn, table string, mig Migration) error {
	_, err := conn.ExecContext(ctx, "INSERT INTO "+table+" (version, name, applied_at) VALUES (?, ?, UTC_TIMESTAMP())", mig.Version, mig.Name)
	return err
}

// Up applies every migration that hasn't been applied yet, in order, and
// returns the ones it ran.
func (m *Migrator) Up(ctx context.Context) ([]Migration, error) {
	var ran []Migration

	err := m.withLock(ctx, func(conn *sql.Conn, table string) error {
		done, err := applied(ctx, conn, table)
		if err != nil {
			return err
		}

		highest := 0
		for v := range done {
			highest = max(highest, v)
		}

		for _, mig := range m.Migrations {
			if _, ok := done[mig.Version]; ok {
				continue
			}
			// A pending migration older than one that's already applied
			// usually means two branches added migrations in parallel. Its
			// SQL was written against an older schema, so refuse to guess.
			if mig.Version < highest {
				return fmt.Errorf("migrate: %s is pending but newer migration %04d is already applied", mig, highest)
			}

			_, err := conn.ExecContext(ctx, mig.SQL)
			if err != nil {
				return fmt.Errorf("migrate: %s failed (the database may be partially migrated): %w", mig, err)
			}
			if err := record(ctx, conn, table, mig); err != nil {
				return err
			}
			ran = append(ran, mig)
		}
		return nil
	})

	return ran, err
}

// Baseline marks every migration up to and including version as applied
// without running it. Use it once on a database whose schema was created
// before migrations were tracked.
func (m *Migrator) Baseline(ctx context.Context, version int) ([]Migration, error) {
	if !slices.ContainsFunc(m.Migrations, func(mig Migration) bool { return mig.Version == version }) {
		return nil, fmt.Errorf("migrate: no migration with version %d", version)
	}

	var marked []Migration

	err := m.withLock(ctx, func(conn *sql.Conn, table string) error {
		done, err := applied(ctx, conn, table)
		if err != nil {
			return err
		}

		for _, mig := range m.Migrations {
			if _, ok := done[mig.Version]; ok || mig.Version > version {
				continue
			}
			if err := record(ctx, conn, table, mig); err != nil {
				return err
			}
			marked = append(marked, mig)
		}
		return nil
	})

	return marked, err
}

// Status reports which migrations have been applied.
func (m *Migrator) Status(ctx context.Context) ([]Status, error) {
	var statuses []Status

	err := m.withLock(ctx, func(conn *sql.Conn, table string) error {
		done, err := applied(ctx, conn, table)
		if err != nil {
			return err
		}

		for _, mig := range m.Migrations {
			at, ok := done[mig.Version]
			statuses = append(statuses, Status{Migration: mig, Applied: ok, AppliedAt: at})
		}
		return nil
	})

	return statuses, err
}
