package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/issueops"
)

// This file replaces the embeddeddolt connection plumbing (withConn,
// commitConn) with the single pooled SQLite handle of Store. Role files copied
// from embeddeddolt call withConn unchanged.

// ErrReadOnly is returned when a write is attempted on a read-only store.
var ErrReadOnly = errors.New("sqlitestore: store is read-only")

// journalEnabled holds the per-store events-journal flag (Store has no field
// for it; T006 owns the struct).
var journalEnabled sync.Map // *Store -> bool

// SetEventsJournalEnabled activates the journal for this store instance only.
func (s *Store) SetEventsJournalEnabled(enabled bool) { journalEnabled.Store(s, enabled) }

func (s *Store) eventsJournalOn() bool {
	v, ok := journalEnabled.Load(s)
	return ok && v.(bool)
}

// withConn runs fn in one transaction. commit=true goes through RunInTx (so
// before-commit hooks fire and the commit is atomic); commit=false rolls back.
func (s *Store) withConn(ctx context.Context, commit bool, fn func(tx *sql.Tx) error) error {
	pending, err := s.commitConn(ctx, commit, fn)
	if err != nil {
		return err
	}
	logBlockedRecheckFailure(pending, s.recheckBlockedAfterCommit(ctx, pending))
	return nil
}

func logBlockedRecheckFailure(pending issueops.BlockedRecheck, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "warning: %s\n", issueops.BlockedRecheckFailureMessage(pending, err))
}

func (s *Store) recheckBlockedAfterCommit(ctx context.Context, pending issueops.BlockedRecheck) error {
	if pending.Empty() || issueops.InBlockedRecheck(ctx) {
		return nil
	}
	ctx, cancel := issueops.BlockedRecheckContext(ctx)
	defer cancel()
	if _, err := s.commitConn(ctx, true, func(tx *sql.Tx) error {
		return issueops.RecomputeIsBlockedInTx(ctx, tx, pending.IssueIDs, pending.WispIDs)
	}); err != nil {
		return issueops.BlockedRecheckFailed(err)
	}
	return nil
}

var errRollbackOnly = errors.New("sqlitestore: read transaction rolled back")

func (s *Store) commitConn(ctx context.Context, commit bool, fn func(tx *sql.Tx) error) (pending issueops.BlockedRecheck, err error) {
	run := func(tx *sql.Tx) error {
		clearJournal := issueops.ScopeEventsJournalTransaction(tx, s.eventsJournalOn())
		defer clearJournal()
		clearRecheck := issueops.ScopeBlockedRecheckTransaction(tx)
		defer clearRecheck()
		if err := fn(tx); err != nil {
			return err
		}
		if commit {
			pending = issueops.TakeBlockedRecheck(tx)
		}
		return nil
	}
	if commit {
		if err = s.RunInTx(ctx, run); err != nil {
			pending = issueops.BlockedRecheck{}
		}
		return
	}
	// Read path: run and always roll back; hooks never fire.
	tx, berr := s.db.BeginTx(ctx, nil)
	if berr != nil {
		return pending, fmt.Errorf("sqlitestore: begin: %w", berr)
	}
	defer func() { _ = tx.Rollback() }()
	return pending, run(tx)
}

func wrapCommitIndeterminate(op string, err error) error {
	return fmt.Errorf("%s: %w: %w", op, err, storage.ErrCommitIndeterminate)
}
