package sqlitestore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

// ddl is the frozen v1 schema (ADR 0001): the upstream v1.3.1 logical schema
// translated to SQLite, plus the lite_* history tables. It contains triggers,
// so it is executed as one script, never split on ';'.
//
//go:embed schema.sql
var ddl string

// SchemaVersion is the PRAGMA user_version this binary writes and accepts.
// A future change is a numbered step v1 -> v2 here, never a replay of upstream
// migrations (ADR 0001 section 1).
const SchemaVersion = 1

// defaultConfigSeeds mirrors upstream migration 0016; seeded on creation only.
var defaultConfigSeeds = [][2]string{
	{"compaction_enabled", "false"},
	{"compact_tier1_days", "30"},
	{"compact_tier1_dep_levels", "2"},
	{"compact_tier2_days", "90"},
	{"compact_tier2_dep_levels", "5"},
	{"compact_tier2_commits", "100"},
	{"compact_batch_size", "50"},
	{"compact_parallel_workers", "5"},
	{"auto_compact_enabled", "false"},
}

// ensureSchema creates an empty file's schema, accepts a v1 store, and refuses
// anything else. It never guesses: a file that has tables but no version, or a
// newer version, is left untouched.
func ensureSchema(ctx context.Context, db *sql.DB, create bool) error {
	version, tables, err := readState(ctx, db)
	if err != nil {
		return err
	}
	switch {
	case version == SchemaVersion:
		return nil
	case version > SchemaVersion:
		return fmt.Errorf("this store was written by a newer beads-lite (schema %d); this binary knows %d", version, SchemaVersion)
	case version == 0 && tables == 0:
		if !create {
			return fmt.Errorf("no beads database here (empty file); run bd init")
		}
		return createSchema(ctx, db)
	default:
		return fmt.Errorf("unrecognised database (schema version %d, %d tables): refusing to modify it", version, tables)
	}
}

func readState(ctx context.Context, db *sql.DB) (version, tables int, err error) {
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, 0, fmt.Errorf("sqlitestore: read user_version: %w", err)
	}
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&tables)
	if err != nil {
		return 0, 0, fmt.Errorf("sqlitestore: read schema: %w", err)
	}
	return version, tables, nil
}

func createSchema(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil) // BEGIN IMMEDIATE via the DSN
	if err != nil {
		return fmt.Errorf("sqlitestore: begin create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Another process may have created the schema while we waited for the lock.
	var v int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("sqlitestore: read user_version: %w", err)
	}
	if v == SchemaVersion {
		return nil
	}
	if _, err := tx.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("sqlitestore: create schema: %w", err)
	}
	if err := seedMigrations(ctx, tx, "schema_migrations", upstreamMainMigrations); err != nil {
		return err
	}
	if err := seedMigrations(ctx, tx, "ignored_schema_migrations", upstreamIgnoredMigrations); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO bd_events_seq (id, next_seq) VALUES (0, 0)"); err != nil {
		return fmt.Errorf("sqlitestore: seed bd_events_seq: %w", err)
	}
	for _, kv := range defaultConfigSeeds {
		if _, err := tx.ExecContext(ctx, "INSERT INTO config (`key`, `value`) VALUES (?, ?)", kv[0], kv[1]); err != nil {
			return fmt.Errorf("sqlitestore: seed config %q: %w", kv[0], err)
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		return fmt.Errorf("sqlitestore: stamp user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlitestore: commit create: %w", err)
	}
	return nil
}

// seedMigrations creates the upstream cursor table and records every v1.3.1
// migration as applied (version = position + 1). Read-only after creation.
func seedMigrations(ctx context.Context, tx *sql.Tx, table string, names []string) error {
	if _, err := tx.ExecContext(ctx, "CREATE TABLE "+table+
		" (version INTEGER PRIMARY KEY, content_hash CHAR(64), name TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("sqlitestore: create %s: %w", table, err)
	}
	for i, n := range names {
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+" (version, name) VALUES (?, ?)", i+1, n); err != nil {
			return fmt.Errorf("sqlitestore: seed %s: %w", table, err)
		}
	}
	return nil
}
