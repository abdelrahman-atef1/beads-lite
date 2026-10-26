package sqlitestore

import (
	"context"

	"github.com/steveyegge/beads/internal/storage"
)

// History is a placeholder so the package satisfies storage.HistoryViewer
// before T009 lands. T009 owns history.go with the real lite_issue_versions
// reader: delete THIS FILE when merging it.
func (s *Store) History(_ context.Context, _ string) ([]*storage.HistoryEntry, error) {
	return nil, unsupported("History")
}
