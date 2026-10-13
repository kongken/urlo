package main

import (
	"os"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

// Goose validate inspects Go source unlike CollectMigrations, which can ignore
// unregistered Go migrations. Keep the canonical SQL directory SQL-only.
func TestGooseCLISQLDirectoryHasNoEmbedSource(t *testing.T) {
	entries, err := os.ReadDir("../../migrations/postgres")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".go") {
			t.Fatalf("Go source %q will be treated as a migration by goose validate", entry.Name())
		}
		if strings.HasSuffix(entry.Name(), ".sql") {
			if _, err := goose.NumericComponent(entry.Name()); err != nil {
				t.Fatal(err)
			}
		}
	}
}
