package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// T047: bootstrap on a sqlite workspace imports issues.jsonl into beads.db
// instead of exiting 1 with "backend sqlite cannot be bootstrapped".
func TestLiteBootstrapSQLiteImportsJSONL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".beads")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	line := `{"_type":"issue","id":"bp-1","title":"boot probe","status":"open","priority":2,"issue_type":"task","created_at":"2026-10-25T07:51:45Z","updated_at":"2026-10-25T07:51:45Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "issues.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := liteBootstrapSQLite(ctx, dir, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "beads.db")); err == nil {
		t.Fatal("dry run created beads.db")
	}
	if err := liteBootstrapSQLite(ctx, dir, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "beads.db")); err != nil {
		t.Fatalf("beads.db missing: %v", err)
	}
	// Second run leaves the existing database alone.
	if err := liteBootstrapSQLite(ctx, dir, false); err != nil {
		t.Fatal(err)
	}
	if got := litePrefixFromJSONL(filepath.Join(dir, "issues.jsonl"), true); got != "bp" {
		t.Fatalf("prefix %q", got)
	}
}
