// Package migrations bundles versioned SQL without adding Go source to Goose's
// migration directory, where the CLI would interpret it as a Go migration.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed postgres/*.sql
var files embed.FS

// Postgres returns only the canonical PostgreSQL migration files.
func Postgres() (fs.FS, error) {
	return fs.Sub(files, "postgres")
}
