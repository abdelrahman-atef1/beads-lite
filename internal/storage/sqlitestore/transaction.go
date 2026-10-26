package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
)

// RegisterBeforeCommit adds a hook that runs, in registration order, inside
// every write transaction started by RunInTx just before COMMIT. Register
// during store setup, before concurrent use.
func (s *Store) RegisterBeforeCommit(h BeforeCommitHook) {
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	s.hooks = append(s.hooks, h)
}

// RunInTx runs fn in one write transaction (BEGIN IMMEDIATE via the DSN). If fn
// succeeds, every before-commit hook runs on the same tx and then it commits;
// if fn or a hook fails, or ctx ends, it rolls back and no hook effect persists.
// A panic in fn rolls back and is re-raised.
func (s *Store) RunInTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlitestore: begin: %w", err)
	}
	done := false
	defer func() {
		if !done {
			_ = tx.Rollback()
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	s.hookMu.RLock()
	hooks := append([]BeforeCommitHook(nil), s.hooks...)
	s.hookMu.RUnlock()
	for _, h := range hooks {
		if err := h(ctx, tx); err != nil {
			return fmt.Errorf("sqlitestore: before-commit hook: %w", err)
		}
	}
	done = true
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlitestore: commit: %w", err)
	}
	return nil
}
