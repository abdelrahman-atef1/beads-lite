package main

import (
	"context"
	"fmt"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/storage/sqlitestore"
)

// lite: bd init defaults to sqlite (W5). The embedded-Dolt lock is never taken
// on the sqlite path: SQLite serialises writers itself, and the registry open
// arm in the pre-run (newRegisteredBackendStore) does not call it either.

func liteDefaultInitBackend() string { return configfile.BackendSQLite }

// liteInitBackendOK reports whether init can provision the backend itself.
func liteInitBackendOK(name string) bool {
	return name == configfile.BackendSQLite || name == configfile.BackendDolt
}

// liteInitBackend maps the (validated) --backend value to the backend init
// provisions; empty means the lite default.
func liteInitBackend(flag string) string {
	if flag == configfile.BackendDolt {
		return configfile.BackendDolt
	}
	return configfile.BackendSQLite
}

// newInitStore opens the store bd init works on: a new sqlite file (created
// with its schema) or, with --backend=dolt, the existing Dolt path.
func newInitStore(ctx context.Context, backend, beadsDir string, doltCfg *dolt.Config) (storage.DoltStorage, error) {
	if backend != configfile.BackendSQLite {
		return newDoltStore(ctx, doltCfg)
	}
	if sqlitestore.Adapt == nil {
		return nil, fmt.Errorf("sqlite storage roles are not wired in this build (T008)")
	}
	p, err := sqlitestore.ResolvePath(beadsDir)
	if err != nil {
		return nil, err
	}
	s, err := sqlitestore.Open(ctx, p, sqlitestore.Options{Create: true})
	if err != nil {
		return nil, err
	}
	st, err := sqlitestore.Adapt(s)
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	return st, nil
}
