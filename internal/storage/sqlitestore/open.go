package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// DefaultFileName is the database file inside .beads/.
const DefaultFileName = "beads.db"

// Options configure Open.
type Options struct {
	// Create allows creating the file and its schema when absent or empty
	// (bd init). Without it a missing file is an error.
	Create bool
	// Actor is the bd actor of this process, recorded as committer in history.
	Actor string
	// ReadOnly opens with query_only; the schema is never created.
	ReadOnly bool
	// NoHistory skips the lite_issue_versions writer. lite: the migration loader
	// seeds versions from Dolt history itself and must not get a second set.
	NoHistory bool
}

// openDB opens the pooled handle. The sqlite dialect translator (T005,
// internal/storage/sqlitedialect) replaces this default when it lands on lite,
// so issueops' MySQL SQL runs unchanged; the schema script needs no translation.
var openDB = func(dsn string) (*sql.DB, error) { return sql.Open("sqlite", dsn) }

// Open opens (and, with Options.Create, creates) the store at path, applying
// the version rules of ADR 0001: create on empty, open v1, refuse newer or
// unknown. The file must be on a local fixed disk.
func Open(ctx context.Context, path string, opts Options) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlitestore: empty database path")
	}
	if err := requireLocalFixedDisk(path); err != nil {
		return nil, fmt.Errorf("sqlitestore: %w", err)
	}
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("sqlitestore: %w", err)
		}
		if !opts.Create {
			return nil, fmt.Errorf("sqlitestore: no beads database at %s; run bd init", path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("sqlitestore: %w", err)
		}
	}
	d := dsn(path)
	if opts.ReadOnly {
		d += "&_pragma=query_only(1)"
		// lite: BEGIN IMMEDIATE is a write lock; a read-only handle must begin deferred.
		d = strings.Replace(d, "_txlock=immediate", "_txlock=deferred", 1)
	}
	db, err := openDB(d)
	if err != nil {
		return nil, fmt.Errorf("sqlitestore: open: %w", err)
	}
	// One connection per process: in-process callers queue on the pool, other
	// processes queue inside SQLite (busy_timeout). Research R3.
	db.SetMaxOpenConns(1)
	if err := ensureSchema(ctx, db, opts.Create && !opts.ReadOnly); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlitestore: %s: %w", path, err)
	}
	st := &Store{db: db, path: path, actor: opts.Actor}
	if !opts.ReadOnly && !opts.NoHistory {
		st.EnableHistory() // T009: every write commit records lite_issue_versions rows
	}
	return st, nil
}
