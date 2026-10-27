package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/beads/internal/configfile"
)

// lite (T047): bd bootstrap on a sqlite workspace. Upstream bootstrap
// provisions Dolt only; for sqlite the useful cases are a fresh clone with
// an exported issues.jsonl and no beads.db (import it) or an empty workspace
// (create an empty database). An existing beads.db is left alone.

// liteBootstrapSQLite runs the sqlite bootstrap.
func liteBootstrapSQLite(ctx context.Context, beadsDir string, dryRun bool) error {
	dbFile := filepath.Join(beadsDir, "beads.db")
	jsonl := filepath.Join(beadsDir, "issues.jsonl")
	if _, err := os.Stat(dbFile); err == nil {
		fmt.Fprintf(os.Stderr, "Database already exists at %s; nothing to do.\n", dbFile)
		return nil
	}
	_, jerr := os.Stat(jsonl)
	hasJSONL := jerr == nil
	if dryRun {
		if hasJSONL {
			fmt.Fprintf(os.Stderr, "Would create %s and import %s.\n", dbFile, jsonl)
		} else {
			fmt.Fprintf(os.Stderr, "Would create an empty %s.\n", dbFile)
		}
		return nil
	}
	if err := os.MkdirAll(beadsDir, 0o750); err != nil {
		return fmt.Errorf("create beads directory: %w", err)
	}
	s, err := newInitStore(ctx, configfile.BackendSQLite, beadsDir, nil)
	if err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	defer func() { _ = s.Close() }()
	if p := litePrefixFromJSONL(jsonl, hasJSONL); p != "" {
		if err := s.SetConfig(ctx, "issue_prefix", p); err != nil {
			return fmt.Errorf("set issue prefix: %w", err)
		}
	}
	if !hasJSONL {
		fmt.Fprintf(os.Stderr, "Created empty database %s\n", dbFile)
		return nil
	}
	n, err := importFromLocalJSONL(ctx, s, jsonl)
	if err != nil {
		return fmt.Errorf("import from JSONL: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Imported %d issues from %s\n", n, jsonl)
	return nil
}

// litePrefixFromJSONL returns the issue prefix of the first issue id
// ("proj-ab1" -> "proj"), or "" when it cannot be told.
func litePrefixFromJSONL(path string, ok bool) string {
	if !ok {
		return ""
	}
	issues, _, err := parseJSONLFile(path)
	if err != nil || len(issues) == 0 {
		return ""
	}
	id := issues[0].ID
	if i := strings.LastIndex(id, "-"); i > 0 {
		return id[:i]
	}
	return ""
}
