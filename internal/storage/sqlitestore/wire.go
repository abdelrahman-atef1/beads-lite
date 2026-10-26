package sqlitestore

import (
	"context"
	"database/sql"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/storage/sqlitedialect"
)

// Compile-time interface checks.
var (
	_ storage.DoltStorage  = (*Store)(nil)
	_ storage.StoreLocator = (*Store)(nil)
)

func init() {
	// issueops writes MySQL SQL; the dialect driver translates it per statement
	// (T005). Replaces the plain driver default in open.go.
	openDB = sqlitedialect.Open
	Adapt = func(s *Store) (storage.DoltStorage, error) {
		// lite: this process serves SQLite, so Dolt-SQL paths in issueops
		// (dolt_*, wisp routing, journal prune) take their SQLite branches (T007).
		issueops.SetDoltSQL(false)
		return s, nil
	}
}

// CLIDir is the directory for dolt CLI operations; empty: SQLite has none.
func (s *Store) CLIDir() string { return "" }

// UnderlyingDB mirrors storage.RawDBAccessor for diagnostics.
func (s *Store) UnderlyingDB() *sql.DB { return s.db }

// RecomputeAllBlocked recomputes is_blocked for every issue and wisp in one
// pass and returns the number of rows corrected.
func (s *Store) RecomputeAllBlocked(ctx context.Context) (int, error) {
	var changed int64
	err := s.withConn(ctx, true, func(tx *sql.Tx) error {
		if err := issueops.GuardBlockedRecomputeWorkingSet(ctx, tx); err != nil {
			return err
		}
		var e error
		changed, e = issueops.RecomputeAllIsBlockedInTx(ctx, tx)
		return e
	})
	return int(changed), err
}
