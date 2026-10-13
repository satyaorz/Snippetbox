// Command migrate creates and updates the Snippetbox database schema.
//
//	migrate [-dsn DSN] up                 apply all pending migrations
//	migrate [-dsn DSN] status             list migrations and whether they've run
//	migrate [-dsn DSN] baseline VERSION   mark migrations up to VERSION as applied
//
// The DSN needs permission to change the schema (CREATE, ALTER, INDEX,
// REFERENCES...), unlike the web app's own read/write-only account. It can also
// be set with SNIPPETBOX_MIGRATE_DSN.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/satyaorz/snippetbox/db"
	"github.com/satyaorz/snippetbox/internal/migrate"

	"github.com/go-sql-driver/mysql"
)

func main() {
	err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	defaultDSN := os.Getenv("SNIPPETBOX_MIGRATE_DSN")
	if defaultDSN == "" {
		defaultDSN = "root@/snippetbox"
	}
	dsn := fs.String("dsn", defaultDSN, "MySQL data source name with schema-changing privileges")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: migrate [-dsn DSN] up | status | baseline VERSION")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Migration files contain several statements each, and the status output
	// needs DATETIME columns scanned into time.Time.
	cfg, err := mysql.ParseDSN(*dsn)
	if err != nil {
		return fmt.Errorf("invalid DSN: %w", err)
	}
	cfg.MultiStatements = true
	cfg.ParseTime = true

	conn, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer conn.Close()

	migrations, err := migrate.Load(db.Migrations, "migrations")
	if err != nil {
		return err
	}
	m := &migrate.Migrator{DB: conn, Migrations: migrations}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	command := fs.Arg(0)
	if command == "" {
		command = "up"
	}

	switch command {
	case "up":
		ran, err := m.Up(ctx)
		for _, mig := range ran {
			fmt.Println("applied", mig)
		}
		if err == nil && len(ran) == 0 {
			fmt.Println("database is up to date")
		}
		return err

	case "status":
		statuses, err := m.Status(ctx)
		if err != nil {
			return err
		}
		for _, s := range statuses {
			state := "pending"
			if s.Applied {
				state = "applied " + s.AppliedAt.Format(time.DateTime)
			}
			fmt.Printf("%-30s %s\n", s.Migration, state)
		}
		return nil

	case "baseline":
		version, err := strconv.Atoi(fs.Arg(1))
		if err != nil {
			return fmt.Errorf("baseline needs a version number, e.g. `migrate baseline 2`")
		}
		marked, err := m.Baseline(ctx, version)
		for _, mig := range marked {
			fmt.Println("marked as applied", mig)
		}
		return err

	default:
		fs.Usage()
		return fmt.Errorf("unknown command %q", command)
	}
}
