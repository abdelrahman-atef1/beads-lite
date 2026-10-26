package sqlitestore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/backends"
)

// Adapt turns an open Store into the storage.DoltStorage the rest of bd uses.
// T008 (role files) sets it; until then opening through the registry reports
// that the roles are not wired, instead of returning a half-working store.
var Adapt func(*Store) (storage.DoltStorage, error)

// ResolvePath returns the database file for a workspace: metadata.json's
// sqlite_path (default beads.db), relative to the beads dir.
func ResolvePath(beadsDir string) (string, error) {
	cfg, err := configfile.Load(beadsDir)
	if err != nil {
		return "", fmt.Errorf("sqlitestore: load config: %w", err)
	}
	p := DefaultFileName
	if cfg != nil && cfg.GetSQLitePath() != "" {
		p = cfg.GetSQLitePath()
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(beadsDir, p)
	}
	return p, nil
}

func openWorkspace(opts Options) func(context.Context, string) (storage.DoltStorage, error) {
	return func(ctx context.Context, beadsDir string) (storage.DoltStorage, error) {
		if Adapt == nil {
			return nil, errors.New("sqlitestore: storage roles are not wired in this build (T008)")
		}
		p, err := ResolvePath(beadsDir)
		if err != nil {
			return nil, err
		}
		s, err := Open(ctx, p, opts)
		if err != nil {
			return nil, err
		}
		st, err := Adapt(s)
		if err != nil {
			_ = s.Close()
			return nil, err
		}
		return st, nil
	}
}

// Register makes "sqlite" a known backend (lifts the removed-backend refusal,
// since the store dispatch checks backends.Registered first). cmd/bd calls it
// once at start-up (T011); it panics on a second call like backends.Register.
func Register() {
	backends.Register(Backend, backends.Backend{
		Open:         openWorkspace(Options{}),
		OpenReadOnly: openWorkspace(Options{ReadOnly: true}),
	})
}
