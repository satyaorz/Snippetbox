// Package db holds the database migrations, embedded so the migrate command
// (and the integration tests) can run them without needing the source tree.
package db

import "embed"

// Migrations contains the files in db/migrations, named NNNN_description.sql
// and applied in order by internal/migrate.
//
//go:embed migrations/*.sql
var Migrations embed.FS
