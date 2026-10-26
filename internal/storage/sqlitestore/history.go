package sqlitestore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// boolCols are 0/1 columns whose Issue fields are bool.
var boolCols = []string{"is_blocked", "ephemeral", "no_history", "pinned", "is_template"}

// History implements storage.HistoryViewer: every version of one issue.
func (s *Store) History(ctx context.Context, id string) ([]*storage.HistoryEntry, error) {
	return s.HistoryLimit(ctx, id, 0)
}

// HistoryLimit returns the versions of one issue, newest first (Dolt order), at
// most limit when limit > 0. An unknown id yields an empty slice, no error.
func (s *Store) HistoryLimit(ctx context.Context, id string, limit int) ([]*storage.HistoryEntry, error) {
	q := `SELECT v.seq, v.committed_at, v.committer, v.scalars, t.body
	      FROM lite_issue_versions v JOIN lite_version_text t ON t.hash = v.text_hash
	      WHERE v.issue_id = ? ORDER BY v.seq DESC`
	args := []any{id}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	defer rows.Close()
	out := []*storage.HistoryEntry{}
	for rows.Next() {
		var seq int64
		var at, committer, scalars, body string
		if err := rows.Scan(&seq, &at, &committer, &scalars, &body); err != nil {
			return nil, err
		}
		iss, err := rebuildIssue(scalars, body)
		if err != nil {
			return nil, fmt.Errorf("history: version %d: %w", seq, err)
		}
		when, _ := time.Parse(time.RFC3339, at)
		out = append(out, &storage.HistoryEntry{
			CommitHash: fmt.Sprintf("lite-%d", seq),
			Committer:  committer,
			CommitDate: when,
			Issue:      iss,
		})
	}
	return out, rows.Err()
}

func rebuildIssue(scalars, body string) (*types.Issue, error) {
	m := map[string]any{}
	if err := json.Unmarshal([]byte(scalars), &m); err != nil {
		return nil, err
	}
	for _, c := range boolCols {
		if f, ok := m[c].(float64); ok {
			m[c] = f != 0
		}
	}
	if v, ok := m["timeout_ns"]; ok { // column name differs from the JSON tag
		m["timeout"] = v
	}
	if w, ok := m["waiters"].(string); ok {
		var l []string
		if json.Unmarshal([]byte(w), &l) == nil {
			m["waiters"] = l
		} else {
			delete(m, "waiters")
		}
	}
	var b textBundle
	if err := json.Unmarshal([]byte(body), &b); err != nil {
		return nil, err
	}
	m["description"], m["design"] = b.Description, b.Design
	m["acceptance_criteria"], m["notes"] = b.AcceptanceCriteria, b.Notes
	if b.Metadata != "" && json.Valid([]byte(b.Metadata)) {
		m["metadata"] = json.RawMessage(b.Metadata)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var iss types.Issue
	if err := json.Unmarshal(raw, &iss); err != nil {
		return nil, err
	}
	return &iss, nil
}

// IterEvents streams the audit events of one issue from the events table,
// newest first, at most limit when limit > 0.
func (s *Store) IterEvents(ctx context.Context, issueID string, limit int) (storage.Iter[types.Event], error) {
	q := `SELECT id, issue_id, event_type, actor, old_value, new_value, comment, created_at
	      FROM events WHERE issue_id = ? ORDER BY created_at DESC, id DESC`
	args := []any{issueID}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("events: %w", err)
	}
	defer rows.Close()
	var evs []*types.Event
	for rows.Next() {
		var e types.Event
		var et string
		if err := rows.Scan(&e.ID, &e.IssueID, &et, &e.Actor, &e.OldValue, &e.NewValue, &e.Comment, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.EventType = types.EventType(et)
		evs = append(evs, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return storage.NewSliceIter(evs), nil
}
