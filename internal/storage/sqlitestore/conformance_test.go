package sqlitestore_test

import (
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/backend/conformance"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/sqlitestore"
)

func newTestStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".beads", "beads.db")
	s, err := sqlitestore.Open(t.Context(), path, sqlitestore.Options{Create: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := sqlitestore.Adapt(s); err != nil {
		t.Fatalf("adapt: %v", err)
	}
	if err := s.SetConfig(t.Context(), "issue_prefix", "test"); err != nil {
		t.Fatalf("set prefix: %v", err)
	}
	return s
}

func TestConformance(t *testing.T) {
	conformance.RunAll(t, func(t *testing.T) storage.DoltStorage { return newTestStore(t) })
}

func TestUnsupportedContract(t *testing.T) {
	conformance.RunUnsupportedContract(t, &sqlitestore.Store{}, sqlitestore.UnsupportedMethods())
}
