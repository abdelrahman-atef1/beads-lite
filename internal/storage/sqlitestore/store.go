// Package sqlitestore is the beads-lite storage backend: one SQLite file per
// project (.beads/beads.db), pure Go (modernc.org/sqlite, no CGO). Schema and
// version rules are frozen by docs/decisions/0001-frozen-sqlite-schema.md.
//
// File ownership: T006 owns this file, transaction.go, open.go, dsn.go,
// schema.go and schema.sql. Role files (T008) and history (T009) build on
// Store and on the BeforeCommit hook list below.
package sqlitestore

import (
	"context"
	"database/sql"
	"sync"
)

// Backend is the metadata.json backend name.
const Backend = "sqlite"

// BeforeCommitHook runs inside a write transaction, after the command's own
// statements and before COMMIT. Returning an error rolls the transaction back.
// T009 registers the lite_issue_versions writer here.
type BeforeCommitHook func(ctx context.Context, tx *sql.Tx) error

// Store is an open beads-lite database. It holds a single connection pool
// pinned to one connection, so in-process callers serialize on it and
// cross-process contention is left to SQLite (WAL plus busy_timeout).
type Store struct {
	db   *sql.DB
	path string
	// actor is the bd actor of this process, recorded as committer in history.
	actor string

	hookMu sync.RWMutex
	hooks  []BeforeCommitHook
}

// DB returns the underlying handle for read queries and for role files that
// run issueops code against a *sql.DB. Writes must go through RunInTx so the
// before-commit hooks fire.
func (s *Store) DB() *sql.DB { return s.db }

// Path returns the database file path the store was opened on.
func (s *Store) Path() string { return s.path }

// Actor returns the bd actor of this process ("" when none was given).
func (s *Store) Actor() string { return s.actor }

// Close releases the database. It is safe to call more than once.
func (s *Store) Close() error { return s.db.Close() }

// CommitGraphUnsupported tells cmd/bd to skip Dolt-only commit maintenance.
func (s *Store) CommitGraphUnsupported() bool { return true }

// CommitPending is a no-op: every write is durable when its transaction commits.
func (s *Store) CommitPending(_ context.Context, _ string) (bool, error) { return false, nil }
