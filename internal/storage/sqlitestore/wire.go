package sqlitestore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"regexp"

	"modernc.org/sqlite"

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

func init() {
	// MySQL's `x REGEXP pattern` becomes regexp(pattern, x) in SQLite, which
	// has no built-in. MySQL matching is case-insensitive by default.
	sqlite.MustRegisterDeterministicScalarFunction("regexp", 2,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			pat, ok1 := args[0].(string)
			val, ok2 := args[1].(string)
			if !ok1 || !ok2 {
				return nil, nil
			}
			re, err := regexp.Compile("(?i)" + pat)
			if err != nil {
				return nil, err
			}
			if re.MatchString(val) {
				return int64(1), nil
			}
			return int64(0), nil
		})
}
