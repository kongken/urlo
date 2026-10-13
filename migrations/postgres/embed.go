// Package postgres contains the versioned SQL bundled with the migration tool.
package postgres

import "embed"

// Files is read-only migration SQL; adding a migration automatically bundles it.
//
//go:embed *.sql
var Files embed.FS
